package data

import (
	"embed"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// migratePostgresSchema applies each embedded SQL migration exactly once in filename order.
func migratePostgresSchema(db *gorm.DB) error {
	if err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version BIGINT PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`).Error; err != nil {
		return err
	}
	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		version, err := migrationVersion(entry.Name())
		if err != nil {
			return err
		}
		script, err := migrationFiles.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return err
		}
		if err := applyMigration(db, version, entry.Name(), string(script)); err != nil {
			return fmt.Errorf("apply migration %s: %w", entry.Name(), err)
		}
	}
	return nil
}

func applyMigration(db *gorm.DB, version int64, name, script string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", int64(0x42494c49)).Error; err != nil {
			return err
		}
		var applied int64
		if err := tx.Raw("SELECT COUNT(*) FROM schema_migrations WHERE version = ?", version).Scan(&applied).Error; err != nil {
			return err
		}
		if applied > 0 {
			return nil
		}
		if err := tx.Exec(script).Error; err != nil {
			return err
		}
		return tx.Exec("INSERT INTO schema_migrations (version, name) VALUES (?, ?)", version, name).Error
	})
}

func migrationVersion(name string) (int64, error) {
	prefix, _, ok := strings.Cut(name, "_")
	if !ok {
		return 0, fmt.Errorf("migration filename %q has no version prefix", name)
	}
	version, err := strconv.ParseInt(prefix, 10, 64)
	if err != nil || version <= 0 {
		return 0, fmt.Errorf("migration filename %q has an invalid version", name)
	}
	return version, nil
}
