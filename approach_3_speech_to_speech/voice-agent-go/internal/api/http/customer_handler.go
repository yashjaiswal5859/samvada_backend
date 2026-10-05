package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"voice-agent-go/internal/services"
)

type CustomerAPIHandler struct {
	customerService *services.CustomerService
}

func NewCustomerAPIHandler(customerService *services.CustomerService) *CustomerAPIHandler {
	return &CustomerAPIHandler{
		customerService: customerService,
	}
}

func (h *CustomerAPIHandler) HandleGetCustomer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed. Use GET.", http.StatusMethodNotAllowed)
		return
	}

	phone := r.URL.Query().Get("phone")
	if phone == "" {
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) > 0 {
			phone = parts[len(parts)-1]
		}
	}

	if phone == "" || phone == "customer" {
		phone = "+919876543210"
	}

	customer, err := h.customerService.GetCustomerByPhone(r.Context(), phone)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": fmt.Sprintf("Failed to fetch customer: %v", err),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(customer)
}
