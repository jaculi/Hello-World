// 组织树仓储 SQL 实现（organizations；RLS 以 tenant_id 隔离）。
package data

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/jsl-aiot/platform/pkg/errors"

	"github.com/jsl-aiot/platform/services/tenant/internal/biz"
)

// OrgRepo 组织仓储。
type OrgRepo struct{}

const orgCols = `row_id, tenant_id, COALESCE(parent_org_id, ''), name, display_name,
    created_by, created_at, updated_by, updated_at, version`

const (
	insertOrgSQL = `INSERT INTO organizations
    (row_id, tenant_id, parent_org_id, name, display_name,
     created_by, created_at, updated_by, updated_at, version)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`

	selectOrgByIDSQL = `SELECT ` + orgCols + ` FROM organizations WHERE row_id = $1 AND deleted_at IS NULL`

	updateOrgSQL = `UPDATE organizations
    SET display_name = $2, updated_by = $3, updated_at = $4, version = version + 1
    WHERE row_id = $1 AND version = $5 AND deleted_at IS NULL`

	listOrgsSQL = `SELECT ` + orgCols + ` FROM organizations WHERE deleted_at IS NULL ORDER BY row_id`
)

// Insert 创建组织（(tenant,name) 唯一约束冲突映射为 name_taken）。
func (OrgRepo) Insert(ctx context.Context, tx biz.Tx, o *biz.Organization) error {
	var parent *string
	if o.ParentOrgID != "" {
		parent = &o.ParentOrgID
	}
	if _, err := tx.Exec(ctx, insertOrgSQL,
		o.ID, o.TenantID, parent, o.Name, o.DisplayName,
		o.Audit.CreatedBy, o.Audit.CreatedAt, o.Audit.UpdatedBy, o.Audit.UpdatedAt, o.Audit.Version,
	); err != nil {
		return mapUnique(err, "uq_orgs_tenant_name", biz.CodeOrgNameTaken, map[string]string{"name": o.Name})
	}
	return nil
}

// GetByID 查询组织（RLS 过滤跨租户）。
func (OrgRepo) GetByID(ctx context.Context, tx biz.Tx, id string) (*biz.Organization, error) {
	row := tx.QueryRow(ctx, selectOrgByIDSQL, id)
	o, err := scanOrg(row)
	if errNoRows(err) {
		return nil, errNotFound(biz.CodeOrgNotFound, "org_id", id)
	}
	return o, err
}

// Update 更新组织（乐观锁）。
func (OrgRepo) Update(ctx context.Context, tx biz.Tx, o *biz.Organization, expectedVersion int64) error {
	cmd, err := tx.Exec(ctx, updateOrgSQL,
		o.ID, o.DisplayName, o.Audit.UpdatedBy, o.Audit.UpdatedAt, expectedVersion)
	if err != nil {
		return errors.Wrap(err, "platform.data_write_failed", 500)
	}
	if cmd.RowsAffected() == 0 {
		return notFoundOrConflict(ctx, tx, "organizations", o.ID, "org_id", biz.CodeOrgNotFound)
	}
	return nil
}

// List 组织清单（RLS 界定当前租户可见范围）。
func (OrgRepo) List(ctx context.Context, tx biz.Tx) ([]*biz.Organization, error) {
	rows, err := tx.Query(ctx, listOrgsSQL)
	if err != nil {
		return nil, errors.Wrap(err, "platform.data_query_failed", 500)
	}
	defer rows.Close()

	var out []*biz.Organization
	for rows.Next() {
		o, err := scanOrg(rows)
		if err != nil {
			return nil, errors.Wrap(err, "platform.data_query_failed", 500)
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// scanOrg 行 → 组织聚合。
func scanOrg(row pgx.Row) (*biz.Organization, error) {
	var o biz.Organization
	err := row.Scan(&o.ID, &o.TenantID, &o.ParentOrgID, &o.Name, &o.DisplayName,
		&o.Audit.CreatedBy, &o.Audit.CreatedAt, &o.Audit.UpdatedBy, &o.Audit.UpdatedAt, &o.Audit.Version)
	if err != nil {
		return nil, err
	}
	return &o, nil
}
