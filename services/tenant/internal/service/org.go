// OrgService 契约实现（api/org/v1）。
package service

import (
	"context"

	commonv1 "github.com/jsl-aiot/platform/api/common/v1"
	orgv1 "github.com/jsl-aiot/platform/api/org/v1"

	"github.com/jsl-aiot/platform/services/tenant/internal/biz"
)

// OrgService 组织服务。
type OrgService struct {
	orgv1.UnimplementedOrgServiceServer
	uc *biz.OrgUseCase
}

// NewOrgService 装配组织服务。
func NewOrgService(uc *biz.OrgUseCase) *OrgService {
	return &OrgService{uc: uc}
}

// CreateOrg 创建组织（父为空挂租户根）。
func (s *OrgService) CreateOrg(ctx context.Context, req *orgv1.CreateOrgRequest) (*orgv1.Organization, error) {
	o, err := s.uc.Create(ctx, req.GetParentOrgId(), req.GetName(), req.GetDisplayName())
	if err != nil {
		return nil, protoError(err)
	}
	return orgToProto(o), nil
}

// GetOrg 查询组织。
func (s *OrgService) GetOrg(ctx context.Context, req *orgv1.GetOrgRequest) (*orgv1.Organization, error) {
	o, err := s.uc.Get(ctx, req.GetOrgId())
	if err != nil {
		return nil, protoError(err)
	}
	return orgToProto(o), nil
}

// ListOrgs 组织清单（分页）。
func (s *OrgService) ListOrgs(ctx context.Context, req *orgv1.ListOrgsRequest) (*orgv1.ListOrgsResponse, error) {
	all, err := s.uc.List(ctx)
	if err != nil {
		return nil, protoError(err)
	}
	offset := decodePageToken(req.GetPage().GetPageToken())
	page := int(req.GetPage().GetPageSize())
	if page <= 0 {
		page = defaultPageSize
	}
	if page > maxPageSize {
		page = maxPageSize
	}
	items, next := pageSlice(all, offset, page)

	out := &orgv1.ListOrgsResponse{
		Orgs:  make([]*orgv1.Organization, 0, len(items)),
		Page:  &commonv1.PageResponse{NextPageToken: next, TotalEstimate: int64(len(all))},
	}
	for _, o := range items {
		out.Orgs = append(out.Orgs, orgToProto(o))
	}
	return out, nil
}

// UpdateOrg 更新组织（乐观锁）。
func (s *OrgService) UpdateOrg(ctx context.Context, req *orgv1.UpdateOrgRequest) (*orgv1.Organization, error) {
	o, err := s.uc.Update(ctx, biz.OrgUpdateInput{
		OrgID:       req.GetOrgId(),
		DisplayName: req.GetDisplayName(),
		Version:     req.GetVersion(),
	})
	if err != nil {
		return nil, protoError(err)
	}
	return orgToProto(o), nil
}

// orgToProto 组织聚合 → 契约消息。
func orgToProto(o *biz.Organization) *orgv1.Organization {
	return &orgv1.Organization{
		OrgId:       o.ID,
		TenantId:    o.TenantID,
		ParentOrgId: o.ParentOrgID,
		Name:        o.Name,
		DisplayName: o.DisplayName,
		Audit:       auditFields(o.Audit),
	}
}
