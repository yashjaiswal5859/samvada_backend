// =============================================================================
// Samvada Platform — ABS PostgreSQL Infrastructure Adapter (GORM)
// =============================================================================

package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/plugin/dbresolver"

	"github.com/samvada/abs/configs"
)

// GormZapLogger bridges GORM's logging interface to Uber Zap.
type GormZapLogger struct {
	ZapLogger     *zap.Logger
	SlowThreshold time.Duration
}

func (l *GormZapLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	return l
}

func (l *GormZapLogger) Info(ctx context.Context, msg string, data ...interface{}) {
	l.ZapLogger.Sugar().Infof(msg, data...)
}

func (l *GormZapLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	l.ZapLogger.Sugar().Warnf(msg, data...)
}

func (l *GormZapLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	l.ZapLogger.Sugar().Errorf(msg, data...)
}

func (l *GormZapLogger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	elapsed := time.Since(begin)
	sql, rows := fc()

	if err != nil && err != gorm.ErrRecordNotFound {
		l.ZapLogger.Error("Database query error",
			zap.Error(err),
			zap.Duration("elapsed", elapsed),
			zap.Int64("rows", rows),
			zap.String("sql", sql),
		)
		return
	}

	if elapsed > l.SlowThreshold {
		l.ZapLogger.Warn("Database slow query detected",
			zap.Duration("elapsed", elapsed),
			zap.Int64("rows", rows),
			zap.String("sql", sql),
		)
		return
	}

	l.ZapLogger.Debug("Database query executed",
		zap.Duration("elapsed", elapsed),
		zap.Int64("rows", rows),
		zap.String("sql", sql),
	)
}

// NewConnection initializes a production-grade PostgreSQL GORM pool.
func NewConnection(cfg *configs.DatabaseConfig, log *zap.Logger) (*gorm.DB, error) {
	dsn := cfg.DSN()

	gLogger := &GormZapLogger{
		ZapLogger:     log.Named("postgres"),
		SlowThreshold: 200 * time.Millisecond,
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger:                 gLogger,
		PrepareStmt:            true,
		SkipDefaultTransaction: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to extract sql.DB handle: %w", err)
	}

	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetimeMin) * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping PostgreSQL database: %w", err)
	}

	log.Info("Connected to PostgreSQL successfully",
		zap.String("host", cfg.Host),
		zap.Int("port", cfg.Port),
		zap.String("dbname", cfg.DBName),
		zap.Int("max_open_conns", cfg.MaxOpenConns),
	)

	// Configure Read Replicas (Phase 0.1)
	if len(cfg.ReplicaHosts) > 0 {
		var replicaDialectors []gorm.Dialector
		for _, host := range cfg.ReplicaHosts {
			trimmed := strings.TrimSpace(host)
			if trimmed == "" {
				continue
			}
			repDSN := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s timezone=UTC",
				trimmed, cfg.Port, cfg.User, cfg.Password, cfg.DBName, cfg.SSLMode)
			replicaDialectors = append(replicaDialectors, postgres.Open(repDSN))
		}

		if len(replicaDialectors) > 0 {
			err = db.Use(dbresolver.Register(dbresolver.Config{
				Replicas:          replicaDialectors,
				Policy:            dbresolver.RandomPolicy{},
				TraceResolverMode: true,
			}).
				SetMaxOpenConns(cfg.MaxOpenConns).
				SetMaxIdleConns(cfg.MaxIdleConns).
				SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetimeMin) * time.Minute))
			if err != nil {
				log.Warn("Failed to register PostgreSQL read replicas", zap.Error(err))
			} else {
				log.Info("Registered PostgreSQL read replicas successfully", zap.Int("replica_count", len(replicaDialectors)))
			}
		}
	}

	return db, nil
}
