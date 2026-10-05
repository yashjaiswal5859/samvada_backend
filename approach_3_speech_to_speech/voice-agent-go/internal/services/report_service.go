package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"voice-agent-go/internal/db"
	"voice-agent-go/internal/models"

	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
)

type TurnAnalytics struct {
	TurnNumber          int       `json:"turn_number"`
	UserQuery           string    `json:"user_query"`
	AgentResponse       string    `json:"agent_response"`
	Emotion             string    `json:"emotion"`
	FrustrationLevel    string    `json:"frustration_level"`
	InterestCategory    string    `json:"interest_category"`
	SentimentScore      float64   `json:"sentiment_score"`
	CSATEstimate        float64   `json:"csat_estimate"`
	DeescalationApplied bool      `json:"deescalation_applied"`
	Timestamp           time.Time `json:"timestamp"`
}

type CallReportSummary struct {
	SessionID            string          `json:"session_id"`
	Phone                string          `json:"phone"`
	CustomerName         string          `json:"customer_name"`
	TotalTurns           int             `json:"total_turns"`
	InitialEmotion       string          `json:"initial_emotion"`
	FinalEmotion         string          `json:"final_emotion"`
	OverallFrustration   string          `json:"overall_frustration"`
	PrimaryInterest      string          `json:"primary_interest"`
	AverageCSAT          float64         `json:"average_csat"`
	DeescalationCount    int             `json:"deescalation_count"`
	RequiresHumanHandoff bool            `json:"requires_human_handoff"`
	TurnHistory          []TurnAnalytics `json:"turn_history"`
	GeneratedAt          time.Time       `json:"generated_at"`
}

type EscalationItem struct {
	ID               uint      `json:"id"`
	SessionID        string    `json:"session_id"`
	Phone            string    `json:"phone"`
	CustomerName     string    `json:"customer_name"`
	FrustrationLevel string    `json:"frustration_level"`
	Emotion          string    `json:"emotion"`
	Reason           string    `json:"reason"`
	CSATRating       float64   `json:"csat_rating"`
	Status           string    `json:"status"`
	CreatedAt        time.Time `json:"created_at"`
}

type ReportService struct {
	mu                  sync.RWMutex
	sessionReport       map[string]*CallReportSummary
	pendingEscalations  []*EscalationItem
	escalationCounter   uint
	kafkaWriter         *kafka.Writer
}

var (
	reportSvcInstance *ReportService
	reportSvcOnce     sync.Once
)

func GetReportService() *ReportService {
	reportSvcOnce.Do(func() {
		var kw *kafka.Writer
		if db.KafkaWriter != nil {
			kw = &kafka.Writer{
				Addr:     db.KafkaWriter.Addr,
				Topic:    "call.analytics",
				Balancer: &kafka.LeastBytes{},
			}
		}
		reportSvcInstance = &ReportService{
			sessionReport:      make(map[string]*CallReportSummary),
			pendingEscalations: make([]*EscalationItem, 0),
			kafkaWriter:        kw,
		}
		reportSvcInstance.seedSampleEscalation()
	})
	return reportSvcInstance
}

func (s *ReportService) seedSampleEscalation() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.escalationCounter++
	sample := &EscalationItem{
		ID:               s.escalationCounter,
		SessionID:        "demo-session-9912",
		Phone:            "+919876543210",
		CustomerName:     "Rahul Sharma",
		FrustrationLevel: "HIGH",
		Emotion:          "frustrated",
		Reason:           "Order #5921 delayed >30 mins. Customer unhappy with initial ETA.",
		CSATRating:       2.1,
		Status:           "PENDING_CALL_BACK",
		CreatedAt:        time.Now().Add(-15 * time.Minute),
	}
	s.pendingEscalations = append(s.pendingEscalations, sample)
}

