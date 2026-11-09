package login

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/ridesafety/auth-service/internal/domain/events"
	"github.com/ridesafety/auth-service/internal/domain/pendingauth"
	"github.com/ridesafety/auth-service/internal/domain/user"
	"github.com/ridesafety/auth-service/internal/platform/eventbus"
	"github.com/ridesafety/auth-service/internal/platform/identity"
	"github.com/ridesafety/auth-service/internal/shared/constants"
	apperrors "github.com/ridesafety/auth-service/internal/shared/errors"
	"github.com/ridesafety/auth-service/internal/shared/otp"
	"gorm.io/gorm"
)

type Service interface {
	Login(ctx context.Context, req LoginRequest, correlationID string) (*LoginResponse, error)
	VerifyLogin(ctx context.Context, req VerifyLoginRequest, correlationID string) (*VerifyLoginResponse, error)
}

type service struct {
	db              *gorm.DB
	userRepo        user.Repository
	pendingAuthRepo pendingauth.Repository
	eventBus        eventbus.EventBus
	orchestrator    identity.IdentityOrchestrator
	otpExpiry       time.Duration
	maxAttempts     int
}

func NewService(
	db *gorm.DB,
	userRepo user.Repository,
	pendingAuthRepo pendingauth.Repository,
	eventBus eventbus.EventBus,
	orchestrator identity.IdentityOrchestrator,
	otpExpiry time.Duration,
	maxAttempts int,
) Service {
	if otpExpiry == 0 {
		otpExpiry = constants.OTPDefaultExpiry
	}
	if maxAttempts == 0 {
		maxAttempts = constants.OTPMaxAttempts
	}
	return &service{
		db:              db,
		userRepo:        userRepo,
		pendingAuthRepo: pendingAuthRepo,
		eventBus:        eventBus,
		orchestrator:    orchestrator,
		otpExpiry:       otpExpiry,
		maxAttempts:     maxAttempts,
	}
}

type otpEmailPayload struct {
	Type  string `json:"type"`
	Email string `json:"email"`
	OTP   string `json:"otp"`
}

type userLoggedInPayload struct {
	KratosID string `json:"kratos_id"`
	Email    string `json:"email"`
}

