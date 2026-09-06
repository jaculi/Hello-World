// Package dataaccess —— RLS 数据访问基类（BP-03 §3.2 / §4.5）。
//
// 硬约束：
//   - 租户数据访问必须在 TenantTx 内进行：事务级 GUC（SET LOCAL 语义）随提交/回滚自动失效，
//     连接归还连接池时绝不残留租户会话变量（防泄漏，M5 步 20 越界实证覆盖）；
//   - 无租户上下文 → 拒绝（fail-closed，BP-05 §4.2 第二道闸）；
//   - 本期仅 T3（单库行级隔离）；T1/T2 路由经 RouteTable 接口预留（BP-03 §3.1）。
package dataaccess

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jsl-aiot/platform/pkg/errors"
	"github.com/jsl-aiot/platform/pkg/tenantcontext"
)

// RLS 会话变量键（迁移脚本中的 RLS 策略引用同名 GUC，BP-03 §4.1）。
const (
	GUCTenantID = "app.tenant_id"
	GUCOrgID    = "app.org_id"
	GUCSiteID   = "app.site_id"
)

// 消息码：无租户上下文的数据库访问（链违规）。
const CodeNoTenantContext = "platform.data_no_tenant_context"

// Tx 数据访问事务接口 —— pgx.Tx 的最小子集。
// 数据层代码仅面向本接口编程，不触碰连接池（BP-03 §4.5 封装约束）。
type Tx interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// Pool 数据访问池（T3 默认池）。
type Pool struct {
	p *pgxpool.Pool
}

// NewPool 建立连接池。
func NewPool(ctx context.Context, dsn string) (*Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, errors.Wrap(err, "platform.data_invalid_dsn", 500)
	}
	p, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, errors.Wrap(err, "platform.data_pool_init_failed", 500)
	}
	return &Pool{p: p}, nil
}

// Close 关闭连接池。
func (p *Pool) Close() { p.p.Close() }

// TenantTx 在 RLS 保护事务中执行 fn：
//
//	BEGIN → set_config(app.tenant_id, …, is_local=true) → fn → COMMIT；fn 返回错误或 panic → ROLLBACK
//
// set_config 第三参 true 即 SET LOCAL 语义：变量作用域锁定本事务，随事务结束自动清空。
// org/site 可得时一并写入，供行业级行级策略（BP-03 §3）使用。
func (p *Pool) TenantTx(ctx context.Context, fn func(ctx context.Context, tx Tx) error) error {
	tc, ok := tenantcontext.From(ctx)
	if !ok || tc.TenantID == "" {
		// fail-closed：无租户上下文拒绝触碰数据库
		return errors.New(CodeNoTenantContext, 500)
	}
	return p.tenantTxAs(ctx, tc, fn)
}

// TenantTxAs 以目标租户身份执行（开通与租户级管理操作，BP-04 §4.5）：
// 保留原上下文 Subject 供审计记录真实操作者（BP-05 §9.1）。
func (p *Pool) TenantTxAs(ctx context.Context, tenantID string, fn func(ctx context.Context, tx Tx) error) error {
	if tenantID == "" {
		return errors.New(CodeNoTenantContext, 500)
	}
	tc, _ := tenantcontext.From(ctx) // 仅保留 Subject；租户以目标租户为准
	return p.tenantTxAs(ctx, tenantcontext.Context{TenantID: tenantID, Subject: tc.Subject}, fn)
}

// tenantTxAs 以指定租户上下文绑定 RLS 会话变量并执行 fn（TenantTx/TenantTxAs 共同路径）。
func (p *Pool) tenantTxAs(ctx context.Context, tc tenantcontext.Context, fn func(ctx context.Context, tx Tx) error) error {
	return pgx.BeginFunc(ctx, p.p, func(pgxTx pgx.Tx) error {
		if _, err := pgxTx.Exec(ctx, "SELECT set_config($1, $2, true)", GUCTenantID, tc.TenantID); err != nil {
			return errors.Wrap(err, "platform.data_rls_bind_failed", 500)
		}
		if tc.OrgID != "" {
			if _, err := pgxTx.Exec(ctx, "SELECT set_config($1, $2, true)", GUCOrgID, tc.OrgID); err != nil {
				return errors.Wrap(err, "platform.data_rls_bind_failed", 500)
			}
		}
		if tc.SiteID != "" {
			if _, err := pgxTx.Exec(ctx, "SELECT set_config($1, $2, true)", GUCSiteID, tc.SiteID); err != nil {
				return errors.Wrap(err, "platform.data_rls_bind_failed", 500)
			}
		}
		return fn(ctx, pgxTx)
	})
}

// ExecMulti 以简单协议执行多语句 SQL（迁移脚本专用；应用查询禁用）。
// 整段脚本须自带 BEGIN/COMMIT 保证原子性。
func (p *Pool) ExecMulti(ctx context.Context, script string) error {
	c, err := p.p.Acquire(ctx)
	if err != nil {
		return errors.Wrap(err, "platform.data_conn_failed", 500)
	}
	defer c.Release()
	res := c.Conn().PgConn().Exec(ctx, script)
	if _, err := res.ReadAll(); err != nil {
		return errors.Wrap(err, "platform.data_migration_failed", 500)
	}
	return nil
}

// AdminQueryStrings 执行管理查询并返回首列文本结果（迁移台账/巡检专用；
// 不绑定租户上下文——RLS 之外的特权路径，应用数据访问禁用）。
func (p *Pool) AdminQueryStrings(ctx context.Context, sql string, args ...any) ([]string, error) {
	c, err := p.p.Acquire(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "platform.data_conn_failed", 500)
	}
	defer c.Release()
	rows, err := c.Conn().Query(ctx, sql, args...)
	if err != nil {
		return nil, errors.Wrap(err, "platform.data_query_failed", 500)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, errors.Wrap(err, "platform.data_query_failed", 500)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.Wrap(err, "platform.data_query_failed", 500)
	}
	return out, nil
}
