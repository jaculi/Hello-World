// BC-P1 领域规则测试（纯 biz，内存替身；RLS/DB 实证见 internal/data 集成测试）。
package biz

import (
	"context"
	"testing"

	"github.com/jsl-aiot/platform/pkg/audit"
	"github.com/jsl-aiot/platform/pkg/errors"
	"github.com/jsl-aiot/platform/pkg/tenantcontext"
)

// nopSink 审计空汇。
type nopSink struct{}

func (nopSink) Emit(_ context.Context, _ audit.Event) error { return nil }

// env 测试环境。
type env struct {
	tx      *MemTransactor
	tenants *MemTenantRepo
	orgs    *MemOrgRepo
	sites   *MemSiteRepo
	events  *MemEventPublisher
	tenant  *TenantUseCase
	org     *OrgUseCase
	site    *SiteUseCase
}

func newEnv() *env {
	return newEnvWithPlans(Plan{ID: "plan-free", Name: "Free", DefaultIsolation: IsolationT3, Active: true})
}

func newEnvWithPlans(plans ...Plan) *env {
	tx := &MemTransactor{}
	tenants := NewMemTenantRepo()
	orgs := NewMemOrgRepo()
	sites := NewMemSiteRepo()
	events := &MemEventPublisher{}
	em := audit.NewEmitter(nopSink{})
	return &env{
		tx: tx, tenants: tenants, orgs: orgs, sites: sites, events: events,
		tenant: NewTenantUseCase(tx, tenants, NewMemPlanRepo(plans...), orgs, sites, events, em),
		org:    NewOrgUseCase(tx, orgs, em),
		site:   NewSiteUseCase(tx, sites, events, em),
	}
}

func ctxAsTenant(tenantID string) context.Context {
	return tenantcontext.With(context.Background(), tenantcontext.Context{TenantID: tenantID, Subject: "test:" + tenantID})
}

// provision 开通一个租户并返回（平台侧上下文）。
func provision(t *testing.T, e *env, name string) *Tenant {
	t.Helper()
	tt, err := e.tenant.CreateTenant(ctxAsTenant("platform"), CreateTenantInput{
		Name: name, PlanID: "plan-free",
	})
	if err != nil {
		t.Fatalf("provision %s: %v", name, err)
	}
	return tt
}

func code(t *testing.T, err error) (string, int) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	e, ok := errors.From(err)
	if !ok {
		t.Fatalf("expected platform error, got %v", err)
	}
	return e.Code, e.HTTPCode
}

// ---------- 租户开通 ----------

func TestCreateTenantSuccess(t *testing.T) {
	e := newEnv()
	tt := provision(t, e, "acme")
	if tt.Status != TenantStatusActive {
		t.Fatalf("status = %s, want active", tt.Status)
	}
	if tt.Isolation != IsolationT3 {
		t.Fatalf("isolation = %s, want t3_row (套餐默认)", tt.Isolation)
	}
	if tt.HomeRegion != "cn-north-1" {
		t.Fatalf("home_region 默认值缺失: %s", tt.HomeRegion)
	}
	if got, err := e.org.List(ctxAsTenant(tt.ID)); err != nil || len(got) != 1 {
		t.Fatalf("根组织未创建: %d, %v", len(got), err)
	}
	if got, err := e.site.List(ctxAsTenant(tt.ID)); err != nil || len(got) != 1 {
		t.Fatalf("根站点未创建: %d, %v", len(got), err)
	}
	if len(e.events.ByType(EventTenantProvisioned)) != 1 {
		t.Fatal("tenant.provisioned 事件缺失")
	}
	if len(e.events.ByType(EventSiteCreated)) != 1 {
		t.Fatal("site.created 事件缺失")
	}
	if tt.Audit.Version != 1 || tt.Audit.CreatedBy != "test:platform" {
		t.Fatalf("审计字段异常: %+v", tt.Audit)
	}
}

func TestCreateTenantNameInvalid(t *testing.T) {
	e := newEnv()
	for _, name := range []string{"ACME", "ab", "-abc", "a b", ""} {
		_, err := e.tenant.CreateTenant(ctxAsTenant("platform"), CreateTenantInput{Name: name, PlanID: "plan-free"})
		if got, _ := code(t, err); got != CodeTenantNameInvalid {
			t.Fatalf("name %q: got %s", name, got)
		}
	}
}

