// =============================================================================
// Samvada Platform — ABS Database Migration Runner
// =============================================================================
// Usage:
//   go run ./cmd/migrate up       # Apply all pending migrations
//   go run ./cmd/migrate status   # Check current migration status
//   go run ./cmd/migrate down     # Rollback the last migration
// =============================================================================

package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/samvada/abs/configs"
)

type MigrationFile struct {
	Version  string
	Name     string
	UpPath   string
	DownPath string
}

func main() {
	cmd := "up"
	if len(os.Args) > 1 {
		cmd = strings.ToLower(os.Args[1])
	}

	cfg, err := configs.Load("configs/config.yaml")
	if err != nil {
		fmt.Printf("❌ Failed to load config: %v\n", err)
		os.Exit(1)
	}

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		cfg.Database.User, cfg.Database.Password, cfg.Database.Host, cfg.Database.Port, cfg.Database.DBName, cfg.Database.SSLMode)

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		fmt.Printf("❌ Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		fmt.Printf("❌ Failed to ping database: %v\n", err)
		os.Exit(1)
	}

	// 1. Ensure schema_migrations table exists
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version VARCHAR(255) PRIMARY KEY,
			applied_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
		);
	`)
	if err != nil {
		fmt.Printf("❌ Failed to create schema_migrations table: %v\n", err)
		os.Exit(1)
	}

	// 2. Discover migrations in migrations/ directory
	migrationsDir := "migrations"
	files, err := os.ReadDir(migrationsDir)
	if err != nil {
		fmt.Printf("❌ Failed to read migrations directory: %v\n", err)
		os.Exit(1)
	}

	migrationMap := make(map[string]*MigrationFile)
	for _, f := range files {
		name := f.Name()
		if f.IsDir() || !strings.HasSuffix(name, ".sql") {
			continue
		}

		parts := strings.SplitN(name, "_", 2)
		if len(parts) < 2 {
			continue
		}
		version := parts[0]

		if _, ok := migrationMap[version]; !ok {
			migrationMap[version] = &MigrationFile{
				Version: version,
				Name:    name,
			}
		}

		fullPath := filepath.Join(migrationsDir, name)
		if strings.HasSuffix(name, ".up.sql") {
			migrationMap[version].UpPath = fullPath
			migrationMap[version].Name = strings.TrimSuffix(name, ".up.sql")
		} else if strings.HasSuffix(name, ".down.sql") {
			migrationMap[version].DownPath = fullPath
		}
	}

	var versions []string
	for v := range migrationMap {
		versions = append(versions, v)
	}
	sort.Strings(versions)

	// 3. Query applied migrations
	rows, err := db.Query(`SELECT version, applied_at FROM schema_migrations ORDER BY version ASC`)
	if err != nil {
		fmt.Printf("❌ Failed to query applied migrations: %v\n", err)
		os.Exit(1)
	}
	defer rows.Close()

	applied := make(map[string]time.Time)
	for rows.Next() {
		var v string
		var t time.Time
		if err := rows.Scan(&v, &t); err == nil {
			applied[v] = t
		}
	}

	switch cmd {
	case "status":
		fmt.Println("===================================================================")
		fmt.Println("  Samvada ABS Database Migration Status")
		fmt.Println("===================================================================")
		for _, v := range versions {
			m := migrationMap[v]
			if t, ok := applied[v]; ok {
				fmt.Printf("  [APPLIED] %s (%s) — %s\n", v, m.Name, t.Format("2006-01-02 15:04:05"))
			} else {
				fmt.Printf("  [PENDING] %s (%s)\n", v, m.Name)
			}
		}
		fmt.Println("===================================================================")

	case "up":
		fmt.Println("===================================================================")
		fmt.Println("  Samvada ABS: Running Pending Migrations...")
		fmt.Println("===================================================================")
		appliedCount := 0
		for _, v := range versions {
			if _, ok := applied[v]; ok {
				continue
			}

			m := migrationMap[v]
			if m.UpPath == "" {
				continue
			}

			sqlContent, err := os.ReadFile(m.UpPath)
			if err != nil {
				fmt.Printf("❌ Failed to read migration file %s: %v\n", m.UpPath, err)
				os.Exit(1)
			}

			fmt.Printf("  Applying %s (%s)...", v, m.Name)
			tx, err := db.Begin()
			if err != nil {
				fmt.Printf(" failed to start transaction: %v\n", err)
				os.Exit(1)
			}

			if _, err := tx.Exec(string(sqlContent)); err != nil {
				tx.Rollback()
				fmt.Printf(" FAILED: %v\n", err)
				os.Exit(1)
			}

			if _, err := tx.Exec(`INSERT INTO schema_migrations (version) VALUES ($1)`, v); err != nil {
				tx.Rollback()
				fmt.Printf(" failed to record migration: %v\n", err)
				os.Exit(1)
			}

			if err := tx.Commit(); err != nil {
				fmt.Printf(" failed to commit: %v\n", err)
				os.Exit(1)
			}

			fmt.Println(" DONE")
			appliedCount++
		}

		if appliedCount == 0 {
			fmt.Println("  All migrations are already up to date. (0 applied)")
		} else {
			fmt.Printf("  Successfully applied %d migration(s).\n", appliedCount)
		}
		fmt.Println("===================================================================")

	case "down":
		fmt.Println("===================================================================")
		fmt.Println("  Samvada ABS: Rolling back last migration...")
		fmt.Println("===================================================================")
		// Find highest applied version
		var lastApplied string
		for i := len(versions) - 1; i >= 0; i-- {
			if _, ok := applied[versions[i]]; ok {
				lastApplied = versions[i]
				break
			}
		}

		if lastApplied == "" {
			fmt.Println("  No applied migrations to roll back.")
			return
		}

		m := migrationMap[lastApplied]
		if m.DownPath == "" {
			fmt.Printf("❌ No down migration found for %s\n", lastApplied)
			os.Exit(1)
		}

		sqlContent, err := os.ReadFile(m.DownPath)
		if err != nil {
			fmt.Printf("❌ Failed to read down migration %s: %v\n", m.DownPath, err)
			os.Exit(1)
		}

		fmt.Printf("  Reverting %s (%s)...", lastApplied, m.Name)
		tx, err := db.Begin()
		if err != nil {
			fmt.Printf(" failed to start transaction: %v\n", err)
			os.Exit(1)
		}

		if _, err := tx.Exec(string(sqlContent)); err != nil {
			tx.Rollback()
			fmt.Printf(" FAILED: %v\n", err)
			os.Exit(1)
		}

		if _, err := tx.Exec(`DELETE FROM schema_migrations WHERE version = $1`, lastApplied); err != nil {
			tx.Rollback()
			fmt.Printf(" failed to delete migration record: %v\n", err)
			os.Exit(1)
		}

		if err := tx.Commit(); err != nil {
			fmt.Printf(" failed to commit: %v\n", err)
			os.Exit(1)
		}

		fmt.Println(" DONE (Rollback complete)")
		fmt.Println("===================================================================")

	default:
		fmt.Printf("Unknown command '%s'. Supported: up, down, status\n", cmd)
		os.Exit(1)
	}
}
