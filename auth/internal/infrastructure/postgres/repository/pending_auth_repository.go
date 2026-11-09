package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/ridesafety/auth-service/internal/domain/pendingauth"
	"github.com/ridesafety/auth-service/internal/infrastructure/postgres/models"
	"github.com/ridesafety/auth-service/internal/platform/eventbus"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type pendingAuthRepository struct {
	db *gorm.DB
}

func NewPendingAuthRepository(db *gorm.DB) pendingauth.Repository {
	return &pendingAuthRepository{
		db: db,
	}
}

func (r *pendingAuthRepository) getTx(ctx context.Context) *gorm.DB {
	if tx, ok := eventbus.GetTx(ctx); ok {
		return tx.WithContext(ctx)
	}
	return r.db.WithContext(ctx)
}

func (r *pendingAuthRepository) Create(ctx context.Context, pa *pendingauth.PendingAuth) error {
	dbPending := models.PendingAuth{
		ID:           pa.ID,
		Email:        pa.Email,
		OTPHash:      pa.OTPHash,
		Purpose:      string(pa.Purpose),
		ExpiresAt:    pa.ExpiresAt,
		AttemptCount: pa.AttemptCount,
		MaxAttempts:  pa.MaxAttempts,
		Metadata:     pa.Metadata,
		CreatedAt:    pa.CreatedAt,
		UpdatedAt:    pa.UpdatedAt,
	}

	err := r.getTx(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "email"}, {Name: "purpose"}},
		DoUpdates: clause.AssignmentColumns([]string{"id", "otp_hash", "expires_at", "attempt_count", "max_attempts", "metadata", "updated_at"}),
	}).Create(&dbPending).Error

	return err
}

func (r *pendingAuthRepository) FindByID(ctx context.Context, id uuid.UUID) (*pendingauth.PendingAuth, error) {
	var dbPending models.PendingAuth
	err := r.getTx(ctx).First(&dbPending, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, pendingauth.ErrSessionNotFound
		}
		return nil, err
	}
	return r.toEntity(&dbPending), nil
}

func (r *pendingAuthRepository) FindByEmailAndPurpose(ctx context.Context, email string, purpose pendingauth.Purpose) (*pendingauth.PendingAuth, error) {
	var dbPending models.PendingAuth
	err := r.getTx(ctx).First(&dbPending, "email = ? AND purpose = ?", email, string(purpose)).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, pendingauth.ErrSessionNotFound
		}
		return nil, err
	}
	return r.toEntity(&dbPending), nil
}

func (r *pendingAuthRepository) Update(ctx context.Context, pa *pendingauth.PendingAuth) error {
	dbPending := models.PendingAuth{
		ID:           pa.ID,
		Email:        pa.Email,
		OTPHash:      pa.OTPHash,
		Purpose:      string(pa.Purpose),
		ExpiresAt:    pa.ExpiresAt,
		AttemptCount: pa.AttemptCount,
		MaxAttempts:  pa.MaxAttempts,
		Metadata:     pa.Metadata,
		CreatedAt:    pa.CreatedAt,
		UpdatedAt:    pa.UpdatedAt,
	}
	return r.getTx(ctx).Save(&dbPending).Error
}

func (r *pendingAuthRepository) Delete(ctx context.Context, id uuid.UUID) error {
	err := r.getTx(ctx).Delete(&models.PendingAuth{}, "id = ?", id).Error
	return err
}

func (r *pendingAuthRepository) DeleteExpired(ctx context.Context) (int64, error) {
	res := r.db.WithContext(ctx).Delete(&models.PendingAuth{}, "expires_at < ?", time.Now().UTC())
	return res.RowsAffected, res.Error
}

func (r *pendingAuthRepository) toEntity(p *models.PendingAuth) *pendingauth.PendingAuth {
	return &pendingauth.PendingAuth{
		ID:           p.ID,
		Email:        p.Email,
		OTPHash:      p.OTPHash,
		Purpose:      pendingauth.Purpose(p.Purpose),
		ExpiresAt:    p.ExpiresAt,
		AttemptCount: p.AttemptCount,
		MaxAttempts:  p.MaxAttempts,
		Metadata:     p.Metadata,
		CreatedAt:    p.CreatedAt,
		UpdatedAt:    p.UpdatedAt,
	}
}