func TestCreateTenantNameTooShort(t *testing.T) {
	e := newEnv()
	_, err := e.tenant.CreateTenant(ctxAsTenant("platform"), CreateTenantInput{Name: "ab", PlanID: "plan-free"})
	if got, _ := code(t, err); got != CodeTenantNameInvalid {
		t.Fatalf("got %s", got)
	}
}

func TestCreateTenantNameTaken(t *testing.T) {
	e := newEnv()
	provision(t, e, "acme")
	_, err := e.tenant.CreateTenant(ctxAsTenant("platform"), CreateTenantInput{Name: "acme", PlanID: "plan-free"})
	if got, _ := code(t, err); got != CodeTenantNameTaken {
		t.Fatalf("got %s", got)
	}
}

func TestCreateTenantPlanNotFound(t *testing.T) {
	e := newEnv()
	_, err := e.tenant.CreateTenant(ctxAsTenant("platform"), CreateTenantInput{Name: "acme", PlanID: "nope"})
	if got, _ := code(t, err); got != CodePlanNotFound {
		t.Fatalf("got %s", got)
	}
}

func TestCreateTenantPlanInactive(t *testing.T) {
	e := newEnvWithPlans(Plan{ID: "plan-old", Name: "Old", DefaultIsolation: IsolationT3, Active: false})
	_, err := e.tenant.CreateTenant(ctxAsTenant("platform"), CreateTenantInput{Name: "acme", PlanID: "plan-old"})
	if got, _ := code(t, err); got != CodePlanInactive {
		t.Fatalf("got %s", got)
	}
}

func TestCreateTenantIsolationUnsupported(t *testing.T) {
	e := newEnv()
	_, err := e.tenant.CreateTenant(ctxAsTenant("platform"), CreateTenantInput{
		Name: "acme", PlanID: "plan-free", Isolation: IsolationT1,
	})
	if got, _ := code(t, err); got != CodeTenantIsolationUnsupported {
		t.Fatalf("got %s", got)
	}
}

// ---------- 租户状态机 ----------

func TestSuspendSuccess(t *testing.T) {
	e := newEnv()
	tt := provision(t, e, "acme")
	out, err := e.tenant.SuspendTenant(ctxAsTenant(tt.ID), tt.ID, "arrears")
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != TenantStatusSuspended {
		t.Fatalf("status = %s", out.Status)
	}
	if len(e.events.ByType(EventTenantSuspended)) != 1 {
		t.Fatal("suspended 事件缺失")
	}
}

func TestSuspendAlreadySuspended(t *testing.T) {
	e := newEnv()
	tt := provision(t, e, "acme")
	if _, err := e.tenant.SuspendTenant(ctxAsTenant(tt.ID), tt.ID, "x"); err != nil {
		t.Fatal(err)
	}
	_, err := e.tenant.SuspendTenant(ctxAsTenant(tt.ID), tt.ID, "y")
	if got, _ := code(t, err); got != CodeTenantInvalidTransition {
		t.Fatalf("got %s", got)
	}
}

func TestSuspendDeactivated(t *testing.T) {
	e := newEnv()
	tt := provision(t, e, "acme")
	_, _ = e.tenant.SuspendTenant(ctxAsTenant(tt.ID), tt.ID, "x")
	_, _ = e.tenant.DeactivateTenant(ctxAsTenant(tt.ID), tt.ID, "done")
	_, err := e.tenant.SuspendTenant(ctxAsTenant(tt.ID), tt.ID, "z")
	if got, _ := code(t, err); got != CodeTenantInvalidTransition {
		t.Fatalf("got %s", got)
	}
}

