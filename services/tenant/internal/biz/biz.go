// BC-P1 核心聚合与租户用例（BP-02 §3.1）。
// 领域规则：
//   - 租户状态机：创建即 ACTIVE；ACTIVE→SUSPENDED；SUSPENDED→ACTIVE；注销仅可自 SUSPENDED 进入；
//   - 租户/组织/站点归属链 tenant → org → site（BP-03 §4.1）强制；
//   - 开通在同事务内完成：租户 + 默认订阅 + 根组织 + 根站点 + 事件（ADR-0005）。
package biz

import (
	"context"
	"regexp"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/jsl-aiot/platform/pkg/audit"
	"github.com/jsl-aiot/platform/pkg/errors"
	"github.com/jsl-aiot/platform/pkg/tenantcontext"
)

// ---------- 聚合与值 ----------

// TenantStatus 租户生命周期状态。
type TenantStatus string

const (
	TenantStatusActive      TenantStatus = "active"
	TenantStatusSuspended   TenantStatus = "suspended"
	TenantStatusDeactivated TenantStatus = "deactivated"
)

// IsolationLevel 租户隔离级别（BP-03 §3.1；本期仅 T3 生效）。
type IsolationLevel string

const (
	IsolationT1 IsolationLevel = "t1_dedicated"
	IsolationT2 IsolationLevel = "t2_schema"
	IsolationT3 IsolationLevel = "t3_row"
)

// Audit 公共审计四件套 + 乐观锁版本（BP-03 §4.1）。
type Audit struct {
	CreatedAt time.Time
	CreatedBy string
	UpdatedAt time.Time
	UpdatedBy string
	Version   int64
}

// Tenant 租户聚合根。
type Tenant struct {
	ID          string
	Name        string
	DisplayName string
	Status      TenantStatus
	Isolation   IsolationLevel
	PlanID      string
	HomeRegion  string
	Audit       Audit
}

// Subscription 默认套餐订阅（开通即建立；计费细化随 BC-P3）。
type Subscription struct {
	ID       string
	TenantID string
	PlanID   string
	Status   string
	Audit    Audit
}

// IndustryAssembly 行业模块装配记录（BP-02 §6：订阅即装配）。
type IndustryAssembly struct {
	TenantID      string
	ModuleID      string
	ModuleVersion string
	Enabled       bool
	Audit         Audit
}

// Organization 组织。
type Organization struct {
	ID          string
	TenantID    string
	ParentOrgID string
	Name        string
	DisplayName string
	Audit       Audit
}

// Site 站点（行业中性的核心空间概念，BP-01 §2.2）。
type Site struct {
	ID           string
	TenantID     string
	ParentSiteID string
	Name         string
	DisplayName  string
	SiteType     string
	Audit        Audit
}

// Plan 套餐。
type Plan struct {
	ID               string
	Name             string
	DefaultIsolation IsolationLevel
	Active           bool
}

