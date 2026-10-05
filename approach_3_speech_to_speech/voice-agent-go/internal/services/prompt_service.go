package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"voice-agent-go/internal/config"
)

type GreetingJSON struct {
	Inbound  string `json:"inbound"`
	Outbound string `json:"outbound"`
}

type PromptService struct {
	cache map[string]string
	mu    sync.RWMutex
}

var (
	promptSvcInstance *PromptService
	promptSvcOnce     sync.Once
)

func GetPromptService() *PromptService {
	promptSvcOnce.Do(func() {
		promptSvcInstance = &PromptService{
			cache: make(map[string]string),
		}
	})
	return promptSvcInstance
}

func (s *PromptService) resolveTenantDir(companyName string) string {
	if config.Settings.TenantDocsPath != "" {
		if _, err := os.Stat(config.Settings.TenantDocsPath); err == nil {
			return config.Settings.TenantDocsPath
		}
	}

	basePath := config.Settings.DocsBasePath
	if basePath == "" {
		basePath = "D:/Voice AI/docs"
	}
	if companyName == "" {
		companyName = config.Settings.DefaultCompanyName
	}

	compDir := filepath.Join(basePath, companyName)
	if _, err := os.Stat(compDir); err == nil {
		return compDir
	}

	if entries, err := os.ReadDir(basePath); err == nil {
		target := strings.ToLower(companyName)
		for _, entry := range entries {
			if entry.IsDir() && strings.ToLower(entry.Name()) == target {
				return filepath.Join(basePath, entry.Name())
			}
		}
	}

	return filepath.Join(basePath, config.Settings.DefaultCompanyName)
}

func (s *PromptService) GetGreeting(direction, companyName, customerName, language string) string {
	tenantDir := s.resolveTenantDir(companyName)
	hasName := customerName != "" && customerName != "there" && customerName != config.Settings.DefaultCustomerName

	jsonPath := filepath.Join(tenantDir, "greeting.json")
	if bytes, err := os.ReadFile(jsonPath); err == nil && len(bytes) > 0 {
		var gJSON GreetingJSON
		if err := json.Unmarshal(bytes, &gJSON); err == nil {
			selected := gJSON.Inbound
			if strings.ToLower(direction) == "outbound" && gJSON.Outbound != "" {
				selected = gJSON.Outbound
			}
			if selected != "" {
				if hasName {
					selected = strings.ReplaceAll(selected, "{name}", customerName)
					selected = strings.ReplaceAll(selected, "{CustomerName}", customerName)
				} else {
					selected = strings.ReplaceAll(selected, "{name}", "")
					selected = strings.ReplaceAll(selected, "{CustomerName}", "")
					selected = strings.ReplaceAll(selected, "  ", " ")
				}
				return strings.TrimSpace(selected)
			}
		}
	}

	txtFilename := "greeting.txt"
	if !hasName {
		txtFilename = "greeting_anonymous.txt"
	}
	if language == "hindi" {
		txtFilename = "greeting_hi.txt"
	}

	txtPath := filepath.Join(tenantDir, txtFilename)
	bytes, err := os.ReadFile(txtPath)
	if err != nil || len(bytes) == 0 {
		fallbackPath := filepath.Join(tenantDir, "greeting.txt")
		bytes, err = os.ReadFile(fallbackPath)
	}

	if err == nil && len(bytes) > 0 {
		text := strings.TrimSpace(string(bytes))
		if hasName {
			text = strings.ReplaceAll(text, "{name}", customerName)
			text = strings.ReplaceAll(text, "{CustomerName}", customerName)
		}
		return text
	}

	if hasName {
		return "[warm] Hi " + customerName + "! Thanks for calling Swiggy HDFC Credit Card Support. I'm Riya. How can I help you today?"
	}
	return "[warm] Hi! Thanks for calling Swiggy HDFC Credit Card Support. My name is Riya. How can I help you today?"
}

func (s *PromptService) GetSystemPrompt(companyName string) string {
	tenantDir := s.resolveTenantDir(companyName)
	promptPath := filepath.Join(tenantDir, "system_prompt.txt")
	bytes, err := os.ReadFile(promptPath)
	if err == nil && len(bytes) > 0 {
		return strings.TrimSpace(string(bytes))
	}

	return "You are Riya, an empathetic & efficient Customer Support Specialist for the co-branded Swiggy HDFC Bank Credit Card. Your role is to assist callers with Swiggy orders, 10% cashback credits, statement queries, card benefits, fee waivers, and card support. Do NOT pitch any products or sales offers. Always check the caller's active order details, card benefits, and company policies. Keep your answers clear, concise (1-2 sentences), and warm."
}
