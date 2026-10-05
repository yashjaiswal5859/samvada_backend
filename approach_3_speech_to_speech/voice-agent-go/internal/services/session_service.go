package services

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"voice-agent-go/internal/db"
	"voice-agent-go/internal/models"
	"voice-agent-go/internal/services/rag"

	"github.com/google/uuid"
)

type SessionService struct {
	ragProvider rag.RAGProvider
}

func NewSessionService(ragProvider rag.RAGProvider) *SessionService {
	return &SessionService{
		ragProvider: ragProvider,
	}
}

func (s *SessionService) CreateSession(sessionID string, companyName string, tenantIDStr string, phone string, direction string) (*models.Call, error) {
	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		tenantID = uuid.New()
	}

	call := &models.Call{
		ID:        uuid.New(),
		TenantID:  tenantID,
		SessionID: sessionID,
		Direction: direction,
		Status:    "active",
		StartedAt: func() *time.Time { t := time.Now(); return &t }(),
	}

	// 1. Fetch User Info & Active Orders from CustomerService at call start
	customerSvc := GetCustomerService()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	customer, err := customerSvc.GetCustomerByPhone(ctx, phone)
	if err != nil {
		log.Printf("[Session Pre-Fetch] Warning: failed to fetch customer info for phone %s: %v", phone, err)
	} else {
		log.Printf("👤 [Session Pre-Fetch Success] Loaded Caller Profile:\n"+
			"  ├─ Name:             %s (%s)\n"+
			"  ├─ Swiggy One:       %v\n"+
			"  ├─ Preferred Addr:   %s\n"+
			"  └─ Active Order #%s:  %s (%s, ETA: %s)",
			customer.Name, customer.Phone, customer.IsSwiggyOne, customer.PreferredAddress,
			func() string {
				if customer.ActiveOrder != nil {
					return customer.ActiveOrder.OrderID
				}
				return "N/A"
			}(),
			func() string {
				if customer.ActiveOrder != nil {
					return customer.ActiveOrder.ItemName
				}
				return "No active order"
			}(),
			func() string {
				if customer.ActiveOrder != nil {
					return customer.ActiveOrder.Status
				}
				return "N/A"
			}(),
			func() string {
				if customer.ActiveOrder != nil {
					return customer.ActiveOrder.ETA
				}
				return "N/A"
			}(),
		)
	}

	// 2. Persist session starting in DB (PostgreSQL) if available
	if db.DB != nil {
		if err := db.DB.Create(call).Error; err != nil {
			log.Printf("[Session] Warning: failed to save call session in DB: %v", err)
		}
	}

	// 3. Initialize RAG on static policies (refund_policy, cancellation_policy)
	if err := s.ragProvider.Initialize(sessionID, companyName, tenantIDStr, phone); err != nil {
		log.Printf("[Session] Warning: failed to initialize RAG session: %v", err)
	}

	// 4. Store active session metadata & customer JSON in Redis
	if db.RedisClient != nil {
		redisCtx := context.Background()
		custJSON, _ := json.Marshal(customer)

		_ = db.RedisClient.HSet(redisCtx, "session:"+sessionID+":state", map[string]interface{}{
			"phone":           phone,
			"tenant_id":        tenantIDStr,
			"status":          "active",
			"customer_json":   string(custJSON),
			"customer_name":   customer.Name,
			"is_swiggy_one":   customer.IsSwiggyOne,
		}).Err()
		_ = db.RedisClient.Expire(redisCtx, "session:"+sessionID+":state", 24*time.Hour).Err()
	}

	log.Printf("[Session] Active session initialized & bound to customer profile: %s (Caller: %s)", sessionID, customer.Name)
	return call, nil
}

func (s *SessionService) EndSession(sessionID string, phone string, tenantIDStr string, dynamicMemories []string) error {
	ctx := context.Background()

	// Update call status in Postgres
	var call models.Call
	if db.DB != nil {
		if err := db.DB.Where("session_id = ?", sessionID).First(&call).Error; err == nil {
			now := time.Now()
			call.Status = "completed"
			call.EndedAt = &now
			if call.StartedAt != nil {
				call.DurationSec = int(now.Sub(*call.StartedAt).Seconds())
			}
			db.DB.Save(&call)

			if len(dynamicMemories) > 0 {
				if err := s.ragProvider.Persist(sessionID, phone, tenantIDStr, dynamicMemories, call.ID.String()); err != nil {
					log.Printf("[Session] Warning: failed to persist dynamic memories: %v", err)
				}

				tenantUUID, _ := uuid.Parse(tenantIDStr)
				for _, memory := range dynamicMemories {
					memRecord := &models.CustomerMemory{
						TenantID:     tenantUUID,
						Phone:        phone,
						MemoryText:   memory,
						SourceCallID: call.ID,
					}
					db.DB.Create(memRecord)
				}
			}
		}
	}

	// Cleanup RAG session index
	if err := s.ragProvider.Destroy(sessionID); err != nil {
		log.Printf("[Session] Warning: failed to destroy RAG session: %v", err)
	}

	// Cleanup active Redis state after a 5-minute cooldown period
	if db.RedisClient != nil {
		_ = db.RedisClient.Expire(ctx, "session:"+sessionID+":state", 5*time.Minute).Err()
	}

	log.Printf("[Session] Session ended and cleaned up: %s", sessionID)
	return nil
}
