package tenantcontext

import (
	"context"
	"testing"
)

func TestWithFrom(t *testing.T) {
	tc := Context{TenantID: "t-1", OrgID: "o-1", SiteID: "s-1"}
	ctx := With(context.Background(), tc)
	got, ok := From(ctx)
	if !ok || got.TenantID != "t-1" || got.OrgID != "o-1" || got.SiteID != "s-1" {
		t.Fatalf("From mismatch: %+v ok=%v", got, ok)
	}
}

func TestFromEmptyTenantRejected(t *testing.T) {
	// TenantID 为空的上下文视为无租户（fail-closed 语义）
	ctx := With(context.Background(), Context{OrgID: "o-1"})
	if _, ok := From(ctx); ok {
		t.Fatal("empty tenant must not resolve")
	}
}

func TestWithSubject(t *testing.T) {
	ctx := WithSubject(With(context.Background(), Context{TenantID: "t-1"}), "user-9")
	got, _ := From(ctx)
	if got.Subject != "user-9" || got.TenantID != "t-1" {
		t.Fatalf("subject not merged: %+v", got)
	}
}
