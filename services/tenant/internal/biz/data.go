// 领域端口：事务口、事件出口与仓储接口。
// 实现位于 internal/data（SQL）与 fakes.go（内存测试替身）；领域层仅面向接口。
package biz

import (
	"context"

	eventsv1 "github.com/jsl-aiot/platform/api/events/v1"
	"github.com/jsl-aiot/platform/pkg/dataaccess"
)

// Tx 数据访问事务句柄（pgx.Tx 最小子集，由 pkg/dataaccess 定义）。
type Tx = dataaccess.Tx

// Transactor 事务口：领域用例在单一 RLS 事务中组合多个仓储操作与事件写入（ADR-0005）。
type Transactor interface {
	// WithinTx 在调用者租户上下文的事务中执行（GUC 绑定调用者租户）。
	WithinTx(ctx context.Context, fn func(ctx context.Context, tx Tx) error) error
	// WithinTenantTx 以目标租户身份执行（开通、租户级管理操作）；
	// 保留原上下文 Subject 供审计记录真实操作者（BP-05 §9.1）。
	WithinTenantTx(ctx context.Context, tenantID string, fn func(ctx context.Context, tx Tx) error) error
}

// EventPublisher 事务性发件箱写入口（ADR-0005）：事件与业务同事务提交。
type EventPublisher interface {
	Enqueue(ctx context.Context, tx Tx, env *eventsv1.EventEnvelope) error
}

// TenantRepo 租户聚合仓储。
// 乐观锁约定：expectedVersion 为变更前的版本号，实现层以 WHERE version=$expected 保证并发安全。
type TenantRepo interface {
	// Insert 原子写入租户 + 默认订阅 + 装配记录。
	Insert(ctx context.Context, tx Tx, t *Tenant, sub *Subscription, assemblies []IndustryAssembly) error
	GetByID(ctx context.Context, tx Tx, id string) (*Tenant, error)
	UpdateStatus(ctx context.Context, tx Tx, t *Tenant, expectedVersion int64) error
	UpsertAssembly(ctx context.Context, tx Tx, a IndustryAssembly) error
	ListAssemblies(ctx context.Context, tx Tx, tenantID string) ([]IndustryAssembly, error)
}

// OrgRepo 组织仓储（RLS 界定可见范围，跨租户天然不可见）。
type OrgRepo interface {
	Insert(ctx context.Context, tx Tx, o *Organization) error
	GetByID(ctx context.Context, tx Tx, id string) (*Organization, error)
	Update(ctx context.Context, tx Tx, o *Organization, expectedVersion int64) error
	List(ctx context.Context, tx Tx) ([]*Organization, error)
}

// SiteRepo 站点仓储。
type SiteRepo interface {
	Insert(ctx context.Context, tx Tx, s *Site) error
	GetByID(ctx context.Context, tx Tx, id string) (*Site, error)
	Update(ctx context.Context, tx Tx, s *Site, expectedVersion int64) error
	// UpdateParent 移动子树（expectedVersion 为移动前版本）。
	UpdateParent(ctx context.Context, tx Tx, siteID, newParentID string, expectedVersion int64) error
	List(ctx context.Context, tx Tx) ([]*Site, error)
	// ListAncestors 返回 siteID 的祖先链（自下而上、含自身；深度上限 64）。
	ListAncestors(ctx context.Context, tx Tx, siteID string) ([]*Site, error)
}

// PlanRepo 套餐目录（平台全局读模型，非租户数据）。
type PlanRepo interface {
	GetByID(ctx context.Context, tx Tx, id string) (*Plan, error)
}
