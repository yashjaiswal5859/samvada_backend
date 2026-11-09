package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/ridesafety/auth-service/internal/domain/user"
	"github.com/ridesafety/auth-service/internal/infrastructure/postgres/models"
	"github.com/ridesafety/auth-service/internal/platform/eventbus"
	"gorm.io/gorm"
)

type userRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) user.Repository {
	return &userRepository{
		db: db,
	}
}

func (r *userRepository) getTx(ctx context.Context) *gorm.DB {
	if tx, ok := eventbus.GetTx(ctx); ok {
		return tx.WithContext(ctx)
	}
	return r.db.WithContext(ctx)
}

func (r *userRepository) Create(ctx context.Context, u *user.User) error {
	dbUser := models.User{
		ID:          u.ID,
		KratosID:    u.KratosID,
		Email:       u.Email,
		DisplayName: u.DisplayName,
		PhoneNumber: u.PhoneNumber,
		IsActive:    u.IsActive,
		CreatedAt:   u.CreatedAt,
		UpdatedAt:   u.UpdatedAt,
	}

	err := r.getTx(ctx).Create(&dbUser).Error
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			if strings.Contains(pgErr.ConstraintName, "email") || strings.Contains(pgErr.Message, "email") {
				return user.ErrEmailAlreadyExists
			}
			return user.ErrKratosIDAlreadyExists
		}
		return err
	}
	return nil
}

func (r *userRepository) FindByID(ctx context.Context, id uuid.UUID) (*user.User, error) {
	var dbUser models.User
	err := r.getTx(ctx).First(&dbUser, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, user.ErrUserNotFound
		}
		return nil, err
	}
	return r.toEntity(&dbUser), nil
}

func (r *userRepository) FindByEmail(ctx context.Context, email string) (*user.User, error) {
	var dbUser models.User
	err := r.getTx(ctx).First(&dbUser, "LOWER(TRIM(email)) = LOWER(TRIM(?))", email).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, user.ErrUserNotFound
		}
		return nil, err
	}
	return r.toEntity(&dbUser), nil
}

func (r *userRepository) FindByKratosID(ctx context.Context, kratosID string) (*user.User, error) {
	var dbUser models.User
	err := r.getTx(ctx).First(&dbUser, "kratos_id = ?", kratosID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, user.ErrUserNotFound
		}
		return nil, err
	}
	return r.toEntity(&dbUser), nil
}

func (r *userRepository) Update(ctx context.Context, u *user.User) error {
	dbUser := models.User{
		ID:          u.ID,
		KratosID:    u.KratosID,
		Email:       u.Email,
		DisplayName: u.DisplayName,
		PhoneNumber: u.PhoneNumber,
		IsActive:    u.IsActive,
		CreatedAt:   u.CreatedAt,
		UpdatedAt:   u.UpdatedAt,
	}

	err := r.getTx(ctx).Save(&dbUser).Error
	if err != nil {
		return err
	}
	return nil
}

func (r *userRepository) Delete(ctx context.Context, id uuid.UUID) error {
	res := r.getTx(ctx).Unscoped().Delete(&models.User{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return user.ErrUserNotFound
	}
	return nil
}

func (r *userRepository) toEntity(u *models.User) *user.User {
	var deletedAt *time.Time
	if u.DeletedAt.Valid {
		deletedAt = &u.DeletedAt.Time
	}
	return &user.User{
		ID:          u.ID,
		KratosID:    u.KratosID,
		Email:       u.Email,
		DisplayName: u.DisplayName,
		PhoneNumber: u.PhoneNumber,
		IsActive:    u.IsActive,
		CreatedAt:   u.CreatedAt,
		UpdatedAt:   u.UpdatedAt,
		DeletedAt:   deletedAt,
	}
}
