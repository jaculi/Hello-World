// 嵌入式迁移执行器：按文件名序执行 internal/data/migrations/*.sql。
// 约定：每个脚本自带 BEGIN/COMMIT（原子性），并在脚本内登记 schema_migrations。
package data

import (
	"context"
	"embed"
	"path"
	"sort"
	"strings"

	"github.com/jsl-aiot/platform/pkg/errors"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate 将未应用的迁移脚本按序执行（幂等：以 schema_migrations 台账为准）。
func (d *Data) Migrate(ctx context.Context) error {
	const ledgerSQL = `CREATE TABLE IF NOT EXISTS schema_migrations (
    version    text PRIMARY KEY,
    applied_at timestamptz NOT NULL DEFAULT now()
)`

	if err := d.pool.ExecMulti(ctx, ledgerSQL); err != nil {
		return err
	}
	applied, err := d.pool.AdminQueryStrings(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return err
	}
	done := make(map[string]bool, len(applied))
	for _, v := range applied {
		done[v] = true
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return errors.Wrap(err, "platform.data_migration_read_failed", 500)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		if !strings.HasSuffix(name, ".sql") {
			continue
		}
		version := strings.TrimSuffix(path.Base(name), ".sql")
		if done[version] {
			continue
		}
		script, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return errors.Wrap(err, "platform.data_migration_read_failed", 500)
		}
		// 脚本自带 BEGIN/COMMIT；失败即整脚本回滚，台账不登记。
		if err := d.pool.ExecMulti(ctx, string(script)); err != nil {
			return err
		}
	}
	return nil
}
