// 租户聚合仓储 SQL 实现（tenants / subscriptions / industry_assemblies）。
// 跨租户可见性由 RLS 策略兜底（tenants 以 row_id 对齐 app.tenant_id）。
package data

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/jsl-aiot/platform/pkg/errors"

	"github.com/jsl-aiot/platform/services/tenant/internal/biz"
)

// TenantRepo 租户仓储。
type TenantRepo struct{}

const tenantCols = `row_id, name, display_name, status, isolation, plan_id, home_region,
    created_by, created_at, updated_by, updated_at, version`

const (
	insertTenantSQL = `INSERT INTO tenants
    (row_id, name, display_name, status, isolation, plan_id, home_region,
     created_by, created_at, updated_by, updated_at, version)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`

	insertSubscriptionSQL = `INSERT INTO subscriptions
    (row_id, tenant_id, plan_id, status, created_by, created_at, updated_by, updated_at, version)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`

	insertAssemblySQL = `INSERT INTO industry_assemblies
    (tenant_id, module_id, module_version, enabled, created_by, created_at, updated_by, updated_at, version)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`

	selectTenantByIDSQL  = `SELECT ` + tenantCols + ` FROM tenants WHERE row_id = $1 AND deleted_at IS NULL`
	updateTenantStatusSQL = `UPDATE tenants
    SET status = $2, updated_by = $3, updated_at = $4, version = version + 1
    WHERE row_id = $1 AND version = $5 AND deleted_at IS NULL`

	upsertAssemblySQL = `INSERT INTO industry_assemblies
    (tenant_id, module_id, module_version, enabled, created_by, created_at, updated_by, updated_at, version)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
ON CONFLICT (tenant_id, module_id) DO UPDATE
SET module_version = EXCLUDED.module_version, enabled = EXCLUDED.enabled,
    updated_by = EXCLUDED.updated_by, updated_at = EXCLUDED.updated_at,
    version = industry_assemblies.version + 1`

	listAssembliesSQL = `SELECT module_id, module_version, enabled, created_by, created_at, updated_by, updated_at, version
FROM industry_assemblies WHERE tenant_id = $1 ORDER BY module_id`
)

// Insert 原子写入租户 + 默认订阅 + 装配记录（调用方处于同事务）。
func (TenantRepo) Insert(ctx context.Context, tx biz.Tx, t *biz.Tenant, sub *biz.Subscription, assemblies []biz.IndustryAssembly) error {
	if _, err := tx.Exec(ctx, insertTenantSQL,
		t.ID, t.Name, t.DisplayName, string(t.Status), string(t.Isolation), t.PlanID, t.HomeRegion,
		t.Audit.CreatedBy, t.Audit.CreatedAt, t.Audit.UpdatedBy, t.Audit.UpdatedAt, t.Audit.Version,
	); err != nil {
		return mapUnique(err, "uq_tenants_name", biz.CodeTenantNameTaken, map[string]string{"name": t.Name})
	}
	if _, err := tx.Exec(ctx, insertSubscriptionSQL,
		sub.ID, sub.TenantID, sub.PlanID, sub.Status,
		sub.Audit.CreatedBy, sub.Audit.CreatedAt, sub.Audit.UpdatedBy, sub.Audit.UpdatedAt, sub.Audit.Version,
	); err != nil {
		return mapUnique(err, "uq_subscriptions_tenant", "platform.data_write_failed", nil)
	}
	for i := range assemblies {
		a := &assemblies[i]
		if _, err := tx.Exec(ctx, insertAssemblySQL,
			a.TenantID, a.ModuleID, a.ModuleVersion, a.Enabled,
			a.Audit.CreatedBy, a.Audit.CreatedAt, a.Audit.UpdatedBy, a.Audit.UpdatedAt, a.Audit.Version,
		); err != nil {
			return mapUnique(err, "", "platform.data_write_failed", nil)
		}
	}
	return nil
}

// GetByID 查询租户（RLS 过滤跨租户）。
func (TenantRepo) GetByID(ctx context.Context, tx biz.Tx, id string) (*biz.Tenant, error) {
	row := tx.QueryRow(ctx, selectTenantByIDSQL, id)
	t, err := scanTenant(row)
	if errNoRows(err) {
		return nil, errNotFound(biz.CodeTenantNotFound, "tenant_id", id)
	}
	return t, err
}

// UpdateStatus 状态迁移（乐观锁）。
func (TenantRepo) UpdateStatus(ctx context.Context, tx biz.Tx, t *biz.Tenant, expectedVersion int64) error {
	cmd, err := tx.Exec(ctx, updateTenantStatusSQL,
		t.ID, string(t.Status), t.Audit.UpdatedBy, t.Audit.UpdatedAt, expectedVersion)
	if err != nil {
		return errors.Wrap(err, "platform.data_write_failed", 500)
	}
	if cmd.RowsAffected() == 0 {
		// 区分：记录缺失 vs 乐观锁冲突
		var one int
		if qerr := tx.QueryRow(ctx, `SELECT 1 FROM tenants WHERE row_id = $1 AND deleted_at IS NULL`, t.ID).Scan(&one); qerr != nil {
			return errNotFound(biz.CodeTenantNotFound, "tenant_id", t.ID)
		}
		return errors.New(biz.CodeVersionConflict, statusConflict)
	}
	return nil
}

// UpsertAssembly 装配记录写入或更新（模块 ID 幂等）。
func (TenantRepo) UpsertAssembly(ctx context.Context, tx biz.Tx, a biz.IndustryAssembly) error {
	if _, err := tx.Exec(ctx, upsertAssemblySQL,
		a.TenantID, a.ModuleID, a.ModuleVersion, a.Enabled,
		a.Audit.CreatedBy, a.Audit.CreatedAt, a.Audit.UpdatedBy, a.Audit.UpdatedAt, a.Audit.Version,
	); err != nil {
		return errors.Wrap(err, "platform.data_write_failed", 500)
	}
	return nil
}

// ListAssemblies 装配清单。
func (TenantRepo) ListAssemblies(ctx context.Context, tx biz.Tx, tenantID string) ([]biz.IndustryAssembly, error) {
	rows, err := tx.Query(ctx, listAssembliesSQL, tenantID)
	if err != nil {
		return nil, errors.Wrap(err, "platform.data_query_failed", 500)
	}
	defer rows.Close()

	var out []biz.IndustryAssembly
	for rows.Next() {
		var a biz.IndustryAssembly
		if err := rows.Scan(&a.ModuleID, &a.ModuleVersion, &a.Enabled,
			&a.Audit.CreatedBy, &a.Audit.CreatedAt, &a.Audit.UpdatedBy, &a.Audit.UpdatedAt, &a.Audit.Version); err != nil {
			return nil, errors.Wrap(err, "platform.data_query_failed", 500)
		}
		a.TenantID = tenantID
		out = append(out, a)
	}
	return out, rows.Err()
}

// scanTenant 行 → 租户聚合。
func scanTenant(row pgx.Row) (*biz.Tenant, error) {
	var t biz.Tenant
	var status, isolation string
	err := row.Scan(&t.ID, &t.Name, &t.DisplayName, &status, &isolation, &t.PlanID, &t.HomeRegion,
		&t.Audit.CreatedBy, &t.Audit.CreatedAt, &t.Audit.UpdatedBy, &t.Audit.UpdatedAt, &t.Audit.Version)
	if err != nil {
		return nil, err
	}
	t.Status = biz.TenantStatus(status)
	t.Isolation = biz.IsolationLevel(isolation)
	return &t, nil
}