func TestReactivateSuccess(t *testing.T) {
	e := newEnv()
	tt := provision(t, e, "acme")
	_, _ = e.tenant.SuspendTenant(ctxAsTenant(tt.ID), tt.ID, "x")
	out, err := e.tenant.ReactivateTenant(ctxAsTenant(tt.ID), tt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != TenantStatusActive {
		t.Fatalf("status = %s", out.Status)
	}
	if len(e.events.ByType(EventTenantReactivated)) != 1 {
		t.Fatal("reactivated 事件缺失")
	}
}

func TestReactivateOnActive(t *testing.T) {
	e := newEnv()
	tt := provision(t, e, "acme")
	_, err := e.tenant.ReactivateTenant(ctxAsTenant(tt.ID), tt.ID)
	if got, _ := code(t, err); got != CodeTenantInvalidTransition {
		t.Fatalf("got %s", got)
	}
}

func TestDeactivateFromSuspended(t *testing.T) {
	e := newEnv()
	tt := provision(t, e, "acme")
	_, _ = e.tenant.SuspendTenant(ctxAsTenant(tt.ID), tt.ID, "x")
	out, err := e.tenant.DeactivateTenant(ctxAsTenant(tt.ID), tt.ID, "terminate")
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != TenantStatusDeactivated {
		t.Fatalf("status = %s", out.Status)
	}
	if len(e.events.ByType(EventTenantDeactivated)) != 1 {
		t.Fatal("deactivated 事件缺失")
	}
}

func TestDeactivateFromActive(t *testing.T) {
	e := newEnv()
	tt := provision(t, e, "acme")
	_, err := e.tenant.DeactivateTenant(ctxAsTenant(tt.ID), tt.ID, "x")
	if got, _ := code(t, err); got != CodeTenantInvalidTransition {
		t.Fatalf("got %s", got)
	}
}

// ---------- 越界与查询 ----------

func TestGetTenantCrossTenantDenied(t *testing.T) {
	e := newEnv()
	a := provision(t, e, "acme")
	b := provision(t, e, "beta")
	_, err := e.tenant.GetTenant(ctxAsTenant(a.ID), b.ID)
	got, httpCode := code(t, err)
	if got != CodeTenantCrossDenied {
		t.Fatalf("got %s", got)
	}
	// 防枚举：伪装成不存在
	if httpCode != 404 {
		t.Fatalf("http = %d, want 404", httpCode)
	}
}

func TestGetTenantSelfOK(t *testing.T) {
	e := newEnv()
	tt := provision(t, e, "acme")
	out, err := e.tenant.GetTenant(ctxAsTenant(tt.ID), tt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if out.ID != tt.ID {
		t.Fatal("mismatch")
	}
}

func TestGetTenantNotFound(t *testing.T) {
	e := newEnv()
	_, err := e.tenant.GetTenant(ctxAsTenant("ghost"), "ghost")
	if got, _ := code(t, err); got != CodeTenantNotFound {
		t.Fatalf("got %s", got)
	}
}

// ---------- 行业装配 ----------

func TestBindAssemblySuccess(t *testing.T) {
	e := newEnv()
	tt := provision(t, e, "acme")
	a, err := e.tenant.BindIndustryAssembly(ctxAsTenant(tt.ID), tt.ID, "industry.building", "1.0.0", true)
	if err != nil {
		t.Fatal(err)
	}
	if a.ModuleID != "industry.building" || !a.Enabled {
		t.Fatalf("assembly = %+v", a)
	}
	if len(e.events.ByType(EventIndustryAssemblyBound)) != 1 {
		t.Fatal("assembly_bound 事件缺失")
	}
	list, err := e.tenant.ListIndustryAssemblies(ctxAsTenant(tt.ID), tt.ID)
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %v, %v", list, err)
	}
}

func TestBindAssemblyUpsert(t *testing.T) {
	e := newEnv()
	tt := provision(t, e, "acme")
	_, _ = e.tenant.BindIndustryAssembly(ctxAsTenant(tt.ID), tt.ID, "industry.building", "1.0.0", true)
	_, _ = e.tenant.BindIndustryAssembly(ctxAsTenant(tt.ID), tt.ID, "industry.building", "1.1.0", false)
	list, _ := e.tenant.ListIndustryAssemblies(ctxAsTenant(tt.ID), tt.ID)
	if len(list) != 1 || list[0].ModuleVersion != "1.1.0" || list[0].Enabled {
		t.Fatalf("upsert 语义未生效: %+v", list)
	}
}

func TestBindAssemblyOnDeactivated(t *testing.T) {
	e := newEnv()
	tt := provision(t, e, "acme")
	_, _ = e.tenant.SuspendTenant(ctxAsTenant(tt.ID), tt.ID, "x")
	_, _ = e.tenant.DeactivateTenant(ctxAsTenant(tt.ID), tt.ID, "y")
	_, err := e.tenant.BindIndustryAssembly(ctxAsTenant(tt.ID), tt.ID, "industry.building", "1.0.0", true)
	if got, _ := code(t, err); got != CodeTenantDeactivated {
		t.Fatalf("got %s", got)
	}
}

func TestBindAssemblyCrossTenant(t *testing.T) {
	e := newEnv()
	a := provision(t, e, "acme")
	b := provision(t, e, "beta")
	_, err := e.tenant.BindIndustryAssembly(ctxAsTenant(a.ID), b.ID, "industry.building", "1.0.0", true)
	if got, _ := code(t, err); got != CodeTenantCrossDenied {
		t.Fatalf("got %s", got)
	}
}

func TestBindAssemblyInvalidModuleID(t *testing.T) {
	e := newEnv()
	tt := provision(t, e, "acme")
	_, err := e.tenant.BindIndustryAssembly(ctxAsTenant(tt.ID), tt.ID, "BAD MODULE", "1.0.0", true)
	if got, _ := code(t, err); got != CodeTenantNameInvalid {
		t.Fatalf("got %s", got)
	}
}

// ---------- 组织 ----------

func TestOrgCreateRootAndNested(t *testing.T) {
	e := newEnv()
	tt := provision(t, e, "acme")
	ctx := ctxAsTenant(tt.ID)
	root, err := e.org.Create(ctx, "", "hq", "总部")
	if err != nil {
		t.Fatal(err)
	}
	child, err := e.org.Create(ctx, root.ID, "rd", "研发")
	if err != nil {
		t.Fatal(err)
	}
	if child.ParentOrgID != root.ID {
		t.Fatalf("parent = %s", child.ParentOrgID)
	}
	if child.Audit.CreatedBy != "test:"+tt.ID {
		t.Fatalf("审计主体异常: %s", child.Audit.CreatedBy)
	}
}

func TestOrgCreateParentNotFound(t *testing.T) {
	e := newEnv()
	tt := provision(t, e, "acme")
	_, err := e.org.Create(ctxAsTenant(tt.ID), "nope", "hq", "")
	if got, _ := code(t, err); got != CodeOrgNotFound {
		t.Fatalf("got %s", got)
	}
}

func TestOrgCreateNameInvalid(t *testing.T) {
	e := newEnv()
	tt := provision(t, e, "acme")
	_, err := e.org.Create(ctxAsTenant(tt.ID), "", "BAD NAME", "")
	if got, _ := code(t, err); got != CodeOrgNameInvalid {
		t.Fatalf("got %s", got)
	}
}

func TestOrgCreateNameTaken(t *testing.T) {
	e := newEnv()
	tt := provision(t, e, "acme")
	ctx := ctxAsTenant(tt.ID)
	if _, err := e.org.Create(ctx, "", "hq", ""); err != nil {
		t.Fatal(err)
	}
	_, err := e.org.Create(ctx, "", "hq", "")
	if got, _ := code(t, err); got != CodeOrgNameTaken {
		t.Fatalf("got %s", got)
	}
}

func TestOrgUpdateOptimisticLock(t *testing.T) {
	e := newEnv()
	tt := provision(t, e, "acme")
	ctx := ctxAsTenant(tt.ID)
	o, _ := e.org.Create(ctx, "", "hq", "")
	_, err := e.org.Update(ctx, OrgUpdateInput{OrgID: o.ID, DisplayName: "总部2", Version: o.Audit.Version + 5})
	if got, _ := code(t, err); got != CodeVersionConflict {
		t.Fatalf("got %s", got)
	}
	out, err := e.org.Update(ctx, OrgUpdateInput{OrgID: o.ID, DisplayName: "总部2", Version: o.Audit.Version})
	if err != nil || out.DisplayName != "总部2" || out.Audit.Version != 2 {
		t.Fatalf("update = %+v, %v", out, err)
	}
}

// ---------- 站点 ----------

func TestSiteCreateAndEvent(t *testing.T) {
	e := newEnv()
	tt := provision(t, e, "acme")
	ctx := ctxAsTenant(tt.ID)
	before := len(e.events.ByType(EventSiteCreated))
	s, err := e.site.Create(ctx, "", "plant-1", "一号厂区", "factory")
	if err != nil {
		t.Fatal(err)
	}
	if s.SiteType != "factory" {
		t.Fatalf("type = %s", s.SiteType)
	}
	if len(e.events.ByType(EventSiteCreated)) != before+1 {
		t.Fatal("site.created 事件缺失")
	}
}

func TestSiteCreateParentMissing(t *testing.T) {
	e := newEnv()
	tt := provision(t, e, "acme")
	_, err := e.site.Create(ctxAsTenant(tt.ID), "nope", "plant-1", "", "factory")
	if got, _ := code(t, err); got != CodeSiteNotFound {
		t.Fatalf("got %s", got)
	}
}

func TestSiteMoveSelfCycle(t *testing.T) {
	e := newEnv()
	tt := provision(t, e, "acme")
	ctx := ctxAsTenant(tt.ID)
	s, _ := e.site.Create(ctx, "", "plant-1", "", "factory")
	_, err := e.site.Move(ctx, s.ID, s.ID)
	if got, _ := code(t, err); got != CodeSiteCycle {
		t.Fatalf("got %s", got)
	}
}

func TestSiteMoveDescendantCycle(t *testing.T) {
	e := newEnv()
	tt := provision(t, e, "acme")
	ctx := ctxAsTenant(tt.ID)
	parent, _ := e.site.Create(ctx, "", "parent", "", "")
	child, _ := e.site.Create(ctx, parent.ID, "child", "", "")
	// 把 parent 移到其自身后代 child 之下 → 拒绝
	_, err := e.site.Move(ctx, parent.ID, child.ID)
	if got, _ := code(t, err); got != CodeSiteCycle {
		t.Fatalf("got %s", got)
	}
}

func TestSiteMoveOK(t *testing.T) {
	e := newEnv()
	tt := provision(t, e, "acme")
	ctx := ctxAsTenant(tt.ID)
	a, _ := e.site.Create(ctx, "", "a", "", "")
	b, _ := e.site.Create(ctx, "", "b", "", "")
	c, _ := e.site.Create(ctx, a.ID, "c", "", "")
	out, err := e.site.Move(ctx, c.ID, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if out.ParentSiteID != b.ID {
		t.Fatalf("parent = %s", out.ParentSiteID)
	}
	// 移回根
	out, err = e.site.Move(ctx, c.ID, "")
	if err != nil || out.ParentSiteID != "" {
		t.Fatalf("move to root: %+v, %v", out, err)
	}
}

func TestSiteMoveCrossTenantInvisible(t *testing.T) {
	e := newEnv()
	a := provision(t, e, "acme")
	b := provision(t, e, "beta")
	sb, _ := e.site.Create(ctxAsTenant(b.ID), "", "b-site", "", "")
	// 租户 A 的会话看不到租户 B 的站点（替身以 not_found 模拟 RLS 不可见）
	_, err := e.site.Move(ctxAsTenant(a.ID), sb.ID, "")
	if got, _ := code(t, err); got != CodeSiteNotFound {
		t.Fatalf("got %s", got)
	}
}

func TestSiteUpdateOptimisticLock(t *testing.T) {
	e := newEnv()
	tt := provision(t, e, "acme")
	ctx := ctxAsTenant(tt.ID)
	s, _ := e.site.Create(ctx, "", "plant-1", "", "factory")
	_, err := e.site.Update(ctx, SiteUpdateInput{SiteID: s.ID, DisplayName: "二号厂区", Version: s.Audit.Version + 1})
	if got, _ := code(t, err); got != CodeVersionConflict {
		t.Fatalf("got %s", got)
	}
}

// ---------- 跨租户组织不可见 ----------

func TestOrgCrossTenantInvisible(t *testing.T) {
	e := newEnv()
	a := provision(t, e, "acme")
	b := provision(t, e, "beta")
	ob, _ := e.org.Create(ctxAsTenant(b.ID), "", "b-hq", "")
	_, err := e.org.Get(ctxAsTenant(a.ID), ob.ID)
	if got, _ := code(t, err); got != CodeOrgNotFound {
		t.Fatalf("got %s", got)
	}
}

func TestSubjectFallback(t *testing.T) {
	if got := subject(context.Background()); got != "system:unknown" {
		t.Fatalf("got %s", got)
	}
	if got := subject(ctxAsTenant("acme")); got != "test:acme" {
		t.Fatalf("got %s", got)
	}
}
