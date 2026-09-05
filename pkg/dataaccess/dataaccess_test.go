package dataaccess

import (
	"context"
	"testing"

	"github.com/jsl-aiot/platform/pkg/errors"
)

// TestTenantTx_FailClosed 无租户上下文必须拒绝（不触碰数据库）。
func TestTenantTx_FailClosed(t *testing.T) {
	p := &Pool{} // 零值池：若实现触碰 pool 会 panic，测试即失败
	err := p.TenantTx(context.Background(), func(ctx context.Context, tx Tx) error {
		t.Fatal("fn must not be called")
		return nil
	})
	e, ok := errors.From(err)
	if !ok || e.Code != CodeNoTenantContext {
		t.Fatalf("expected no_tenant_context, got %v", err)
	}
}

func TestMemRouteTable(t *testing.T) {
	rt := NewMemRouteTable()
	if err := rt.Register("t-1", Route{Level: LevelT3}); err != nil {
		t.Fatal(err)
	}
	// T3 不允许携带 Schema/DSN
	if err := rt.Register("t-2", Route{Level: LevelT3, DSN: "postgres://x"}); err == nil {
		t.Fatal("T3 with DSN must be rejected")
	}
	if _, ok := rt.Lookup("t-1"); !ok {
		t.Fatal("t-1 route missing")
	}
	if err := rt.Register("", Route{Level: LevelT3}); err == nil {
		t.Fatal("empty tenant id must be rejected")
	}
}