func (s *ReportService) LogTurnAnalytics(ctx context.Context, sessionID, phone, customerName, userQuery, agentResponse, emotion, interest string, csat float64, deescalation bool) *TurnAnalytics {
	s.mu.Lock()
	defer s.mu.Unlock()

	summary, exists := s.sessionReport[sessionID]
	if !exists {
		summary = &CallReportSummary{
			SessionID:            sessionID,
			Phone:                phone,
			CustomerName:         customerName,
			TotalTurns:           0,
			InitialEmotion:       emotion,
			FinalEmotion:         emotion,
			OverallFrustration:   "LOW",
			PrimaryInterest:      interest,
			AverageCSAT:          csat,
			DeescalationCount:    0,
			RequiresHumanHandoff: false,
			TurnHistory:          make([]TurnAnalytics, 0),
			GeneratedAt:          time.Now(),
		}
		s.sessionReport[sessionID] = summary
	}

	summary.TotalTurns++
	summary.FinalEmotion = emotion
	if interest != "" && interest != "general" {
		summary.PrimaryInterest = interest
	}
	if deescalation {
		summary.DeescalationCount++
	}

	totalCSAT := 0.0
	for _, t := range summary.TurnHistory {
		totalCSAT += t.CSATEstimate
	}
	totalCSAT += csat
	summary.AverageCSAT = totalCSAT / float64(summary.TotalTurns)

	frustrationLevel := "LOW"
	if emotion == "angry" || csat < 2.5 {
		frustrationLevel = "HIGH"
	} else if emotion == "frustrated" || csat < 3.5 {
		frustrationLevel = "MEDIUM"
	}
	summary.OverallFrustration = frustrationLevel

	sentScore := 0.1
	switch emotion {
	case "angry":
		sentScore = -0.9
	case "frustrated":
		sentScore = -0.6
	case "happy":
		sentScore = +0.8
	}

	if frustrationLevel == "HIGH" && !summary.RequiresHumanHandoff {
		summary.RequiresHumanHandoff = true
		s.escalationCounter++

		escalation := &EscalationItem{
			ID:               s.escalationCounter,
			SessionID:        sessionID,
			Phone:            phone,
			CustomerName:     customerName,
			FrustrationLevel: "HIGH",
			Emotion:          emotion,
			Reason:           fmt.Sprintf("Customer expressed %s emotion on turn %d. User Query: %q", emotion, summary.TotalTurns, userQuery),
			CSATRating:       summary.AverageCSAT,
			Status:           "PENDING_CALL_BACK",
			CreatedAt:        time.Now(),
		}

		s.pendingEscalations = append(s.pendingEscalations, escalation)

		log.Printf("⚠️ [ESCALATION QUEUED] Customer %s (%s) flagged for Human Call-Back! Frustration: HIGH | CSAT: %.1f | Reason: %s",
			customerName, phone, summary.AverageCSAT, escalation.Reason)

		go s.publishKafkaEvent(sessionID, "call.escalation.created", escalation)
		go s.persistEscalationToDB(escalation)
	}

	turn := TurnAnalytics{
		TurnNumber:          summary.TotalTurns,
		UserQuery:           userQuery,
		AgentResponse:       agentResponse,
		Emotion:             emotion,
		FrustrationLevel:    frustrationLevel,
		InterestCategory:    interest,
		SentimentScore:      sentScore,
		CSATEstimate:        csat,
		DeescalationApplied: deescalation,
		Timestamp:           time.Now(),
	}

	summary.TurnHistory = append(summary.TurnHistory, turn)
	summary.GeneratedAt = time.Now()

	go s.publishKafkaEvent(sessionID, "call.turn.analytics", turn)
	go s.persistTimelineToDB(sessionID, summary)

	log.Printf("[Report Service] Session %s | Turn %d logged: Emotion=%s, Frustration=%s, Interest=%s, CSAT=%.1f, Deescalation=%v",
		sessionID, turn.TurnNumber, emotion, frustrationLevel, interest, csat, deescalation)

	return &turn
}

