package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/ridesafety/auth-service/internal/domain/oauthsession"
	"github.com/ridesafety/auth-service/internal/infrastructure/postgres/models"
	"github.com/ridesafety/auth-service/internal/platform/eventbus"
	"gorm.io/gorm"
)

type oauthSessionRepository struct {
	db *gorm.DB
}

func NewOAuthSessionRepository(db *gorm.DB) oauthsession.Repository {
	return &oauthSessionRepository{
		db: db,
	}
}

func (r *oauthSessionRepository) getTx(ctx context.Context) *gorm.DB {
	if tx, ok := eventbus.GetTx(ctx); ok {
		return tx.WithContext(ctx)
	}
	return r.db.WithContext(ctx)
}

func (r *oauthSessionRepository) Create(ctx context.Context, s *oauthsession.OAuthSession) error {
	dbSession := models.OAuthSession{
		ID:             s.ID,
		State:          s.State,
		Nonce:          s.Nonce,
		CodeVerifier:   s.CodeVerifier,
		LoginChallenge: s.LoginChallenge,
		Used:           s.Used,
		CreatedAt:      s.CreatedAt,
		ExpiresAt:      s.ExpiresAt,
	}
	return r.getTx(ctx).Create(&dbSession).Error
}

func (r *oauthSessionRepository) FindByID(ctx context.Context, id uuid.UUID) (*oauthsession.OAuthSession, error) {
	var dbSession models.OAuthSession
	err := r.getTx(ctx).First(&dbSession, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, oauthsession.ErrSessionNotFound
		}
		return nil, err
	}
	return r.toEntity(&dbSession), nil
}

func (r *oauthSessionRepository) FindByState(ctx context.Context, state string) (*oauthsession.OAuthSession, error) {
	var dbSession models.OAuthSession
	err := r.getTx(ctx).First(&dbSession, "state = ?", state).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, oauthsession.ErrSessionNotFound
		}
		return nil, err
	}
	return r.toEntity(&dbSession), nil
}

func (r *oauthSessionRepository) UpdateLoginChallenge(ctx context.Context, id uuid.UUID, challenge string) error {
	res := r.getTx(ctx).Model(&models.OAuthSession{}).Where("id = ?", id).Update("login_challenge", challenge)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return oauthsession.ErrSessionNotFound
	}
	return nil
}

func (r *oauthSessionRepository) MarkUsed(ctx context.Context, id uuid.UUID) error {
	res := r.getTx(ctx).Model(&models.OAuthSession{}).Where("id = ?", id).Update("used", true)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return oauthsession.ErrSessionNotFound
	}
	return nil
}

func (r *oauthSessionRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.getTx(ctx).Delete(&models.OAuthSession{}, "id = ?", id).Error
}

func (r *oauthSessionRepository) DeleteExpired(ctx context.Context) (int64, error) {
	res := r.db.WithContext(ctx).Delete(&models.OAuthSession{}, "expires_at < ?", time.Now().UTC())
	return res.RowsAffected, res.Error
}

func (r *oauthSessionRepository) toEntity(s *models.OAuthSession) *oauthsession.OAuthSession {
	return &oauthsession.OAuthSession{
		ID:             s.ID,
		State:          s.State,
		Nonce:          s.Nonce,
		CodeVerifier:   s.CodeVerifier,
		LoginChallenge: s.LoginChallenge,
		Used:           s.Used,
		CreatedAt:      s.CreatedAt,
		ExpiresAt:      s.ExpiresAt,
	}
}
