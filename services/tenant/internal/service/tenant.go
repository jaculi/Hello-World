// TenantService 契约实现（api/tenant/v1）。
package service

import (
	"context"

	tenantv1 "github.com/jsl-aiot/platform/api/tenant/v1"

	"github.com/jsl-aiot/platform/services/tenant/internal/biz"
)

// TenantService 租户服务。
type TenantService struct {
	tenantv1.UnimplementedTenantServiceServer
	uc *biz.TenantUseCase
}

// NewTenantService 装配租户服务。
func NewTenantService(uc *biz.TenantUseCase) *TenantService {
	return &TenantService{uc: uc}
}

// CreateTenant 开通租户（创建即 ACTIVE）。
func (s *TenantService) CreateTenant(ctx context.Context, req *tenantv1.CreateTenantRequest) (*tenantv1.Tenant, error) {
	t, err := s.uc.CreateTenant(ctx, biz.CreateTenantInput{
		Name:        req.GetName(),
		DisplayName: req.GetDisplayName(),
		Isolation:   isolationFromProto(req.GetIsolation()),
		PlanID:      req.GetPlanId(),
		HomeRegion:  req.GetHomeRegion(),
	})
	if err != nil {
		return nil, protoError(err)
	}
	return tenantToProto(t), nil
}

// GetTenant 查询租户。
func (s *TenantService) GetTenant(ctx context.Context, req *tenantv1.GetTenantRequest) (*tenantv1.Tenant, error) {
	t, err := s.uc.GetTenant(ctx, req.GetTenantId())
	if err != nil {
		return nil, protoError(err)
	}
	return tenantToProto(t), nil
}

// SuspendTenant 冻结租户。
func (s *TenantService) SuspendTenant(ctx context.Context, req *tenantv1.SuspendTenantRequest) (*tenantv1.Tenant, error) {
	t, err := s.uc.SuspendTenant(ctx, req.GetTenantId(), req.GetReason())
	if err != nil {
		return nil, protoError(err)
	}
	return tenantToProto(t), nil
}

// ReactivateTenant 恢复租户。
func (s *TenantService) ReactivateTenant(ctx context.Context, req *tenantv1.ReactivateTenantRequest) (*tenantv1.Tenant, error) {
	t, err := s.uc.ReactivateTenant(ctx, req.GetTenantId())
	if err != nil {
		return nil, protoError(err)
	}
	return tenantToProto(t), nil
}

// DeactivateTenant 注销租户（终态）。
func (s *TenantService) DeactivateTenant(ctx context.Context, req *tenantv1.DeactivateTenantRequest) (*tenantv1.Tenant, error) {
	t, err := s.uc.DeactivateTenant(ctx, req.GetTenantId(), req.GetReason())
	if err != nil {
		return nil, protoError(err)
	}
	return tenantToProto(t), nil
}

// BindIndustryAssembly 绑定行业模块装配。
func (s *TenantService) BindIndustryAssembly(ctx context.Context, req *tenantv1.BindIndustryAssemblyRequest) (*tenantv1.IndustryAssembly, error) {
	a, err := s.uc.BindIndustryAssembly(ctx, req.GetTenantId(), req.GetModuleId(), req.GetModuleVersion(), req.GetEnabled())
	if err != nil {
		return nil, protoError(err)
	}
	return assemblyToProto(a), nil
}

// ListIndustryAssemblies 装配清单。
func (s *TenantService) ListIndustryAssemblies(ctx context.Context, req *tenantv1.ListIndustryAssembliesRequest) (*tenantv1.ListIndustryAssembliesResponse, error) {
	list, err := s.uc.ListIndustryAssemblies(ctx, req.GetTenantId())
	if err != nil {
		return nil, protoError(err)
	}
	out := &tenantv1.ListIndustryAssembliesResponse{Assemblies: make([]*tenantv1.IndustryAssembly, 0, len(list))}
	for i := range list {
		out.Assemblies = append(out.Assemblies, assemblyToProto(&list[i]))
	}
	return out, nil
}

// tenantToProto 租户聚合 → 契约消息。
func tenantToProto(t *biz.Tenant) *tenantv1.Tenant {
	return &tenantv1.Tenant{
		TenantId:    t.ID,
		Name:        t.Name,
		DisplayName: t.DisplayName,
		Status:      statusToProto(t.Status),
		Isolation:   isolationToProto(t.Isolation),
		PlanId:      t.PlanID,
		HomeRegion:  t.HomeRegion,
		Audit:       auditFields(t.Audit),
	}
}

// assemblyToProto 装配记录 → 契约消息。
func assemblyToProto(a *biz.IndustryAssembly) *tenantv1.IndustryAssembly {
	return &tenantv1.IndustryAssembly{
		ModuleId:      a.ModuleID,
		ModuleVersion: a.ModuleVersion,
		Enabled:       a.Enabled,
		Audit:         auditFields(a.Audit),
	}
}
