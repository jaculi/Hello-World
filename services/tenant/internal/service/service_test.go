// 接口层测试：协议映射 + 错误编码（biz 规则由领域层测试覆盖；RLS/SQL 实证见 M5 集成测试）。
package service

import (
	"context"
	"testing"

	kratoserrors "github.com/go-kratos/kratos/v3/errors"

	commonv1 "github.com/jsl-aiot/platform/api/common/v1"
	orgv1 "github.com/jsl-aiot/platform/api/org/v1"
	sitev1 "github.com/jsl-aiot/platform/api/site/v1"
	tenantv1 "github.com/jsl-aiot/platform/api/tenant/v1"
	"github.com/jsl-aiot/platform/pkg/audit"
	"github.com/jsl-aiot/platform/pkg/tenantcontext"

	"github.com/jsl-aiot/platform/services/tenant/internal/biz"
)

// nopSink 审计空汇。
type nopSink struct{}

func (nopSink) Emit(_ context.Context, _ audit.Event) error { return nil }

// env 服务层测试环境（biz 内存替身装配）。
type env struct {
	tenant *TenantService
	org    *OrgService
	site   *SiteService
}

func newEnv() *env {
	em := audit.NewEmitter(nopSink{})
	plans := biz.NewMemPlanRepo(biz.Plan{ID: "plan-free", Name: "Free", DefaultIsolation: biz.IsolationT3, Active: true})
	tx := biz.MemTransactor{}
	tenants, orgs, sites := biz.NewMemTenantRepo(), biz.NewMemOrgRepo(), biz.NewMemSiteRepo()
	events := &biz.MemEventPublisher{}
	return &env{
		tenant: NewTenantService(biz.NewTenantUseCase(tx, tenants, plans, orgs, sites, events, em)),
		org:    NewOrgService(biz.NewOrgUseCase(tx, orgs, em)),
		site:   NewSiteService(biz.NewSiteUseCase(tx, sites, events, em)),
	}
}

func ctxAs(tenantID string) context.Context {
	return tenantcontext.With(context.Background(), tenantcontext.Context{TenantID: tenantID, Subject: "test:" + tenantID})
}

func ctxPlatform() context.Context {
	return tenantcontext.With(context.Background(), tenantcontext.Context{TenantID: "platform", Subject: "test:platform"})
}

// provision 经接口层开通租户（平台侧上下文），返回 tenant_id。
func provision(t *testing.T, e *env, name string) string {
	t.Helper()
	tt, err := e.tenant.CreateTenant(ctxPlatform(), &tenantv1.CreateTenantRequest{
		Name: name, PlanId: "plan-free",
	})
	if err != nil {
		t.Fatalf("provision %s: %v", name, err)
	}
	return tt.TenantId
}

// wantReason 断言错误为 kratos 传输错误且 reason（消息码）匹配，返回 HTTP 状态码。
func wantReason(t *testing.T, err error, reason string) int {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	ke := kratoserrors.FromError(err)
	if ke == nil {
		t.Fatal("expected kratos error, got nil")
	}
	if ke.Reason != reason {
		t.Fatalf("reason = %s, want %s", ke.Reason, reason)
	}
	return int(ke.Code)
}

// ---------- 租户接口 ----------

func TestCreateTenantMapping(t *testing.T) {
	e := newEnv()
	tt, err := e.tenant.CreateTenant(ctxPlatform(), &tenantv1.CreateTenantRequest{
		Name: "acme", PlanId: "plan-free",
	})
	if err != nil {
		t.Fatal(err)
	}
	if tt.TenantId == "" || tt.Name != "acme" {
		t.Fatalf("标识映射异常: %+v", tt)
	}
	if tt.Status != tenantv1.TenantStatus_TENANT_STATUS_ACTIVE {
		t.Fatalf("status = %s, want ACTIVE", tt.Status)
	}
	if tt.Isolation != tenantv1.IsolationLevel_ISOLATION_LEVEL_T3_ROW {
		t.Fatalf("isolation = %s, want T3_ROW（套餐默认）", tt.Isolation)
	}
	if tt.DisplayName != "acme" {
		t.Fatalf("display_name 回退缺失: %s", tt.DisplayName)
	}
	if tt.Audit == nil || tt.Audit.Version != 1 {
		t.Fatalf("audit 字段缺失: %+v", tt.Audit)
	}
}

func TestCreateTenantNameInvalid(t *testing.T) {
	e := newEnv()
	_, err := e.tenant.CreateTenant(ctxPlatform(), &tenantv1.CreateTenantRequest{Name: "BAD NAME", PlanId: "plan-free"})
	if got := wantReason(t, err, "platform.tenant.name_invalid"); got != 400 {
		t.Fatalf("http code = %d, want 400", got)
	}
}

func TestGetTenantCrossTenantDenied(t *testing.T) {
	e := newEnv()
	a := provision(t, e, "acme")
	_, err := e.tenant.GetTenant(ctxAs("other-tenant"), &tenantv1.GetTenantRequest{TenantId: a})
	if got := wantReason(t, err, "platform.tenant.cross_tenant_denied"); got != 404 {
		t.Fatalf("http code = %d, want 404（伪装不存在防枚举）", got)
	}
}

