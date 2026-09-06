// 内存测试替身：领域测试与 e2e 共用（生产实现见 internal/data）。
// 语义与 SQL 实现对齐：唯一约束冲突、乐观锁预期版本、RLS 式租户隔离由替身模拟。
package biz

import (
	"context"
	"fmt"
	"sync"

	eventsv1 "github.com/jsl-aiot/platform/api/events/v1"
	"github.com/jsl-aiot/platform/pkg/errors"
	"github.com/jsl-aiot/platform/pkg/tenantcontext"
)

// MemTransactor 事务口替身：直接执行 fn（tx 传 nil）。
type MemTransactor struct{}

func (MemTransactor) WithinTx(ctx context.Context, fn func(context.Context, Tx) error) error {
	return fn(ctx, nil)
}

// WithinTenantTx 以目标租户身份执行，保留原 Subject。
func (MemTransactor) WithinTenantTx(ctx context.Context, tenantID string, fn func(context.Context, Tx) error) error {
	subj := ""
	if tc, ok := tenantcontext.From(ctx); ok {
		subj = tc.Subject
	}
	return fn(tenantcontext.With(ctx, tenantcontext.Context{TenantID: tenantID, Subject: subj}), nil)
}

// MemEventPublisher 事件出口替身：仅收集。
type MemEventPublisher struct {
	mu     sync.Mutex
	Events []*eventsv1.EventEnvelope
}

func (m *MemEventPublisher) Enqueue(_ context.Context, _ Tx, env *eventsv1.EventEnvelope) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Events = append(m.Events, env)
	return nil
}

// ByType 返回指定类型的事件。
func (m *MemEventPublisher) ByType(t string) []*eventsv1.EventEnvelope {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*eventsv1.EventEnvelope
	for _, e := range m.Events {
		if e.EventType == t {
			out = append(out, e)
		}
	}
	return out
}

// MemPlanRepo 套餐目录替身。
type MemPlanRepo struct {
	byID map[string]Plan
}

func NewMemPlanRepo(plans ...Plan) *MemPlanRepo {
	m := &MemPlanRepo{byID: make(map[string]Plan, len(plans))}
	for _, p := range plans {
		m.byID[p.ID] = p
	}
	return m
}

func (m *MemPlanRepo) GetByID(_ context.Context, _ Tx, id string) (*Plan, error) {
	if p, ok := m.byID[id]; ok {
		p2 := p
		return &p2, nil
	}
	return nil, errors.New(CodePlanNotFound, statusNotFound).WithParam("plan_id", id)
}

// MemTenantRepo 租户仓储替身。
type MemTenantRepo struct {
	mu               sync.Mutex
	tenants          map[string]*Tenant
	names            map[string]string
	assemblyByTenant map[string][]IndustryAssembly
}

func NewMemTenantRepo() *MemTenantRepo {
	return &MemTenantRepo{
		tenants:          make(map[string]*Tenant),
		names:            make(map[string]string),
		assemblyByTenant: make(map[string][]IndustryAssembly),
	}
}

func (m *MemTenantRepo) Insert(_ context.Context, _ Tx, t *Tenant, sub *Subscription, assemblies []IndustryAssembly) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, taken := m.names[t.Name]; taken {
		return errors.New(CodeTenantNameTaken, statusConflict).WithParam("name", t.Name)
	}
	cp := *t
	m.tenants[t.ID] = &cp
	m.names[t.Name] = t.ID
	m.assemblyByTenant[t.ID] = append(m.assemblyByTenant[t.ID], assemblies...)
	_ = sub // 订阅随租户落库（替身不单列存储）
	return nil
}

func (m *MemTenantRepo) GetByID(ctx context.Context, _ Tx, id string) (*Tenant, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tenants[id]
	if ok {
		if tc, _ := tenantcontext.From(ctx); tc.TenantID != "" && t.ID != tc.TenantID {
			ok = false // 模拟 RLS：跨租户不可见
		}
	}
	if !ok {
		return nil, errors.New(CodeTenantNotFound, statusNotFound).WithParam("tenant_id", id)
	}
	cp := *t
	return &cp, nil
}

func (m *MemTenantRepo) UpdateStatus(_ context.Context, _ Tx, t *Tenant, expectedVersion int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	stored, ok := m.tenants[t.ID]
	if !ok {
		return errors.New(CodeTenantNotFound, statusNotFound).WithParam("tenant_id", t.ID)
	}
	if stored.Audit.Version != expectedVersion {
		return errors.New(CodeVersionConflict, statusConflict)
	}
	cp := *t
	m.tenants[t.ID] = &cp
	return nil
}

func (m *MemTenantRepo) UpsertAssembly(_ context.Context, _ Tx, a IndustryAssembly) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	list := m.assemblyByTenant[a.TenantID]
	for i := range list {
		if list[i].ModuleID == a.ModuleID {
			list[i] = a
			m.assemblyByTenant[a.TenantID] = list
			return nil
		}
	}
	m.assemblyByTenant[a.TenantID] = append(list, a)
	return nil
}

func (m *MemTenantRepo) ListAssemblies(_ context.Context, _ Tx, tenantID string) ([]IndustryAssembly, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]IndustryAssembly, len(m.assemblyByTenant[tenantID]))
	copy(out, m.assemblyByTenant[tenantID])
	return out, nil
}

// MemOrgRepo 组织仓储替身。
type MemOrgRepo struct {
	mu   sync.Mutex
	orgs map[string]*Organization
}

