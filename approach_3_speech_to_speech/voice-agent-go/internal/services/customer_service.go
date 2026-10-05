package services

import (
	"context"
	"strings"
	"sync"
	"time"

	"voice-agent-go/internal/db"
	"voice-agent-go/internal/models"

	"github.com/google/uuid"
)

type ActiveOrder struct {
	OrderID         string    `json:"order_id"`
	ItemName        string    `json:"item_name"`
	Restaurant      string    `json:"restaurant"`
	Status          string    `json:"status"` // e.g. "OUT_FOR_DELIVERY", "PREPARING", "DELIVERED", "DELAYED"
	ETA             string    `json:"eta"`
	TotalAmount     float64   `json:"total_amount"`
	RiderName       string    `json:"rider_name"`
	EligibleRefund  bool      `json:"eligible_refund"`
	RefundLimitCap  float64   `json:"refund_limit_cap"`
	OrderedAt       time.Time `json:"ordered_at"`
}

type CustomerDetail struct {
	ID               string        `json:"id"`
	Phone            string        `json:"phone"`
	Name             string        `json:"name"`
	IsSwiggyOne      bool          `json:"is_swiggy_one"`
	CSATHistory      float64       `json:"csat_history"`
	TotalOrders      int           `json:"total_orders"`
	PreferredAddress string        `json:"preferred_address"`
	ActiveOrder      *ActiveOrder  `json:"active_order,omitempty"`
	Notes            string        `json:"notes,omitempty"`
}

type CustomerService struct {
	mu          sync.RWMutex
	mockData    map[string]*CustomerDetail
}

var (
	customerSvcInstance *CustomerService
	customerSvcOnce     sync.Once
)

func GetCustomerService() *CustomerService {
	customerSvcOnce.Do(func() {
		customerSvcInstance = &CustomerService{
			mockData: make(map[string]*CustomerDetail),
		}
		customerSvcInstance.seedDefaults()
	})
	return customerSvcInstance
}

func (s *CustomerService) seedDefaults() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Seed Customer 1: Rahul Sharma (+919876543210)
	s.mockData["+919876543210"] = &CustomerDetail{
		ID:               uuid.New().String(),
		Phone:            "+919876543210",
		Name:             "Rahul Sharma",
		IsSwiggyOne:      true,
		CSATHistory:      4.8,
		TotalOrders:      42,
		PreferredAddress: "Flat 402, Sunshine Apartments, Indiranagar, Bengaluru",
		Notes:            "VIP Customer. Prefers minimal calling, quick refunds if delayed.",
		ActiveOrder: &ActiveOrder{
			OrderID:        "5921",
			ItemName:       "Hyderabadi Chicken Biryani x2, Thums Up x1",
			Restaurant:     "Meghana Foods, Indiranagar",
			Status:         "OUT_FOR_DELIVERY",
			ETA:            "6 minutes",
			TotalAmount:    480.00,
			RiderName:      "Ramesh Kumar",
			EligibleRefund: true,
			RefundLimitCap: 500.00,
			OrderedAt:      time.Now().Add(-20 * time.Minute),
		},
	}

	// Seed Customer 2: Hackathon Judge (+919999999999)
	s.mockData["+919999999999"] = &CustomerDetail{
		ID:               uuid.New().String(),
		Phone:            "+919999999999",
		Name:             "Microsoft Hackathon Judge",
		IsSwiggyOne:      true,
		CSATHistory:      5.0,
		TotalOrders:      120,
		PreferredAddress: "Microsoft Campus, Building 3, Hyderabad",
		Notes:            "Executive VIP Tester for Autonomous Voice Support System.",
		ActiveOrder: &ActiveOrder{
			OrderID:        "1001",
			ItemName:       "Large Pepperoni Pizza & Garlic Bread",
			Restaurant:     "Domino's Pizza",
			Status:         "DELAYED",
			ETA:            "12 minutes (Delayed by 15 mins)",
			TotalAmount:    650.00,
			RiderName:      "Suresh V",
			EligibleRefund: true,
			RefundLimitCap: 650.00,
			OrderedAt:      time.Now().Add(-45 * time.Minute),
		},
	}
}

func NormalizePhone(phone string) string {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return ""
	}
	if !strings.HasPrefix(phone, "+") {
		if strings.HasPrefix(phone, "91") && len(phone) == 12 {
			phone = "+" + phone
		} else if len(phone) == 10 {
			phone = "+91" + phone
		}
	}
	return phone
}

func (s *CustomerService) GetCustomerByPhone(ctx context.Context, phone string) (*CustomerDetail, error) {
	normPhone := NormalizePhone(phone)

	s.mu.RLock()
	detail, exists := s.mockData[normPhone]
	s.mu.RUnlock()

	if exists {
		return detail, nil
	}

	if db.DB != nil {
		var profile models.CustomerProfile
		if err := db.DB.WithContext(ctx).Where("phone = ?", normPhone).First(&profile).Error; err == nil {
			return &CustomerDetail{
				ID:               profile.ID.String(),
				Phone:            profile.Phone,
				Name:             profile.Name,
				IsSwiggyOne:      true,
				CSATHistory:      4.5,
				TotalOrders:      10,
				PreferredAddress: "Registered Address",
				Notes:            profile.Notes,
			}, nil
		}
	}

	return &CustomerDetail{
		ID:               uuid.New().String(),
		Phone:            normPhone,
		Name:             "Valued Customer",
		IsSwiggyOne:      false,
		CSATHistory:      4.0,
		TotalOrders:      5,
		PreferredAddress: "Current Delivery Address",
		Notes:            "Standard customer support caller",
		ActiveOrder: &ActiveOrder{
			OrderID:        "8842",
			ItemName:       "Paneer Butter Masala & Butter Naan",
			Restaurant:     "Punjab Grill",
			Status:         "PREPARING",
			ETA:            "18 minutes",
			TotalAmount:    350.00,
			RiderName:      "Assigned Rider",
			EligibleRefund: true,
			RefundLimitCap: 350.00,
			OrderedAt:      time.Now().Add(-10 * time.Minute),
		},
	}, nil
}
