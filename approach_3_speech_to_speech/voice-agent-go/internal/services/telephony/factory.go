package telephony

import (
	"fmt"
	"log"
	"strings"
	"sync"

	"voice-agent-go/internal/config"
)

// TelephonyFactory manages the registry of telephony providers adhering to the Open-Closed Principle (SOLID).
// New providers (e.g. Tata Telecommunications, Airtel Cloud, Bandwidth) can be registered without modifying existing orchestrator code.
type TelephonyFactory struct {
	providers map[string]TelephonyProvider
	mu        sync.RWMutex
}

var (
	defaultFactory *TelephonyFactory
	once           sync.Once
)

// GetFactory returns the singleton instance of the TelephonyFactory
func GetFactory() *TelephonyFactory {
	once.Do(func() {
		defaultFactory = &TelephonyFactory{
			providers: make(map[string]TelephonyProvider),
		}
		// Register built-in providers automatically
		defaultFactory.RegisterProvider(NewExotelProvider())
		defaultFactory.RegisterProvider(NewTwilioProvider())
		defaultFactory.RegisterProvider(NewPlivoProvider())
		defaultFactory.RegisterProvider(NewTelnyxProvider())
	})
	return defaultFactory
}

// RegisterProvider registers or overrides a provider in the factory
func (f *TelephonyFactory) RegisterProvider(provider TelephonyProvider) {
	if provider == nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	name := strings.ToLower(provider.GetProviderName())
	f.providers[name] = provider
	log.Printf("[Telephony Factory] Registered telephony provider: %s", name)
}

// GetProvider retrieves a specific provider by name (e.g., "exotel", "twilio", "plivo", "telnyx").
// If name is empty, it resolves the default provider configured in config (e.g., DEFAULT_TELEPHONY_PROVIDER="exotel").
func (f *TelephonyFactory) GetProvider(name string) (TelephonyProvider, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	target := strings.ToLower(strings.TrimSpace(name))
	if target == "" {
		target = strings.ToLower(config.Settings.DefaultTelephonyProvider)
	}
	if target == "" {
		target = "exotel" // Default to Exotel for Indian telecommunication landscape
	}

	provider, exists := f.providers[target]
	if !exists {
		log.Printf("[Telephony Factory] Warning: Provider %q not found. Falling back to Exotel.", target)
		provider, exists = f.providers["exotel"]
		if !exists {
			return nil, fmt.Errorf("telephony provider %q not registered and fallback failed", target)
		}
	}

	return provider, nil
}

// GetDefaultProvider returns the configured default TelephonyProvider
func GetDefaultProvider() (TelephonyProvider, error) {
	return GetFactory().GetProvider("")
}
