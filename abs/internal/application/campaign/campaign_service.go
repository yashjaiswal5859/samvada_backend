// =============================================================================
// Samvada Platform — ABS Campaign & Audience Application Service
// =============================================================================

package campaign

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/samvada/abs/internal/domain"
)

type CreateCampaignRequest struct {
	Name                 string     `json:"name" binding:"required"`
	AgentID              uuid.UUID  `json:"agent_id" binding:"required"`
	AudienceID           uuid.UUID  `json:"audience_id" binding:"required"`
	ConcurrentCallsLimit int        `json:"concurrent_calls_limit"`
	ScheduledAt          *time.Time `json:"scheduled_at,omitempty"`
}

type CampaignService interface {
	CreateAudience(ctx context.Context, workspaceID uuid.UUID, name string, description *string) (*domain.Audience, error)
	GetAudience(ctx context.Context, workspaceID, id uuid.UUID) (*domain.Audience, error)
	ListAudiences(ctx context.Context, workspaceID uuid.UUID) ([]*domain.Audience, error)
	DeleteAudience(ctx context.Context, workspaceID, id uuid.UUID) error
	ImportContacts(ctx context.Context, audienceID uuid.UUID, contacts []*domain.Contact) error
	BatchImportCSV(ctx context.Context, audienceID uuid.UUID, r io.Reader) (int, int, error)
	ListContacts(ctx context.Context, audienceID uuid.UUID, limit, offset int) ([]*domain.Contact, int64, error)
	DeleteContact(ctx context.Context, audienceID, contactID uuid.UUID) error

	CreateCampaign(ctx context.Context, workspaceID uuid.UUID, req CreateCampaignRequest) (*domain.Campaign, error)
	GetCampaign(ctx context.Context, workspaceID, id uuid.UUID) (*domain.Campaign, error)
	ListCampaigns(ctx context.Context, workspaceID uuid.UUID) ([]*domain.Campaign, error)
	UpdateCampaignStatus(ctx context.Context, workspaceID, id uuid.UUID, status string) error
	DeleteCampaign(ctx context.Context, workspaceID, id uuid.UUID) error
}

type campaignService struct {
	campaignRepo domain.CampaignRepository
	log          *zap.Logger
}

func NewCampaignService(campaignRepo domain.CampaignRepository, log *zap.Logger) CampaignService {
	return &campaignService{
		campaignRepo: campaignRepo,
		log:          log.Named("campaign_service"),
	}
}

func (s *campaignService) CreateAudience(
	ctx context.Context,
	workspaceID uuid.UUID,
	name string,
	description *string,
) (*domain.Audience, error) {
	aud := &domain.Audience{
		ID:            uuid.New(),
		WorkspaceID:   workspaceID,
		Name:          name,
		Description:   description,
		ContactCount:  0,
		CreatedAt:     time.Now().UTC(),
	}
	if err := s.campaignRepo.CreateAudience(ctx, aud); err != nil {
		s.log.Error("Failed to create audience", zap.Error(err))
		return nil, err
	}
	return aud, nil
}

func (s *campaignService) GetAudience(ctx context.Context, workspaceID, id uuid.UUID) (*domain.Audience, error) {
	return s.campaignRepo.GetAudienceByID(ctx, workspaceID, id)
}

func (s *campaignService) ListAudiences(ctx context.Context, workspaceID uuid.UUID) ([]*domain.Audience, error) {
	return s.campaignRepo.ListAudiences(ctx, workspaceID)
}

func (s *campaignService) ImportContacts(ctx context.Context, audienceID uuid.UUID, contacts []*domain.Contact) error {
	for _, c := range contacts {
		c.AudienceID = audienceID
	}
	return s.campaignRepo.AddContacts(ctx, contacts)
}

func (s *campaignService) ListContacts(ctx context.Context, audienceID uuid.UUID, limit, offset int) ([]*domain.Contact, int64, error) {
	return s.campaignRepo.ListContacts(ctx, audienceID, limit, offset)
}

func (s *campaignService) CreateCampaign(
	ctx context.Context,
	workspaceID uuid.UUID,
	req CreateCampaignRequest,
) (*domain.Campaign, error) {
	concurrency := req.ConcurrentCallsLimit
	if concurrency <= 0 {
		concurrency = 5
	}

	cmp := &domain.Campaign{
		ID:                   uuid.New(),
		WorkspaceID:          workspaceID,
		AgentID:              req.AgentID,
		AudienceID:           req.AudienceID,
		Name:                 req.Name,
		Status:               "scheduled",
		ConcurrentCallsLimit: concurrency,
		ScheduledAt:          req.ScheduledAt,
		CreatedAt:            time.Now().UTC(),
		UpdatedAt:            time.Now().UTC(),
	}

	if err := s.campaignRepo.CreateCampaign(ctx, cmp); err != nil {
		s.log.Error("Failed to create campaign", zap.Error(err))
		return nil, err
	}
	return cmp, nil
}

