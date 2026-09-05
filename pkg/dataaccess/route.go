package dataaccess

import (
	"sync"

	"github.com/jsl-aiot/platform/pkg/errors"
)

// IsolationLevel 租户隔离级别（BP-03 §3.1）。开通时由 BC-P1 登记。
type IsolationLevel string

const (
	LevelT3 IsolationLevel = "T3" // 行级 RLS（默认，共享库）—— 本期唯一生效级别
	LevelT2 IsolationLevel = "T2" // 独立 Schema（预留）
	LevelT1 IsolationLevel = "T1" // 独立实例/库（预留，含专有云）
)

// Route 租户数据路由。
type Route struct {
	Level  IsolationLevel
	Schema string // T2 专用：Schema 名
	DSN    string // T1 专用：独立实例 DSN
}

// RouteTable 租户数据路由表。
// BC-P1 在开通租户时登记；步 16 接入进程内热更新 + 事件广播（多副本一致性）。
type RouteTable interface {
	Register(tenantID string, r Route) error
	Lookup(tenantID string) (Route, bool)
}

// MemRouteTable 进程内路由表（初版；分布式热更新由步 16 广播机制替换）。
type MemRouteTable struct {
	mu      sync.RWMutex
	routes  map[string]Route
}

func NewMemRouteTable() *MemRouteTable {
	return &MemRouteTable{routes: make(map[string]Route)}
}

// Register 登记租户路由；T3 仅允许默认路由（Level+空 Schema/DSN）。
func (m *MemRouteTable) Register(tenantID string, r Route) error {
	if tenantID == "" {
		return errors.New("platform.data_invalid_route", 500)
	}
	switch r.Level {
	case LevelT3:
		if r.Schema != "" || r.DSN != "" {
			return errors.New("platform.data_invalid_route", 500)
		}
	case LevelT2, LevelT1:
		// 结构预留，实际路由（独立 Schema/实例）随多级隔离 ADR 实施
	default:
		return errors.New("platform.data_invalid_route", 500)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.routes[tenantID] = r
	return nil
}

func (m *MemRouteTable) Lookup(tenantID string) (Route, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, ok := m.routes[tenantID]
	return r, ok
}
