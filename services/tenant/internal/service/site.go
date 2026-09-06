// SiteService 契约实现（api/site/v1）。
package service

import (
	"context"

	commonv1 "github.com/jsl-aiot/platform/api/common/v1"
	sitev1 "github.com/jsl-aiot/platform/api/site/v1"

	"github.com/jsl-aiot/platform/services/tenant/internal/biz"
)

// SiteService 站点服务。
type SiteService struct {
	sitev1.UnimplementedSiteServiceServer
	uc *biz.SiteUseCase
}

// NewSiteService 装配站点服务。
func NewSiteService(uc *biz.SiteUseCase) *SiteService {
	return &SiteService{uc: uc}
}

// CreateSite 创建站点（父为空挂租户根）。
func (s *SiteService) CreateSite(ctx context.Context, req *sitev1.CreateSiteRequest) (*sitev1.Site, error) {
	site, err := s.uc.Create(ctx, req.GetParentSiteId(), req.GetName(), req.GetDisplayName(), req.GetSiteType())
	if err != nil {
		return nil, protoError(err)
	}
	return siteToProto(site), nil
}

// GetSite 查询站点。
func (s *SiteService) GetSite(ctx context.Context, req *sitev1.GetSiteRequest) (*sitev1.Site, error) {
	site, err := s.uc.Get(ctx, req.GetSiteId())
	if err != nil {
		return nil, protoError(err)
	}
	return siteToProto(site), nil
}

// ListSites 站点清单（分页）。
func (s *SiteService) ListSites(ctx context.Context, req *sitev1.ListSitesRequest) (*sitev1.ListSitesResponse, error) {
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

	out := &sitev1.ListSitesResponse{
		Sites: make([]*sitev1.Site, 0, len(items)),
		Page:  &commonv1.PageResponse{NextPageToken: next, TotalEstimate: int64(len(all))},
	}
	for _, site := range items {
		out.Sites = append(out.Sites, siteToProto(site))
	}
	return out, nil
}

// UpdateSite 更新站点（乐观锁）。
func (s *SiteService) UpdateSite(ctx context.Context, req *sitev1.UpdateSiteRequest) (*sitev1.Site, error) {
	site, err := s.uc.Update(ctx, biz.SiteUpdateInput{
		SiteID:      req.GetSiteId(),
		DisplayName: req.GetDisplayName(),
		SiteType:    req.GetSiteType(),
		Version:     req.GetVersion(),
	})
	if err != nil {
		return nil, protoError(err)
	}
	return siteToProto(site), nil
}

// MoveSite 移动站点子树（成环拒绝）。
func (s *SiteService) MoveSite(ctx context.Context, req *sitev1.MoveSiteRequest) (*sitev1.Site, error) {
	site, err := s.uc.Move(ctx, req.GetSiteId(), req.GetNewParentSiteId())
	if err != nil {
		return nil, protoError(err)
	}
	return siteToProto(site), nil
}

// siteToProto 站点聚合 → 契约消息。
func siteToProto(s *biz.Site) *sitev1.Site {
	return &sitev1.Site{
		SiteId:       s.ID,
		TenantId:     s.TenantID,
		ParentSiteId: s.ParentSiteID,
		Name:         s.Name,
		DisplayName:  s.DisplayName,
		SiteType:     s.SiteType,
		Audit:        auditFields(s.Audit),
	}
}