type userRegisteredPayload struct {
	KratosID    string `json:"kratos_id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	DeviceID    string `json:"device_id"`
}

func (s *service) Login(ctx context.Context, req LoginRequest, correlationID string) (*LoginResponse, error) {
	// 1. If user already exists in database, verify they are not deactivated.
	// Unified auth: if user does not exist yet, allow them to receive OTP and auto-provision on verify.
	u, err := s.userRepo.FindByEmail(ctx, req.Email)
	if err == nil {
		if !u.IsActive {
			return nil, apperrors.Forbidden("This account has been deactivated. Please contact support.", nil)
		}
	} else if !errors.Is(err, user.ErrUserNotFound) {
		return nil, apperrors.Internal("Failed to check user details", err)
	}

	// 2. Generate OTP
	otpVal, err := otp.GenerateOTP()
	if err != nil {
		return nil, apperrors.Internal("Failed to generate OTP", err)
	}
	fmt.Printf("\n[DEV_AUTH_OTP] OTP for %s is: %s\n\n", req.Email, otpVal)
	otpHash := otp.HashOTP(otpVal)

	paID := uuid.New()
	pa := &pendingauth.PendingAuth{
		ID:           paID,
		Email:        req.Email,
		OTPHash:      otpHash,
		Purpose:      pendingauth.PurposeLogin,
		ExpiresAt:    time.Now().UTC().Add(s.otpExpiry),
		AttemptCount: 0,
		MaxAttempts:  s.maxAttempts,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}

	// 3. Begin GORM transaction
	tx := s.db.Begin()
	if tx.Error != nil {
		return nil, apperrors.Internal("Failed to start transaction", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	ctxTx := eventbus.WithTx(ctx, tx)

	if err := s.pendingAuthRepo.Create(ctxTx, pa); err != nil {
		tx.Rollback()
		return nil, apperrors.Internal("Failed to save pending auth", err)
	}

	// Create outbox event
	emailPayload := otpEmailPayload{
		Type:  "LOGIN_OTP",
		Email: req.Email,
		OTP:   otpVal,
	}

	evt, err := events.NewDomainEvent(
		events.OTPRequested,
		paID.String(),
		"PendingAuth",
		constants.EventVersion,
		correlationID,
		emailPayload,
	)
	if err != nil {
		tx.Rollback()
		return nil, apperrors.Internal("Failed to create domain event", err)
	}

	if err := s.eventBus.Publish(ctxTx, evt); err != nil {
		tx.Rollback()
		return nil, apperrors.Internal("Failed to persist outbox event", err)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, apperrors.Internal("Failed to commit transaction", err)
	}

	return &LoginResponse{
		RequestID: paID.String(),
		ExpiresIn: int64(s.otpExpiry.Seconds()),
	}, nil
}

func (s *service) VerifyLogin(ctx context.Context, req VerifyLoginRequest, correlationID string) (*VerifyLoginResponse, error) {
	paID, err := uuid.Parse(req.RequestID)
	if err != nil {
		return nil, apperrors.BadRequest("Invalid requestId format", err)
	}

	// 1. Find pending auth
	pa, err := s.pendingAuthRepo.FindByID(ctx, paID)
	if err != nil {
		if errors.Is(err, pendingauth.ErrSessionNotFound) {
			return nil, apperrors.BadRequest("Authentication session expired or invalid", err)
		}
		return nil, apperrors.Internal("Failed to load session", err)
	}

	if pa.Purpose != pendingauth.PurposeLogin {
		return nil, apperrors.BadRequest("Invalid purpose for this session", nil)
	}

	// 2. Check expired
	if pa.IsExpired() {
		_ = s.pendingAuthRepo.Delete(ctx, pa.ID)
		return nil, apperrors.BadRequest("Authentication session has expired. Please request a new code.", pendingauth.ErrOTPExpired)
	}

	// 3. Check attempt count
	if pa.HasExceededAttempts() {
		_ = s.pendingAuthRepo.Delete(ctx, pa.ID)
		return nil, apperrors.Forbidden("Too many failed attempts", pendingauth.ErrMaxAttemptsExceeded)
	}

	// 4. Verify OTP
	if !otp.VerifyOTP(req.OTP, pa.OTPHash) {
		pa.IncrementAttempts()
		pa.UpdatedAt = time.Now().UTC()
		_ = s.pendingAuthRepo.Update(ctx, pa)

		if pa.HasExceededAttempts() {
			_ = s.pendingAuthRepo.Delete(ctx, pa.ID)
			return nil, apperrors.Forbidden("Too many failed attempts", pendingauth.ErrMaxAttemptsExceeded)
		}

		return nil, apperrors.BadRequest("Invalid OTP", pendingauth.ErrOTPInvalid)
	}

	// 5. Check if user already exists in database
	u, err := s.userRepo.FindByEmail(ctx, pa.Email)
	var isNewUser bool
	var kratosID string

	if err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			isNewUser = true
		} else {
			return nil, apperrors.Internal("Failed to retrieve user details", err)
		}
	} else {
		kratosID = u.KratosID
	}

	var resolvedUserID string
	if !isNewUser && u != nil {
		resolvedUserID = u.ID.String()
	}

	// 6. Unified Provisioning: if user is new, automatically provision and start onboarding!
	if isNewUser {
		var isNewIdentity bool
		kratosID, err = s.orchestrator.GetIdentityByEmail(ctx, pa.Email)
		if err != nil {
			if errors.Is(err, identity.ErrIdentityNotFound) {
				kratosID, err = s.orchestrator.CreateIdentity(ctx, pa.Email)
				if err != nil {
					return nil, apperrors.Internal("Failed to create identity in Kratos", err)
				}
				isNewIdentity = true
			} else {
				return nil, apperrors.Internal("Failed to query identity in Kratos", err)
			}
		}

		compensate := func() {
			if isNewIdentity {
				_ = s.orchestrator.DeleteIdentity(ctx, kratosID)
			}
		}

		tx := s.db.Begin()
		if tx.Error != nil {
			compensate()
			return nil, apperrors.Internal("Failed to start transaction", tx.Error)
		}
		defer func() {
			if r := recover(); r != nil {
				tx.Rollback()
				compensate()
			}
		}()

		ctxTx := eventbus.WithTx(ctx, tx)

		userUUID := uuid.New()
		resolvedUserID = userUUID.String()
		displayName := strings.Split(pa.Email, "@")[0]
		newUser := &user.User{
			ID:          userUUID,
			KratosID:    kratosID,
			Email:       pa.Email,
			DisplayName: displayName,
			IsActive:    true,
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		}

		if err := s.userRepo.Create(ctxTx, newUser); err != nil {
			tx.Rollback()
			compensate()
			if errors.Is(err, user.ErrEmailAlreadyExists) {
				existingUser, fetchErr := s.userRepo.FindByEmail(ctx, pa.Email)
				if fetchErr == nil {
					kratosID = existingUser.KratosID
				}
			} else {
				return nil, apperrors.Internal("Failed to save local user", err)
			}
		}

		// Create Keto permissions (regular user member role)
		relationReq := identity.RelationshipRequest{
			Namespace: "platform",
			Object:    "platform",
			Relation:  "member",
			Subject:   kratosID,
		}
		_ = s.orchestrator.CreateRelationship(ctxTx, relationReq)

		deviceRelationReq := identity.RelationshipRequest{
			Namespace: "devices",
			Object:    "SYSTEM",
			Relation:  "owner",
			Subject:   kratosID,
		}
		_ = s.orchestrator.CreateRelationship(ctxTx, deviceRelationReq)

		// Delete pending session
		if err := s.pendingAuthRepo.Delete(ctxTx, pa.ID); err != nil {
			tx.Rollback()
			compensate()
			return nil, apperrors.Internal("Failed to remove session", err)
		}

		// Persist UserRegistered domain event
		regPayload := userRegisteredPayload{
			KratosID:    kratosID,
			Email:       pa.Email,
			DisplayName: displayName,
			DeviceID:    "SYSTEM",
		}
		evt, err := events.NewDomainEvent(
			events.UserRegistered,
			kratosID,
			"User",
			constants.EventVersion,
			correlationID,
			regPayload,
		)
		if err == nil {
			_ = s.eventBus.Publish(ctxTx, evt)
		}

		if err := tx.Commit().Error; err != nil {
			compensate()
			return nil, apperrors.Internal("Failed to commit user creation", err)
		}
	} else {
		// Existing user flow
		tx := s.db.Begin()
		if tx.Error != nil {
			return nil, apperrors.Internal("Failed to start transaction", tx.Error)
		}
		defer func() {
			if r := recover(); r != nil {
				tx.Rollback()
			}
		}()

		ctxTx := eventbus.WithTx(ctx, tx)

		if err := s.pendingAuthRepo.Delete(ctxTx, pa.ID); err != nil {
			tx.Rollback()
			return nil, apperrors.Internal("Failed to remove session", err)
		}

		loginPayload := userLoggedInPayload{
			KratosID: u.KratosID,
			Email:    u.Email,
		}
		evt, err := events.NewDomainEvent(
			events.UserLoggedIn,
			u.KratosID,
			"User",
			constants.EventVersion,
			correlationID,
			loginPayload,
		)
		if err == nil {
			_ = s.eventBus.Publish(ctxTx, evt)
		}

		if err := tx.Commit().Error; err != nil {
			return nil, apperrors.Internal("Failed to commit session cleanup", err)
		}
	}

	// 7. Generate tokens via Hydra with custom claims (user_id, email)
	customClaims := map[string]interface{}{
		"user_id": resolvedUserID,
		"email":   pa.Email,
	}
	tokens, err := s.orchestrator.GenerateTokens(ctx, kratosID, customClaims)
	if err != nil {
		return nil, apperrors.Internal("Failed to generate access tokens", err)
	}

	return &VerifyLoginResponse{
		AccessToken:  tokens.AccessToken,
		ExpiresIn:    tokens.ExpiresIn,
		RefreshToken: tokens.RefreshToken,
	}, nil
}
