// 组织用例：组织树管理（根组织由租户开通时初始化；新组织挂在根或既有组织之下）。
package biz

import (
	"context"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/jsl-aiot/platform/pkg/audit"
	"github.com/jsl-aiot/platform/pkg/errors"
	"github.com/jsl-aiot/platform/pkg/tenantcontext"
)

// OrgUseCase 组织用例。
type OrgUseCase struct {
	tx    Transactor
	orgs  OrgRepo
	audit *audit.Emitter

	newID func() string
	now   func() time.Time
}

// NewOrgUseCase 装配组织用例。
func NewOrgUseCase(tx Transactor, orgs OrgRepo, em *audit.Emitter) *OrgUseCase {
	return &OrgUseCase{
		tx: tx, orgs: orgs, audit: em,
		newID: func() string { return ulid.Make().String() },
		now:   time.Now,
	}
}

// Create 创建组织。parentOrgID 为空表示挂在租户根。
func (uc *OrgUseCase) Create(ctx context.Context, parentOrgID, name, displayName string) (*Organization, error) {
	tc, _ := tenantcontext.From(ctx)
	if tc.TenantID == "" {
		return nil, errors.New(tenantcontext.CodeTenantMissing, statusBadRequest) // fail-closed：无租户上下文即拒
	}
	if !objectNameRe.MatchString(name) {
		return nil, errors.New(CodeOrgNameInvalid, statusBadRequest).WithParam("name", name)
	}
	var out *Organization
	err := uc.tx.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		if parentOrgID != "" {
			if _, err := uc.orgs.GetByID(ctx, tx, parentOrgID); err != nil {
				return err // 不存在或跨租户（RLS 不可见）统一为 not_found
			}
		}
		now := uc.now().UTC()
		o := &Organization{
			ID: uc.newID(), TenantID: tc.TenantID, ParentOrgID: parentOrgID,
			Name: name, DisplayName: firstNonEmpty(displayName, name),
			Audit: Audit{CreatedAt: now, CreatedBy: subject(ctx), UpdatedAt: now, UpdatedBy: subject(ctx), Version: 1},
		}
		if err := uc.orgs.Insert(ctx, tx, o); err != nil {
			return err // (tenant,name) 唯一约束冲突映射为 CodeOrgNameTaken
		}
		out = o
		return nil
	})
	if err != nil {
		return nil, err
	}
	_ = uc.audit.Emit(ctx, "platform.org.created", out.ID, audit.ResultSuccess, nil)
	return out, nil
}

// Get 查询组织。
func (uc *OrgUseCase) Get(ctx context.Context, orgID string) (*Organization, error) {
	var out *Organization
	err := uc.tx.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		var err error
		out, err = uc.orgs.GetByID(ctx, tx, orgID)
		return err
	})
	return out, err
}

// OrgUpdateInput 组织更新入参（乐观锁）。
type OrgUpdateInput struct {
	OrgID       string
	DisplayName string
	// Version 预期的当前版本号。
	Version int64
}

// Update 更新组织（乐观锁冲突返回 CodeVersionConflict）。
func (uc *OrgUseCase) Update(ctx context.Context, in OrgUpdateInput) (*Organization, error) {
	var out *Organization
	err := uc.tx.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		o, err := uc.orgs.GetByID(ctx, tx, in.OrgID)
		if err != nil {
			return err
		}
		if o.Audit.Version != in.Version {
			return errors.New(CodeVersionConflict, statusConflict).WithParam("expected", intToStr(in.Version))
		}
		o.DisplayName = firstNonEmpty(in.DisplayName, o.DisplayName)
		o.Audit.UpdatedAt = uc.now().UTC()
		o.Audit.UpdatedBy = subject(ctx)
		o.Audit.Version++
		if err := uc.orgs.Update(ctx, tx, o, in.Version); err != nil {
			return err
		}
		out = o
		return nil
	})
	return out, err
}

// List 组织清单（RLS 界定当前租户可见范围）。
func (uc *OrgUseCase) List(ctx context.Context) ([]*Organization, error) {
	var out []*Organization
	err := uc.tx.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		var err error
		out, err = uc.orgs.List(ctx, tx)
		return err
	})
	return out, err
}