func TestSuspendInvalidTransition(t *testing.T) {
	e := newEnv()
	tt := provision(t, e, "acme")
	// 租户侧上下文操作自身；ACTIVE 直接注销（仅 SUSPENDED 可注销）→ 409 invalid_transition
	_, err := e.tenant.DeactivateTenant(ctxAs(tt), &tenantv1.DeactivateTenantRequest{TenantId: tt, Reason: "test"})
	if got := wantReason(t, err, "platform.tenant.invalid_transition"); got != 409 {
		t.Fatalf("http code = %d, want 409", got)
	}
}

// ---------- 组织接口 ----------

func TestOrgCreateAndUpdateConflict(t *testing.T) {
	e := newEnv()
	tid := provision(t, e, "acme")
	ctx := ctxAs(tid)

	o, err := e.org.CreateOrg(ctx, &orgv1.CreateOrgRequest{Name: "hq", DisplayName: "总部"})
	if err != nil {
		t.Fatal(err)
	}
	if o.TenantId != tid || o.DisplayName != "总部" {
		t.Fatalf("映射异常: %+v", o)
	}

	// 版本号不符 → 409 version_conflict
	_, err = e.org.UpdateOrg(ctx, &orgv1.UpdateOrgRequest{OrgId: o.OrgId, DisplayName: "总部2", Version: o.Audit.Version + 5})
	if got := wantReason(t, err, "platform.version_conflict"); got != 409 {
		t.Fatalf("http code = %d, want 409", got)
	}

	out, err := e.org.UpdateOrg(ctx, &orgv1.UpdateOrgRequest{OrgId: o.OrgId, DisplayName: "总部2", Version: o.Audit.Version})
	if err != nil {
		t.Fatal(err)
	}
	if out.DisplayName != "总部2" || out.Audit.Version != 2 {
		t.Fatalf("update = %+v", out)
	}
}

func TestOrgListPaging(t *testing.T) {
	e := newEnv()
	tid := provision(t, e, "acme")
	ctx := ctxAs(tid)
	for _, n := range []string{"a", "b", "c"} {
		if _, err := e.org.CreateOrg(ctx, &orgv1.CreateOrgRequest{Name: n}); err != nil {
			t.Fatal(err)
		}
	}
	// root + 3 = 4 条；page_size=2 → 首页 2 条 + 下一页游标
	p1, err := e.org.ListOrgs(ctx, &orgv1.ListOrgsRequest{Page: &commonv1.PageRequest{PageSize: 2}})
	if err != nil {
		t.Fatal(err)
	}
	if len(p1.Orgs) != 2 || p1.Page.NextPageToken == "" || p1.Page.TotalEstimate != 4 {
		t.Fatalf("page1 = %d items, token %q, estimate %d", len(p1.Orgs), p1.Page.NextPageToken, p1.Page.TotalEstimate)
	}
	p2, err := e.org.ListOrgs(ctx, &orgv1.ListOrgsRequest{Page: &commonv1.PageRequest{PageSize: 2, PageToken: p1.Page.NextPageToken}})
	if err != nil {
		t.Fatal(err)
	}
	if len(p2.Orgs) != 2 || p2.Page.NextPageToken != "" {
		t.Fatalf("page2 = %d items, token %q", len(p2.Orgs), p2.Page.NextPageToken)
	}
}

// ---------- 站点接口 ----------

func TestSiteMoveCycleDenied(t *testing.T) {
	e := newEnv()
	tid := provision(t, e, "acme")
	ctx := ctxAs(tid)

	parent, err := e.site.CreateSite(ctx, &sitev1.CreateSiteRequest{Name: "parent"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := e.site.CreateSite(ctx, &sitev1.CreateSiteRequest{Name: "child", ParentSiteId: parent.SiteId})
	if err != nil {
		t.Fatal(err)
	}
	// 移动 parent 至其后代 child 之下 → 422 cycle
	_, err = e.site.MoveSite(ctx, &sitev1.MoveSiteRequest{SiteId: parent.SiteId, NewParentSiteId: child.SiteId})
	if got := wantReason(t, err, "platform.site.cycle"); got != 422 {
		t.Fatalf("http code = %d, want 422", got)
	}
}

func TestSiteListPaging(t *testing.T) {
	e := newEnv()
	tid := provision(t, e, "acme")
	ctx := ctxAs(tid)
	for _, n := range []string{"s1", "s2", "s3"} {
		if _, err := e.site.CreateSite(ctx, &sitev1.CreateSiteRequest{Name: n}); err != nil {
			t.Fatal(err)
		}
	}
	p1, err := e.site.ListSites(ctx, &sitev1.ListSitesRequest{Page: &commonv1.PageRequest{PageSize: 2}})
	if err != nil {
		t.Fatal(err)
	}
	if len(p1.Sites) != 2 || p1.Page.NextPageToken == "" {
		t.Fatalf("page1 = %d items, token %q", len(p1.Sites), p1.Page.NextPageToken)
	}
	p2, err := e.site.ListSites(ctx, &sitev1.ListSitesRequest{Page: &commonv1.PageRequest{PageSize: 2, PageToken: p1.Page.NextPageToken}})
	if err != nil {
		t.Fatal(err)
	}
	if len(p2.Sites) != 2 || p2.Page.NextPageToken != "" {
		t.Fatalf("page2 = %d items, token %q", len(p2.Sites), p2.Page.NextPageToken)
	}
}