func NewMemOrgRepo() *MemOrgRepo { return &MemOrgRepo{orgs: make(map[string]*Organization)} }

func (m *MemOrgRepo) Insert(_ context.Context, _ Tx, o *Organization) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, other := range m.orgs {
		if other.TenantID == o.TenantID && other.Name == o.Name {
			return errors.New(CodeOrgNameTaken, statusConflict).WithParam("name", o.Name)
		}
	}
	cp := *o
	m.orgs[o.ID] = &cp
	return nil
}

func (m *MemOrgRepo) GetByID(ctx context.Context, _ Tx, id string) (*Organization, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	o, ok := m.orgs[id]
	if ok {
		if tc, _ := tenantcontext.From(ctx); tc.TenantID != "" && o.TenantID != tc.TenantID {
			ok = false // 模拟 RLS：跨租户不可见
		}
	}
	if !ok {
		return nil, errors.New(CodeOrgNotFound, statusNotFound).WithParam("org_id", id)
	}
	cp := *o
	return &cp, nil
}

func (m *MemOrgRepo) Update(_ context.Context, _ Tx, o *Organization, expectedVersion int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	stored, ok := m.orgs[o.ID]
	if !ok {
		return errors.New(CodeOrgNotFound, statusNotFound).WithParam("org_id", o.ID)
	}
	if stored.Audit.Version != expectedVersion {
		return errors.New(CodeVersionConflict, statusConflict)
	}
	cp := *o
	m.orgs[o.ID] = &cp
	return nil
}

func (m *MemOrgRepo) List(ctx context.Context, _ Tx) ([]*Organization, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	tc, _ := tenantcontext.From(ctx)
	var out []*Organization
	for _, o := range m.orgs {
		if tc.TenantID != "" && o.TenantID != tc.TenantID {
			continue // 模拟 RLS：仅当前租户可见
		}
		cp := *o
		out = append(out, &cp)
	}
	return out, nil
}

// MemSiteRepo 站点仓储替身。
type MemSiteRepo struct {
	mu    sync.Mutex
	sites map[string]*Site
}

func NewMemSiteRepo() *MemSiteRepo { return &MemSiteRepo{sites: make(map[string]*Site)} }

func (m *MemSiteRepo) Insert(_ context.Context, _ Tx, s *Site) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, other := range m.sites {
		if other.TenantID == s.TenantID && other.Name == s.Name {
			return errors.New(CodeSiteNameTaken, statusConflict).WithParam("name", s.Name)
		}
	}
	cp := *s
	m.sites[s.ID] = &cp
	return nil
}

func (m *MemSiteRepo) GetByID(ctx context.Context, _ Tx, id string) (*Site, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sites[id]
	if ok {
		if tc, _ := tenantcontext.From(ctx); tc.TenantID != "" && s.TenantID != tc.TenantID {
			ok = false // 模拟 RLS：跨租户不可见
		}
	}
	if !ok {
		return nil, errors.New(CodeSiteNotFound, statusNotFound).WithParam("site_id", id)
	}
	cp := *s
	return &cp, nil
}

func (m *MemSiteRepo) Update(_ context.Context, _ Tx, s *Site, expectedVersion int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	stored, ok := m.sites[s.ID]
	if !ok {
		return errors.New(CodeSiteNotFound, statusNotFound).WithParam("site_id", s.ID)
	}
	if stored.Audit.Version != expectedVersion {
		return errors.New(CodeVersionConflict, statusConflict)
	}
	cp := *s
	m.sites[s.ID] = &cp
	return nil
}

func (m *MemSiteRepo) UpdateParent(_ context.Context, _ Tx, siteID, newParentID string, expectedVersion int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	stored, ok := m.sites[siteID]
	if !ok {
		return errors.New(CodeSiteNotFound, statusNotFound).WithParam("site_id", siteID)
	}
	if stored.Audit.Version != expectedVersion {
		return errors.New(CodeVersionConflict, statusConflict)
	}
	stored.ParentSiteID = newParentID
	stored.Audit.Version++
	return nil
}

func (m *MemSiteRepo) List(ctx context.Context, _ Tx) ([]*Site, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	tc, _ := tenantcontext.From(ctx)
	var out []*Site
	for _, s := range m.sites {
		if tc.TenantID != "" && s.TenantID != tc.TenantID {
			continue // 模拟 RLS：仅当前租户可见
		}
		cp := *s
		out = append(out, &cp)
	}
	return out, nil
}

// ListAncestors 自下而上走父链（含自身；深度上限 64，成环数据防护）。
func (m *MemSiteRepo) ListAncestors(_ context.Context, _ Tx, siteID string) ([]*Site, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*Site
	cur, ok := m.sites[siteID]
	if !ok {
		return nil, errors.New(CodeSiteNotFound, statusNotFound).WithParam("site_id", siteID)
	}
	seen := make(map[string]bool, 8)
	for cur != nil {
		if seen[cur.ID] {
			return nil, fmt.Errorf("memsite: parent cycle at %s", cur.ID)
		}
		seen[cur.ID] = true
		cp := *cur
		out = append(out, &cp)
		if len(out) > 64 {
			return nil, fmt.Errorf("memsite: ancestor chain exceeds depth limit")
		}
		if cur.ParentSiteID == "" {
			break
		}
		next, ok := m.sites[cur.ParentSiteID]
		if !ok {
			break
		}
		cur = next
	}
	return out, nil
}
