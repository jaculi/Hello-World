// 站点树仓储 SQL 实现（sites；RLS 以 tenant_id 隔离）。
// ListAncestors 以递归 CTE 自下而上走父链（含自身），深度上限 64（成环数据防护）。
package data

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/jsl-aiot/platform/pkg/errors"

	"github.com/jsl-aiot/platform/services/tenant/internal/biz"
)

// SiteRepo 站点仓储。
type SiteRepo struct{}

const siteCols = `row_id, tenant_id, COALESCE(parent_site_id, ''), name, display_name, site_type,
    created_by, created_at, updated_by, updated_at, version`

const (
	insertSiteSQL = `INSERT INTO sites
    (row_id, tenant_id, parent_site_id, name, display_name, site_type,
     created_by, created_at, updated_by, updated_at, version)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`

	selectSiteByIDSQL = `SELECT ` + siteCols + ` FROM sites WHERE row_id = $1 AND deleted_at IS NULL`

	updateSiteSQL = `UPDATE sites
    SET display_name = $2, site_type = $3, updated_by = $4, updated_at = $5, version = version + 1
    WHERE row_id = $1 AND version = $6 AND deleted_at IS NULL`

	updateSiteParentSQL = `UPDATE sites
    SET parent_site_id = $2, version = version + 1
    WHERE row_id = $1 AND version = $3 AND deleted_at IS NULL`

	listSitesSQL = `SELECT ` + siteCols + ` FROM sites WHERE deleted_at IS NULL ORDER BY row_id`

	// listAncestorsSQL 祖先链：首个分支为目标站点自身，后续沿 parent_site_id 上溯。
	listAncestorsSQL = `WITH RECURSIVE chain AS (
    SELECT row_id, tenant_id, COALESCE(parent_site_id, '') AS parent_site_id, name, display_name, site_type,
           created_by, created_at, updated_by, updated_at, version, 0 AS depth
    FROM sites WHERE row_id = $1 AND deleted_at IS NULL
  UNION ALL
    SELECT s.row_id, s.tenant_id, COALESCE(s.parent_site_id, ''), s.name, s.display_name, s.site_type,
           s.created_by, s.created_at, s.updated_by, s.updated_at, s.version, c.depth + 1
    FROM sites s JOIN chain c ON s.row_id = c.parent_site_id
    WHERE c.depth < 64 AND s.deleted_at IS NULL
)
SELECT row_id, tenant_id, parent_site_id, name, display_name, site_type,
       created_by, created_at, updated_by, updated_at, version
FROM chain ORDER BY depth`
)

// Insert 创建站点（(tenant,name) 唯一约束冲突映射为 name_taken）。
func (SiteRepo) Insert(ctx context.Context, tx biz.Tx, s *biz.Site) error {
	var parent *string
	if s.ParentSiteID != "" {
		parent = &s.ParentSiteID
	}
	if _, err := tx.Exec(ctx, insertSiteSQL,
		s.ID, s.TenantID, parent, s.Name, s.DisplayName, s.SiteType,
		s.Audit.CreatedBy, s.Audit.CreatedAt, s.Audit.UpdatedBy, s.Audit.UpdatedAt, s.Audit.Version,
	); err != nil {
		return mapUnique(err, "uq_sites_tenant_name", biz.CodeSiteNameTaken, map[string]string{"name": s.Name})
	}
	return nil
}

// GetByID 查询站点（RLS 过滤跨租户）。
func (SiteRepo) GetByID(ctx context.Context, tx biz.Tx, id string) (*biz.Site, error) {
	row := tx.QueryRow(ctx, selectSiteByIDSQL, id)
	s, err := scanSite(row)
	if errNoRows(err) {
		return nil, errNotFound(biz.CodeSiteNotFound, "site_id", id)
	}
	return s, err
}

// Update 更新站点（乐观锁）。
func (SiteRepo) Update(ctx context.Context, tx biz.Tx, s *biz.Site, expectedVersion int64) error {
	cmd, err := tx.Exec(ctx, updateSiteSQL,
		s.ID, s.DisplayName, s.SiteType, s.Audit.UpdatedBy, s.Audit.UpdatedAt, expectedVersion)
	if err != nil {
		return errors.Wrap(err, "platform.data_write_failed", 500)
	}
	if cmd.RowsAffected() == 0 {
		return notFoundOrConflict(ctx, tx, "sites", s.ID, "site_id", biz.CodeSiteNotFound)
	}
	return nil
}

// UpdateParent 移动子树（乐观锁；环校验由领域层完成）。
func (SiteRepo) UpdateParent(ctx context.Context, tx biz.Tx, siteID, newParentID string, expectedVersion int64) error {
	var parent *string
	if newParentID != "" {
		parent = &newParentID
	}
	cmd, err := tx.Exec(ctx, updateSiteParentSQL, siteID, parent, expectedVersion)
	if err != nil {
		return errors.Wrap(err, "platform.data_write_failed", 500)
	}
	if cmd.RowsAffected() == 0 {
		return notFoundOrConflict(ctx, tx, "sites", siteID, "site_id", biz.CodeSiteNotFound)
	}
	return nil
}

// List 站点清单（RLS 界定当前租户可见范围）。
func (SiteRepo) List(ctx context.Context, tx biz.Tx) ([]*biz.Site, error) {
	rows, err := tx.Query(ctx, listSitesSQL)
	if err != nil {
		return nil, errors.Wrap(err, "platform.data_query_failed", 500)
	}
	defer rows.Close()

	var out []*biz.Site
	for rows.Next() {
		s, err := scanSite(rows)
		if err != nil {
			return nil, errors.Wrap(err, "platform.data_query_failed", 500)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ListAncestors 祖先链（自下而上、含自身；深度上限 64）。
func (SiteRepo) ListAncestors(ctx context.Context, tx biz.Tx, siteID string) ([]*biz.Site, error) {
	rows, err := tx.Query(ctx, listAncestorsSQL, siteID)
	if err != nil {
		return nil, errors.Wrap(err, "platform.data_query_failed", 500)
	}
	defer rows.Close()

	var out []*biz.Site
	for rows.Next() {
		s, err := scanSite(rows)
		if err != nil {
			return nil, errors.Wrap(err, "platform.data_query_failed", 500)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.Wrap(err, "platform.data_query_failed", 500)
	}
	if len(out) == 0 {
		return nil, errNotFound(biz.CodeSiteNotFound, "site_id", siteID)
	}
	return out, nil
}

// scanSite 行 → 站点聚合。
func scanSite(row pgx.Row) (*biz.Site, error) {
	var s biz.Site
	err := row.Scan(&s.ID, &s.TenantID, &s.ParentSiteID, &s.Name, &s.DisplayName, &s.SiteType,
		&s.Audit.CreatedBy, &s.Audit.CreatedAt, &s.Audit.UpdatedBy, &s.Audit.UpdatedAt, &s.Audit.Version)
	if err != nil {
		return nil, err
	}
	return &s, nil
}
