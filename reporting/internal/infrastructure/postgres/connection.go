// =============================================================================
// Samvada Platform — Reporting Engine PostgreSQL Infrastructure Adapter (GORM)
// =============================================================================

package postgres

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/samvada/reporting/configs"
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
	}
}

// NewConnection initializes a GORM PostgreSQL connection pool.
func NewConnection(cfg *configs.DatabaseConfig, log *zap.Logger) (*gorm.DB, error) {
	if cfg.Host == "" {
		return nil, fmt.Errorf("postgres host is empty")
	}

	dsn := cfg.DSN()
	customLogger := &GormZapLogger{
		ZapLogger:     log.Named("postgres"),
		SlowThreshold: 200 * time.Millisecond,
	}

	gormCfg := &gorm.Config{
		Logger:                                   customLogger,
		PrepareStmt:                              true,
		SkipDefaultTransaction:                   true,
		DisableForeignKeyConstraintWhenMigrating: true,
	}

	db, err := gorm.Open(postgres.Open(dsn), gormCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve sql.DB from gorm: %w", err)
	}

	maxOpen := cfg.MaxOpenConns
	if maxOpen <= 0 {
		maxOpen = 25
	}
	maxIdle := cfg.MaxIdleConns
	if maxIdle <= 0 {
		maxIdle = 5
	}
	connLifetime := cfg.ConnMaxLifetimeMin
	if connLifetime <= 0 {
		connLifetime = 15
	}

	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetMaxIdleConns(maxIdle)
	sqlDB.SetConnMaxLifetime(time.Duration(connLifetime) * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping PostgreSQL failed: %w", err)
	}

	log.Info("PostgreSQL connection established successfully",
		zap.String("host", cfg.Host),
		zap.Int("port", cfg.Port),
		zap.String("database", cfg.DBName),
	)

	return db, nil
}
