// SQL 层错误映射：唯一约束 → 领域消息码；其余写失败归并（Cause 仅入日志，不外传）。
package data

import (
	"context"
	stderrors "errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/jsl-aiot/platform/pkg/errors"

	"github.com/jsl-aiot/platform/services/tenant/internal/biz"
)

// HTTP 状态映射（与 biz/errors.go 保持一致）。
const (
	statusNotFound = 404
	statusConflict = 409
)

// pgErrCodeUnique PostgreSQL 唯一约束冲突。
const pgErrCodeUnique = "23505"

// mapUnique 唯一约束冲突映射为领域消息码（constraint 为空表示任意唯一约束）。
func mapUnique(err error, constraint, code string, params map[string]string) error {
	var pgErr *pgconn.PgError
	if stderrors.As(err, &pgErr) && pgErr.Code == pgErrCodeUnique {
		if constraint == "" || pgErr.ConstraintName == constraint {
			e := errors.New(code, statusConflict)
			for k, v := range params {
				e = e.WithParam(k, v)
			}
			return e
		}
	}
	return errors.Wrap(err, "platform.data_write_failed", 500)
}

// errNotFound 记录缺失（跨租户经 RLS 过滤后同形）。
func errNotFound(code, param, id string) error {
	return errors.New(code, statusNotFound).WithParam(param, id)
}

// errNoRows 统一 pgx.ErrNoRows 判断。
func errNoRows(err error) bool {
	return stderrors.Is(err, pgx.ErrNoRows)
}

// notFoundOrConflict 乐观锁更新未命中：区分记录缺失（RLS 过滤同形）与版本冲突。
func notFoundOrConflict(ctx context.Context, tx biz.Tx, table, id, param, code string) error {
	var one int
	q := "SELECT 1 FROM " + table + " WHERE row_id = $1 AND deleted_at IS NULL"
	if err := tx.QueryRow(ctx, q, id).Scan(&one); err != nil {
		return errNotFound(code, param, id)
	}
	return errors.New(biz.CodeVersionConflict, statusConflict)
}