// tenantNameRe 租户外部标识：小写字母数字与连字符，3-63 位。
var tenantNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{2,62}$`)

// objectNameRe 组织/站点名称：小写字母数字与连字符/下划线，1-64 位。
var objectNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// moduleIDRe 行业模块 ID：允许点分命名空间（如 industry.building）。
var moduleIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

// subject 提取操作主体（审计与字段填充用）。
func subject(ctx context.Context) string {
	if tc, ok := tenantcontext.From(ctx); ok && tc.Subject != "" {
		return tc.Subject
	}
	return "system:unknown"
}

// guardCrossTenant 跨租户访问闸（BP-05 §4.2 第二道闸）：
// 调用者租户与目标租户不一致即拒，伪装成“不存在”防枚举。
func guardCrossTenant(ctx context.Context, tenantID string) error {
	if tc, ok := tenantcontext.From(ctx); ok && tc.TenantID != "" && tc.TenantID != tenantID {
		return errors.New(CodeTenantCrossDenied, statusNotFound).WithParam("tenant_id", tenantID)
	}
	return nil
}

// ---------- 租户用例 ----------

// TenantUseCase 租户用例。
type TenantUseCase struct {
	tx      Transactor
	tenants TenantRepo
	plans   PlanRepo
	orgs    OrgRepo
	sites   SiteRepo
	events  EventPublisher
	audit   *audit.Emitter

	newID func() string
	now   func() time.Time
}

// NewTenantUseCase 装配租户用例。
func NewTenantUseCase(
	tx Transactor, tenants TenantRepo, plans PlanRepo,
	orgs OrgRepo, sites SiteRepo, events EventPublisher, em *audit.Emitter,
) *TenantUseCase {
	return &TenantUseCase{
		tx: tx, tenants: tenants, plans: plans, orgs: orgs, sites: sites,
		events: events, audit: em,
		newID: func() string { return ulid.Make().String() },
		now:   time.Now,
	}
}

// CreateTenantInput 开通租户入参。
type CreateTenantInput struct {
	Name        string
	DisplayName string
	// Isolation 为空时取套餐默认隔离级别。
	Isolation  IsolationLevel
	PlanID     string
	HomeRegion string
}

// CreateTenant 开通租户（Provision 语义，创建即 ACTIVE）。
// 同事务完成：租户 + 默认订阅 + 根组织 + 根站点 + tenant.provisioned / site.created 事件。
func (uc *TenantUseCase) CreateTenant(ctx context.Context, in CreateTenantInput) (*Tenant, error) {
	if !tenantNameRe.MatchString(in.Name) {
		return nil, errors.New(CodeTenantNameInvalid, statusBadRequest).WithParam("name", in.Name)
	}
	tid := uc.newID()
	var created *Tenant
	err := uc.tx.WithinTenantTx(ctx, tid, func(ctx context.Context, tx Tx) error {
		plan, err := uc.plans.GetByID(ctx, tx, in.PlanID)
		if err != nil {
			return err
		}
		if !plan.Active {
			return errors.New(CodePlanInactive, statusConflict).WithParam("plan_id", in.PlanID)
		}
		iso := in.Isolation
		if iso == "" {
			iso = plan.DefaultIsolation
		}
		if iso != IsolationT3 {
			// 本期仅 T3 路由生效（pkg/dataaccess RouteTable 约束）
			return errors.New(CodeTenantIsolationUnsupported, statusUnprocessable).WithParam("isolation", string(iso))
		}
		now := uc.now().UTC()
		au := Audit{CreatedAt: now, CreatedBy: subject(ctx), UpdatedAt: now, UpdatedBy: subject(ctx), Version: 1}
		t := &Tenant{
			ID: tid, Name: in.Name,
			DisplayName: firstNonEmpty(in.DisplayName, in.Name),
			Status:      TenantStatusActive, Isolation: iso,
			PlanID: in.PlanID, HomeRegion: firstNonEmpty(in.HomeRegion, "cn-north-1"),
			Audit: au,
		}
		sub := &Subscription{ID: uc.newID(), TenantID: tid, PlanID: in.PlanID, Status: "active", Audit: au}
		org := &Organization{ID: uc.newID(), TenantID: tid, Name: "root", DisplayName: "根组织", Audit: au}
		site := &Site{ID: uc.newID(), TenantID: tid, Name: "root", DisplayName: "根站点", SiteType: "root", Audit: au}
		if err := uc.tenants.Insert(ctx, tx, t, sub, nil); err != nil {
			return err // 名称全局冲突由唯一约束映射为 CodeTenantNameTaken
		}
		if err := uc.orgs.Insert(ctx, tx, org); err != nil {
			return err
		}
		if err := uc.sites.Insert(ctx, tx, site); err != nil {
			return err
		}
		if err := uc.events.Enqueue(ctx, tx,
			newEnvelope(ctx, tid, EventTenantProvisioned, tenantProvisionedPayload(t, org.ID, site.ID))); err != nil {
			return err
		}
		if err := uc.events.Enqueue(ctx, tx,
			newEnvelope(ctx, tid, EventSiteCreated, siteCreatedPayload(site))); err != nil {
			return err
		}
		created = t
		return nil
	})
	if err != nil {
		return nil, err
	}
	_ = uc.audit.Emit(ctx, "platform.tenant.provision", created.ID, audit.ResultSuccess,
		map[string]string{"plan_id": in.PlanID, "isolation": string(created.Isolation)})
	return created, nil
}

// GetTenant 查询租户。
func (uc *TenantUseCase) GetTenant(ctx context.Context, tenantID string) (*Tenant, error) {
	if err := guardCrossTenant(ctx, tenantID); err != nil {
		return nil, err
	}
	var t *Tenant
	err := uc.tx.WithinTenantTx(ctx, tenantID, func(ctx context.Context, tx Tx) error {
		var err error
		t, err = uc.tenants.GetByID(ctx, tx, tenantID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return t, nil
}

// SuspendTenant 冻结租户（ACTIVE→SUSPENDED），产生写冻结（BP-07 计费冻结链前置）。
func (uc *TenantUseCase) SuspendTenant(ctx context.Context, tenantID, reason string) (*Tenant, error) {
	return uc.transition(ctx, tenantID, TenantStatusActive, TenantStatusSuspended, reason)
}

// ReactivateTenant 恢复租户（SUSPENDED→ACTIVE）。
func (uc *TenantUseCase) ReactivateTenant(ctx context.Context, tenantID string) (*Tenant, error) {
	return uc.transition(ctx, tenantID, TenantStatusSuspended, TenantStatusActive, "")
}

// DeactivateTenant 注销租户（仅 SUSPENDED→DEACTIVATED；保留期物理清理随 BP-07）。
func (uc *TenantUseCase) DeactivateTenant(ctx context.Context, tenantID, reason string) (*Tenant, error) {
	return uc.transition(ctx, tenantID, TenantStatusSuspended, TenantStatusDeactivated, reason)
}

// transition 状态迁移通用路径：校验 → 更新 → 事件，同事务原子完成。
func (uc *TenantUseCase) transition(ctx context.Context, tenantID string, from, to TenantStatus, reason string) (*Tenant, error) {
	if err := guardCrossTenant(ctx, tenantID); err != nil {
		return nil, err
	}
	var updated *Tenant
	err := uc.tx.WithinTenantTx(ctx, tenantID, func(ctx context.Context, tx Tx) error {
		t, err := uc.tenants.GetByID(ctx, tx, tenantID)
		if err != nil {
			return err
		}
		if t.Status != from {
			return errors.New(CodeTenantInvalidTransition, statusConflict).
				WithParam("from", string(t.Status)).WithParam("to", string(to))
		}
		expectedVersion := t.Audit.Version
		t.Status = to
		t.Audit.UpdatedAt = uc.now().UTC()
		t.Audit.UpdatedBy = subject(ctx)
		t.Audit.Version++
		if err := uc.tenants.UpdateStatus(ctx, tx, t, expectedVersion); err != nil {
			return err
		}
		var payload isEnvelopePayload
		switch to {
		case TenantStatusSuspended:
			payload = suspendedPayload(t, reason)
		case TenantStatusActive:
			payload = reactivatedPayload(t)
		default:
			payload = deactivatedPayload(t, reason)
		}
		if err := uc.events.Enqueue(ctx, tx, newEnvelope(ctx, tenantID, eventTypeOf(to), payload)); err != nil {
			return err
		}
		updated = t
		return nil
	})
	if err != nil {
		return nil, err
	}
	_ = uc.audit.Emit(ctx, "platform.tenant."+string(to), tenantID, audit.ResultSuccess,
		map[string]string{"reason": reason})
	return updated, nil
}

// BindIndustryAssembly 绑定/更新行业模块装配（BP-02 §6 装配三重校验的租户侧入口）。
func (uc *TenantUseCase) BindIndustryAssembly(ctx context.Context, tenantID, moduleID, moduleVersion string, enabled bool) (*IndustryAssembly, error) {
	if err := guardCrossTenant(ctx, tenantID); err != nil {
		return nil, err
	}
	if !moduleIDRe.MatchString(moduleID) {
		return nil, errors.New(CodeTenantNameInvalid, statusBadRequest).WithParam("module_id", moduleID)
	}
	var out *IndustryAssembly
	err := uc.tx.WithinTenantTx(ctx, tenantID, func(ctx context.Context, tx Tx) error {
		t, err := uc.tenants.GetByID(ctx, tx, tenantID)
		if err != nil {
			return err
		}
		if t.Status == TenantStatusDeactivated {
			return errors.New(CodeTenantDeactivated, statusConflict)
		}
		now := uc.now().UTC()
		a := IndustryAssembly{
			TenantID: tenantID, ModuleID: moduleID, ModuleVersion: moduleVersion, Enabled: enabled,
			Audit: Audit{CreatedAt: now, CreatedBy: subject(ctx), UpdatedAt: now, UpdatedBy: subject(ctx), Version: 1},
		}
		if err := uc.tenants.UpsertAssembly(ctx, tx, a); err != nil {
			return err
		}
		out = &a
		return uc.events.Enqueue(ctx, tx, newEnvelope(ctx, tenantID, EventIndustryAssemblyBound,
			assemblyBoundPayload(tenantID, moduleID, moduleVersion)))
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ListIndustryAssemblies 装配清单。
func (uc *TenantUseCase) ListIndustryAssemblies(ctx context.Context, tenantID string) ([]IndustryAssembly, error) {
	if err := guardCrossTenant(ctx, tenantID); err != nil {
		return nil, err
	}
	var out []IndustryAssembly
	err := uc.tx.WithinTenantTx(ctx, tenantID, func(ctx context.Context, tx Tx) error {
		var err error
		out, err = uc.tenants.ListAssemblies(ctx, tx, tenantID)
		return err
	})
	return out, err
}

// ---------- 小工具 ----------

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