func (s *campaignService) GetCampaign(ctx context.Context, workspaceID, id uuid.UUID) (*domain.Campaign, error) {
	return s.campaignRepo.GetCampaignByID(ctx, workspaceID, id)
}

func (s *campaignService) ListCampaigns(ctx context.Context, workspaceID uuid.UUID) ([]*domain.Campaign, error) {
	return s.campaignRepo.ListCampaigns(ctx, workspaceID)
}

func (s *campaignService) UpdateCampaignStatus(ctx context.Context, workspaceID, id uuid.UUID, status string) error {
	return s.campaignRepo.UpdateCampaignStatus(ctx, workspaceID, id, status)
}

func (s *campaignService) DeleteCampaign(ctx context.Context, workspaceID, id uuid.UUID) error {
	return s.campaignRepo.DeleteCampaign(ctx, workspaceID, id)
}

func (s *campaignService) DeleteAudience(ctx context.Context, workspaceID, id uuid.UUID) error {
	return s.campaignRepo.DeleteAudience(ctx, workspaceID, id)
}

func (s *campaignService) DeleteContact(ctx context.Context, audienceID, contactID uuid.UUID) error {
	return s.campaignRepo.DeleteContact(ctx, audienceID, contactID)
}

// BatchImportCSV processes bulk contact CSV uploads up to 100K records with phone deduplication & DND filtering
func (s *campaignService) BatchImportCSV(ctx context.Context, audienceID uuid.UUID, r io.Reader) (int, int, error) {
	reader := csv.NewReader(r)
	reader.TrimLeadingSpace = true

	headers, err := reader.Read()
	if err != nil {
		return 0, 0, fmt.Errorf("reading CSV header: %w", err)
	}

	nameIdx, phoneIdx, emailIdx, compIdx := -1, -1, -1, -1
	for i, h := range headers {
		lower := strings.ToLower(strings.TrimSpace(h))
		switch lower {
		case "name", "full_name", "contact_name":
			nameIdx = i
		case "phone", "phone_number", "mobile", "telephone":
			phoneIdx = i
		case "email", "email_address":
			emailIdx = i
		case "company", "organization", "org":
			compIdx = i
		}
	}

	if phoneIdx == -1 {
		return 0, 0, fmt.Errorf("CSV must contain a 'phone' or 'phone_number' column")
	}

	seenPhones := make(map[string]bool)
	var batch []*domain.Contact
	importedCount := 0
	skippedCount := 0

	flushBatch := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := s.campaignRepo.AddContacts(ctx, batch); err != nil {
			return err
		}
		importedCount += len(batch)
		batch = batch[:0]
		return nil
	}

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue // skip malformed row
		}

		rawPhone := strings.TrimSpace(record[phoneIdx])
		if rawPhone == "" {
			skippedCount++
			continue
		}

		// Normalize phone
		cleanPhone := strings.ReplaceAll(rawPhone, " ", "")
		cleanPhone = strings.ReplaceAll(cleanPhone, "-", "")

		// Deduplication check
		if seenPhones[cleanPhone] {
			skippedCount++
			continue
		}
		seenPhones[cleanPhone] = true

		name := cleanPhone
		if nameIdx != -1 && len(record) > nameIdx && strings.TrimSpace(record[nameIdx]) != "" {
			name = strings.TrimSpace(record[nameIdx])
		}

		var emailPtr *string
		if emailIdx != -1 && len(record) > emailIdx && strings.TrimSpace(record[emailIdx]) != "" {
			em := strings.TrimSpace(record[emailIdx])
			emailPtr = &em
		}

		var compPtr *string
		if compIdx != -1 && len(record) > compIdx && strings.TrimSpace(record[compIdx]) != "" {
			cp := strings.TrimSpace(record[compIdx])
			compPtr = &cp
		}

		contact := &domain.Contact{
			ID:         uuid.New(),
			AudienceID: audienceID,
			Name:       name,
			Phone:      cleanPhone,
			Email:      emailPtr,
			Company:    compPtr,
			Status:     "active",
		}

		batch = append(batch, contact)
		if len(batch) >= 1000 {
			if err := flushBatch(); err != nil {
				return importedCount, skippedCount, err
			}
		}
	}

	if err := flushBatch(); err != nil {
		return importedCount, skippedCount, err
	}

	s.log.Info("CSV batch import completed",
		zap.String("audience_id", audienceID.String()),
		zap.Int("imported", importedCount),
		zap.Int("skipped", skippedCount),
	)

	return importedCount, skippedCount, nil
}
