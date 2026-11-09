package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/ridesafety/auth-service/internal/bootstrap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	cfg, err := bootstrap.LoadConfig()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s TimeZone=UTC",
		cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, cfg.DBName, cfg.DBSSLMode)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}

	err = db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version BIGINT PRIMARY KEY,
		dirty BOOLEAN NOT NULL
	)`).Error
	if err != nil {
		log.Fatalf("failed to create schema_migrations table: %v", err)
	}

	args := os.Args[1:]
	if len(args) == 0 {
		log.Fatal("expected 'up' or 'down' subcommand")
	}

	cmd := args[0]
	switch cmd {
	case "up":
		runMigrations(db, "up")
	case "down":
		runMigrations(db, "down")
	default:
		log.Fatalf("unknown subcommand: %s", cmd)
	}
}

type migrationFile struct {
	version int
	name    string
	path    string
	isUp    bool
}

func runMigrations(db *gorm.DB, direction string) {
	files, err := filepath.Glob("migrations/*.sql")
	if err != nil {
		log.Fatalf("failed to read migrations directory: %v", err)
	}

	var migs []migrationFile
	for _, f := range files {
		base := filepath.Base(f)
		parts := strings.Split(base, "_")
		if len(parts) < 2 {
			continue
		}
		version, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}

		isUp := strings.HasSuffix(base, ".up.sql")
		isDown := strings.HasSuffix(base, ".down.sql")

		if (direction == "up" && isUp) || (direction == "down" && isDown) {
			migs = append(migs, migrationFile{
				version: version,
				name:    base,
				path:    f,
				isUp:    isUp,
			})
		}
	}

	if direction == "up" {
		sort.Slice(migs, func(i, j int) bool {
			return migs[i].version < migs[j].version
		})
	} else {
		sort.Slice(migs, func(i, j int) bool {
			return migs[i].version > migs[j].version
		})
	}

	var currentVersion int64
	var dirty bool
	err = db.Raw("SELECT version, dirty FROM schema_migrations LIMIT 1").Row().Scan(&currentVersion, &dirty)
	if err != nil {
		currentVersion = 0
		dirty = false
	}

	if dirty {
		log.Fatal("database is in a dirty state. Fix manually before proceeding.")
	}

	for _, m := range migs {
		if direction == "up" && int64(m.version) <= currentVersion {
			continue
		}
		if direction == "down" && int64(m.version) > currentVersion {
			continue
		}

		log.Printf("Running migration %s...", m.name)
		content, err := os.ReadFile(m.path)
		if err != nil {
			log.Fatalf("failed to read migration file %s: %v", m.name, err)
		}

		err = db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec("INSERT INTO schema_migrations (version, dirty) VALUES (?, ?) ON CONFLICT (version) DO UPDATE SET dirty = ?", m.version, true, true).Error; err != nil {
				return err
			}

			if err := tx.Exec(string(content)).Error; err != nil {
				return err
			}

			if direction == "up" {
				if err := tx.Exec("DELETE FROM schema_migrations").Error; err != nil {
					return err
				}
				if err := tx.Exec("INSERT INTO schema_migrations (version, dirty) VALUES (?, ?)", m.version, false).Error; err != nil {
					return err
				}
			} else {
				if err := tx.Exec("DELETE FROM schema_migrations").Error; err != nil {
					return err
				}
				prevVersion := m.version - 1
				if prevVersion > 0 {
					if err := tx.Exec("INSERT INTO schema_migrations (version, dirty) VALUES (?, ?)", prevVersion, false).Error; err != nil {
						return err
					}
				}
			}

			return nil
		})

		if err != nil {
			log.Fatalf("Migration %s failed: %v", m.name, err)
		}
		log.Printf("Migration %s completed successfully.", m.name)
	}

	log.Println("Migrations finished successfully.")
}
