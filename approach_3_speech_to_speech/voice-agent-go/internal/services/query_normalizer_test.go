package services_test

import (
	"testing"
	"voice-agent-go/internal/services"
)

func TestQueryNormalizer(t *testing.T) {
	// Build vocabulary for Dmarketing test company
	vocab := services.BuildDocVocabulary("Dmarketing")
	if len(vocab.Words) == 0 {
		t.Fatalf("Expected non-empty doc vocabulary for Dmarketing")
	}

	tests := []struct {
		name     string
		input    string
		contains string
	}{
		{
			name:     "Phonetic ASR Error 'aco' to 'SEO'",
			input:    "aco package ka price kya hai",
			contains: "SEO",
		},
		{
			name:     "Phonetic ASR Error 'geegel' to 'Google'",
			input:    "geegel ads rate kya hai",
			contains: "Google",
		},
		{
			name:     "Devanagari Hindi 'एसईओ' to 'SEO'",
			input:    "आपका एसईओ पैकेज",
			contains: "SEO",
		},
		{
			name:     "Misspelled 'pakej' to 'package'",
			input:    "silver pakej ka rate",
			contains: "package",
		},
		{
			name:     "Misspelled 'prais' to 'price'",
			input:    "what is the prais",
			contains: "price",
		},
		{
			name:     "ASR Mishearing 'Pounder' to 'founder'",
			input:    "Pounder का नाम क्या है?",
			contains: "founder",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalized := services.NormalizeQuery(tt.input, vocab)
			t.Logf("Input: %q -> Normalized: %q", tt.input, normalized)
			if !containsSubstring(normalized, tt.contains) {
				t.Errorf("Expected normalized query %q to contain %q", normalized, tt.contains)
			}
		})
	}
}

func containsSubstring(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && (s == sub || searchSub(s, sub)))
}

func searchSub(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
