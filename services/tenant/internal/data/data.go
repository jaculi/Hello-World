// Package data —— BC-P1 数据访问层（BP-03 §3/§4，步 15-16）。
//
// 硬约束：
//   - 仓储实现面向 pkg/dataaccess 的 Tx 接口与 RLS 事务口，不触碰连接池；
//   - 租户数据的可见性由数据库 RLS 策略兜底（app.tenant_id 事务级绑定）；
//   - 唯一约束冲突映射为领域消息码（name_taken）；乐观锁以 WHERE version=$expected 落实。
//
// 装配：New（PostgreSQL）供生产；main 在无 DSN 的开发环境以 biz 内存替身顶替。
package data

import (
	"context"

	"github.com/jsl-aiot/platform/pkg/dataaccess"

	"github.com/jsl-aiot/platform/services/tenant/internal/biz"
)

// Data 数据层装配：领域端口 → SQL 实现。
type Data struct {
	pool *dataaccess.Pool

	Tx      biz.Transactor    // 事务口（TenantTx / TenantTxAs）
	Tenants biz.TenantRepo    // 租户聚合（含订阅与装配记录）
	Orgs    biz.OrgRepo       // 组织树
	Sites   biz.SiteRepo      // 站点树
	Plans   biz.PlanRepo      // 套餐目录（全局读模型）
	Events  biz.EventPublisher // 事务性发件箱（ADR-0005）
}

// New 依连接池装配数据层。
func New(pool *dataaccess.Pool) *Data {
	tx := &txer{pool: pool}
	return &Data{
		pool:    pool,
		Tx:      tx,
		Tenants: &TenantRepo{},
		Orgs:    &OrgRepo{},
		Sites:   &SiteRepo{},
		Plans:   &PlanRepo{},
		Events:  &Outbox{},
	}
}

// Close 释放连接池。
func (d *Data) Close() {
	if d.pool != nil {
		d.pool.Close()
	}
}

// txer Transactor 的 SQL 实现。
type txer struct {
	pool *dataaccess.Pool
}

func (t *txer) WithinTx(ctx context.Context, fn func(context.Context, biz.Tx) error) error {
	return t.pool.TenantTx(ctx, fn)
}

func (t *txer) WithinTenantTx(ctx context.Context, tenantID string, fn func(context.Context, biz.Tx) error) error {
	return t.pool.TenantTxAs(ctx, tenantID, fn)
}