func (s *ReportService) GetPendingEscalations() []*EscalationItem {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if db.DB != nil {
		var dbRecords []models.EscalationRecord
		if err := db.DB.Where("status = ?", "PENDING_CALL_BACK").Find(&dbRecords).Error; err == nil && len(dbRecords) > 0 {
			resultMap := make(map[string]*EscalationItem)
			for _, item := range s.pendingEscalations {
				resultMap[item.SessionID] = item
			}
			for _, rec := range dbRecords {
				if _, exists := resultMap[rec.SessionID]; !exists {
					item := &EscalationItem{
						ID:               rec.ID,
						SessionID:        rec.SessionID,
						Phone:            rec.Phone,
						CustomerName:     rec.CustomerName,
						FrustrationLevel: rec.FrustrationLevel,
						Emotion:          rec.Emotion,
						Reason:           rec.Reason,
						CSATRating:       rec.CSATRating,
						Status:           rec.Status,
						CreatedAt:        rec.CreatedAt,
					}
					resultMap[rec.SessionID] = item
				}
			}
			list := make([]*EscalationItem, 0, len(resultMap))
			for _, v := range resultMap {
				list = append(list, v)
			}
			return list
		}
	}

	return s.pendingEscalations
}

func (s *ReportService) ResolveEscalation(id uint) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, item := range s.pendingEscalations {
		if item.ID == id {
			item.Status = "COMPLETED"
			log.Printf("✅ [ESCALATION RESOLVED] Human supervisor completed call-back for %s (%s)", item.CustomerName, item.Phone)

			if db.DB != nil {
				_ = db.DB.Model(&models.EscalationRecord{}).Where("id = ?", id).Update("status", "COMPLETED").Error
			}
			return true
		}
	}
	return false
}

func (s *ReportService) publishKafkaEvent(sessionID, eventType string, payload interface{}) {
	if s.kafkaWriter == nil && db.KafkaWriter == nil {
		return
	}
	writer := s.kafkaWriter
	if writer == nil {
		writer = db.KafkaWriter
	}

	eventData, err := json.Marshal(map[string]interface{}{
		"event_type": eventType,
		"session_id": sessionID,
		"data":       payload,
		"timestamp":  time.Now().Format(time.RFC3339),
	})
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_ = writer.WriteMessages(ctx, kafka.Message{
		Key:   []byte(sessionID),
		Value: eventData,
	})
}

func (s *ReportService) persistTimelineToDB(sessionID string, summary *CallReportSummary) {
	if db.DB == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	timelineJSON, err := json.Marshal(summary)
	if err != nil {
		return
	}

	callUUID, _ := uuid.Parse(sessionID)
	if callUUID == uuid.Nil {
		callUUID = uuid.New()
	}

	timelineRecord := models.EmotionTimeline{
		CallID:    callUUID,
		Timeline:  string(timelineJSON),
		Timestamp: time.Now(),
	}

	_ = db.DB.WithContext(ctx).Save(&timelineRecord).Error
}

func (s *ReportService) persistEscalationToDB(esc *EscalationItem) {
	if db.DB == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	rec := models.EscalationRecord{
		SessionID:        esc.SessionID,
		Phone:            esc.Phone,
		CustomerName:     esc.CustomerName,
		FrustrationLevel: esc.FrustrationLevel,
		Emotion:          esc.Emotion,
		Reason:           esc.Reason,
		CSATRating:       esc.CSATRating,
		Status:           esc.Status,
		CreatedAt:        esc.CreatedAt,
	}

	_ = db.DB.WithContext(ctx).Create(&rec).Error
}

func (s *ReportService) GetReport(sessionID string) (*CallReportSummary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	summary, exists := s.sessionReport[sessionID]
	if !exists {
		return nil, fmt.Errorf("no report found for session_id %s", sessionID)
	}
	return summary, nil
}
