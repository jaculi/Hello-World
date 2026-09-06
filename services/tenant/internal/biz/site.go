// 站点用例：站点树管理（BP-01 §2.2）。
// 领域规则：移动子树禁止成环——目标父站点不得为自身或自身的后代。
package biz

import (
	"context"
	"strconv"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/jsl-aiot/platform/pkg/audit"
	"github.com/jsl-aiot/platform/pkg/errors"
	"github.com/jsl-aiot/platform/pkg/tenantcontext"
)

func intToStr(v int64) string { return strconv.FormatInt(v, 10) }

// SiteUseCase 站点用例。
type SiteUseCase struct {
	tx     Transactor
	sites  SiteRepo
	events EventPublisher
	audit  *audit.Emitter

	newID func() string
	now   func() time.Time
}

// NewSiteUseCase 装配站点用例。
func NewSiteUseCase(tx Transactor, sites SiteRepo, events EventPublisher, em *audit.Emitter) *SiteUseCase {
	return &SiteUseCase{
		tx: tx, sites: sites, events: events, audit: em,
		newID: func() string { return ulid.Make().String() },
		now:   time.Now,
	}
}

// Create 创建站点。parentSiteID 为空表示租户根层级。
func (uc *SiteUseCase) Create(ctx context.Context, parentSiteID, name, displayName, siteType string) (*Site, error) {
	tc, _ := tenantcontext.From(ctx)
	if tc.TenantID == "" {
		return nil, errors.New(tenantcontext.CodeTenantMissing, statusBadRequest) // fail-closed：无租户上下文即拒
	}
	if !objectNameRe.MatchString(name) {
		return nil, errors.New(CodeSiteNameInvalid, statusBadRequest).WithParam("name", name)
	}
	var out *Site
	err := uc.tx.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		if parentSiteID != "" {
			if _, err := uc.sites.GetByID(ctx, tx, parentSiteID); err != nil {
				return err
			}
		}
		now := uc.now().UTC()
		s := &Site{
			ID: uc.newID(), TenantID: tc.TenantID, ParentSiteID: parentSiteID,
			Name: name, DisplayName: firstNonEmpty(displayName, name), SiteType: siteType,
			Audit: Audit{CreatedAt: now, CreatedBy: subject(ctx), UpdatedAt: now, UpdatedBy: subject(ctx), Version: 1},
		}
		if err := uc.sites.Insert(ctx, tx, s); err != nil {
			return err
		}
		if err := uc.events.Enqueue(ctx, tx, newEnvelope(ctx, s.TenantID, EventSiteCreated, siteCreatedPayload(s))); err != nil {
			return err
		}
		out = s
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Get 查询站点。
func (uc *SiteUseCase) Get(ctx context.Context, siteID string) (*Site, error) {
	var out *Site
	err := uc.tx.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		var err error
		out, err = uc.sites.GetByID(ctx, tx, siteID)
		return err
	})
	return out, err
}

// SiteUpdateInput 站点更新入参（乐观锁）。
type SiteUpdateInput struct {
	SiteID      string
	DisplayName string
	SiteType    string
	// Version 预期的当前版本号。
	Version int64
}

// Update 更新站点。
func (uc *SiteUseCase) Update(ctx context.Context, in SiteUpdateInput) (*Site, error) {
	var out *Site
	err := uc.tx.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		s, err := uc.sites.GetByID(ctx, tx, in.SiteID)
		if err != nil {
			return err
		}
		if s.Audit.Version != in.Version {
			return errors.New(CodeVersionConflict, statusConflict).WithParam("expected", intToStr(in.Version))
		}
		s.DisplayName = firstNonEmpty(in.DisplayName, s.DisplayName)
		s.SiteType = firstNonEmpty(in.SiteType, s.SiteType)
		s.Audit.UpdatedAt = uc.now().UTC()
		s.Audit.UpdatedBy = subject(ctx)
		s.Audit.Version++
		if err := uc.sites.Update(ctx, tx, s, in.Version); err != nil {
			return err
		}
		out = s
		return nil
	})
	return out, err
}

// Move 移动站点子树。newParentSiteID 为空表示移动到租户根层级。
// 领域规则：目标父不得为自身或自身后代（循环依赖拒绝）。
func (uc *SiteUseCase) Move(ctx context.Context, siteID, newParentSiteID string) (*Site, error) {
	var out *Site
	err := uc.tx.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		s, err := uc.sites.GetByID(ctx, tx, siteID)
		if err != nil {
			return err
		}
		if newParentSiteID != "" {
			if newParentSiteID == siteID {
				return errors.New(CodeSiteCycle, statusUnprocessable).WithParam("site_id", siteID)
			}
			ancestors, err := uc.sites.ListAncestors(ctx, tx, newParentSiteID)
			if err != nil {
				return err
			}
			for _, a := range ancestors {
				if a.ID == siteID {
					return errors.New(CodeSiteCycle, statusUnprocessable).
						WithParam("site_id", siteID).WithParam("new_parent_site_id", newParentSiteID)
				}
			}
		}
		if err := uc.sites.UpdateParent(ctx, tx, siteID, newParentSiteID, s.Audit.Version); err != nil {
			return err
		}
		out, err = uc.sites.GetByID(ctx, tx, siteID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// List 站点清单。
func (uc *SiteUseCase) List(ctx context.Context) ([]*Site, error) {
	var out []*Site
	err := uc.tx.WithinTx(ctx, func(ctx context.Context, tx Tx) error {
		var err error
		out, err = uc.sites.List(ctx, tx)
		return err
	})
	return out, err
}
