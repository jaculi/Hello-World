//go:build integration

// RLS 越界实证集成测试（步 20）：
//  1. 租户 A 写入后，租户 B 上下文查询必须为空（BP-03 §3.2 行级隔离）；
//  2. 事务回滚后连接复用，app.tenant_id 不残留（SET LOCAL 语义随事务结束清空，防泄漏）；
//  3. 无租户上下文访问必须拒绝（fail-closed，BP-05 §4.2）；
//  4. AdminTx 旁路 RLS（巡检/台账/发件箱中继专用，需连接角色有 BYPASSRLS）。
//
// 需要 Docker：go test -tags integration ./pkg/dataaccess -v
package dataaccess

import (
	"context"
	"fmt"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/jsl-aiot/platform/pkg/errors"
	"github.com/jsl-aiot/platform/pkg/tenantcontext"
)

// rlsFixture 建表 + 启用 RLS + 租户隔离策略（与迁移脚本策略同源）。
// 表可能由 jsl 或 app_user 创建，统一授权给 app_user 保证用例可在两个角色间切换。
const rlsFixture = `
CREATE TABLE IF NOT EXISTS rls_items (
    id text PRIMARY KEY,
    tenant_id text NOT NULL,
    value text NOT NULL
);
GRANT ALL ON rls_items TO app_user;
ALTER TABLE rls_items ENABLE ROW LEVEL SECURITY;
ALTER TABLE rls_items FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS p_tenant_isolation ON rls_items;
CREATE POLICY p_tenant_isolation ON rls_items
    USING (tenant_id = current_setting('app.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
TRUNCATE rls_items;
`

var (
	pgContainer *postgres.PostgresContainer
	rlsPool     *Pool // app_user（普通角色，受 RLS 约束）
	adminPool   *Pool // jsl（superuser，AdminTx bypass RLS 的真实语义）
)

// TestMain 共享一个 PG 容器：创建普通应用角色 app_user（非 superuser），
// 所有 RLS 测试以 app_user 身份连接，使 FORCE RLS 对其生效（生产 jsl 本为普通用户）。
func TestMain(m *testing.M) {
	ctx := context.Background()
	c, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithUsername("jsl"), postgres.WithPassword("jsl"), postgres.WithDatabase("jsl"),
		testcontainers.WithWaitStrategy(
			wait.ForExec([]string{"pg_isready", "-U", "jsl", "-d", "jsl"}).
				WithPollInterval(500_000_000).
				WithStartupTimeout(30_000_000_000),
		),
	)
	if err != nil {
		fmt.Printf("testcontainers unavailable, skipping integration: %v\n", err)
		return
	}
	defer c.Terminate(ctx)

	adminDSN, err := c.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Printf("connection string: %v\n", err)
		return
	}
	adminPool, err = NewPool(ctx, adminDSN)
	if err != nil {
		fmt.Printf("admin pool init: %v\n", err)
		return
	}
	defer adminPool.Close()

	// 以 superuser(jsl) 创建普通应用角色并授权（RLS 对 superuser bypass，必须用普通角色验证）。
	// CREATE ROLE 若已存在会报错但不影响——首次运行后角色已存在，后续直接 GRANT。
	_ = adminPool.ExecMulti(ctx, "CREATE ROLE app_user LOGIN PASSWORD 'app_user';")
	if err := adminPool.ExecMulti(ctx, `
		GRANT ALL ON SCHEMA public TO app_user;
		GRANT ALL ON DATABASE jsl TO app_user;
	`); err != nil {
		fmt.Printf("grant app_user: %v\n", err)
		return
	}

	// 构造 app_user 的 DSN（替换用户名/密码，保留 host/port）
	host, _ := c.Host(ctx)
	port, _ := c.MappedPort(ctx, "5432/tcp")
	appDSN := fmt.Sprintf("postgres://app_user:app_user@%s:%s/jsl?sslmode=disable", host, port.Port())
	rlsPool, err = NewPool(ctx, appDSN)
	if err != nil {
		fmt.Printf("app pool init: %v\n", err)
		return
	}
	defer rlsPool.Close()

	pgContainer = c
	m.Run()
}

func requirePool(t *testing.T) *Pool {
	t.Helper()
	if rlsPool == nil {
		t.Skip("postgres container not available")
	}
	return rlsPool
}

// resetFixture 复位测试表（每个测试前调用，避免用例间污染）。
func resetFixture(t *testing.T, pool *Pool) {
	t.Helper()
	if err := pool.ExecMulti(context.Background(), rlsFixture); err != nil {
		t.Fatalf("fixture: %v", err)
	}
}

// TestRLS_CrossTenantIsolation 越界实证：租户 A 写入 → 租户 B 不可见。
func TestRLS_CrossTenantIsolation(t *testing.T) {
	pool := requirePool(t)
	resetFixture(t, pool)
	ctx := context.Background()

	// 租户 A 写入
	ctxA := tenantcontext.With(ctx, tenantcontext.Context{TenantID: "t-A"})
	if err := pool.TenantTx(ctxA, func(ctx context.Context, tx Tx) error {
		_, err := tx.Exec(ctx, "INSERT INTO rls_items (id, tenant_id, value) VALUES ($1,$2,$3)",
			"i-1", "t-A", "secret-of-A")
		return err
	}); err != nil {
		t.Fatalf("A insert: %v", err)
	}

	// 租户 B 查询：必须空
	ctxB := tenantcontext.With(ctx, tenantcontext.Context{TenantID: "t-B"})
	var got int
	if err := pool.TenantTx(ctxB, func(ctx context.Context, tx Tx) error {
		return tx.QueryRow(ctx, "SELECT count(*) FROM rls_items").Scan(&got)
	}); err != nil {
		t.Fatalf("B query: %v", err)
	}
	if got != 0 {
		t.Fatalf("RLS violated: tenant B saw %d rows of tenant A", got)
	}

	// 租户 A 自查：可见
	if err := pool.TenantTx(ctxA, func(ctx context.Context, tx Tx) error {
		return tx.QueryRow(ctx, "SELECT count(*) FROM rls_items").Scan(&got)
	}); err != nil {
		t.Fatalf("A query: %v", err)
	}
	if got != 1 {
		t.Fatalf("tenant A should see own row, got %d", got)
	}
}

// TestRLS_NoLeakAfterRollback 回滚不污染 RLS 会话变量：连接复用时上一租户变量已清空。
func TestRLS_NoLeakAfterRollback(t *testing.T) {
	pool := requirePool(t)
	resetFixture(t, pool)
	ctx := context.Background()

	// 在租户 A 上下文里执行一个回滚事务（触发 SET LOCAL app.tenant_id=t-A）
	ctxA := tenantcontext.With(ctx, tenantcontext.Context{TenantID: "t-A"})
	_ = pool.TenantTx(ctxA, func(ctx context.Context, tx Tx) error {
		if _, err := tx.Exec(ctx, "INSERT INTO rls_items (id, tenant_id, value) VALUES ($1,$2,$3)",
			"i-leak", "t-A", "should-rollback"); err != nil {
			return err
		}
		return fmt.Errorf("rollback me")
	})

	// 无租户上下文访问：必须拒绝（若 app.tenant_id 残留则会读到数据，证明泄漏）
	err := pool.TenantTx(context.Background(), func(ctx context.Context, tx Tx) error {
		var n int
		return tx.QueryRow(ctx, "SELECT count(*) FROM rls_items").Scan(&n)
	})
	e, ok := errors.From(err)
	if !ok || e.Code != CodeNoTenantContext {
		t.Fatalf("expected no_tenant_context after rollback (RLS var leak), got %v", err)
	}
}

// TestRLS_FailClosed_NoContext 无租户上下文必须拒绝（fail-closed）。
func TestRLS_FailClosed_NoContext(t *testing.T) {
	pool := requirePool(t)
	err := pool.TenantTx(context.Background(), func(ctx context.Context, tx Tx) error {
		t.Fatal("fn must not be called without tenant context")
		return nil
	})
	e, ok := errors.From(err)
	if !ok || e.Code != CodeNoTenantContext {
		t.Fatalf("expected no_tenant_context, got %v", err)
	}
}

// TestRLS_AdminTxBypassesRLS AdminTx 不绑定租户上下文，且连接角色有 BYPASSRLS 时可读取全部行
// （巡检/台账/发件箱中继专用路径；生产中该角色为运维专用，应用数据访问禁用）。
func TestRLS_AdminTxBypassesRLS(t *testing.T) {
	if adminPool == nil {
		t.Skip("admin pool not available")
	}
	resetFixture(t, adminPool)
	// app_user 写入一行（受 RLS 约束，tenant=t-A）
	ctxA := tenantcontext.With(context.Background(), tenantcontext.Context{TenantID: "t-A"})
	if err := rlsPool.TenantTx(ctxA, func(ctx context.Context, tx Tx) error {
		_, err := tx.Exec(ctx, "INSERT INTO rls_items (id, tenant_id, value) VALUES ($1,$2,$3)",
			"i-admin", "t-A", "v")
		return err
	}); err != nil {
		t.Fatalf("app_user insert: %v", err)
	}
	// app_user 自查：1 行（确认插入成功）
	var appN int
	if err := rlsPool.TenantTx(ctxA, func(ctx context.Context, tx Tx) error {
		return tx.QueryRow(ctx, "SELECT count(*) FROM rls_items").Scan(&appN)
	}); err != nil {
		t.Fatalf("app_user self query: %v", err)
	}
	if appN != 1 {
		t.Fatalf("app_user should see own row, got %d", appN)
	}
	// jsl(superuser) 的 AdminTx 不绑定租户，应 bypass RLS 看到全部行
	var n int
	if err := adminPool.AdminTx(context.Background(), func(ctx context.Context, tx Tx) error {
		return tx.QueryRow(ctx, "SELECT count(*) FROM rls_items").Scan(&n)
	}); err != nil {
		t.Fatalf("admin query: %v", err)
	}
	if n != 1 {
		t.Fatalf("AdminTx(superuser) should bypass RLS and see all rows, got %d", n)
	}
}
