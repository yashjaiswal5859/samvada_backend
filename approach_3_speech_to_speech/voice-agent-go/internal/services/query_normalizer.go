package services

import (
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"unicode"

	"voice-agent-go/internal/config"
)

// DocVocabulary holds the domain vocabulary extracted strictly from company RAG documents
type DocVocabulary struct {
	CompanyName   string
	Words         map[string]bool   // Lowercase word set: {"seo": true, "google": true, ...}
	OriginalCase  map[string]string // Lowercase -> Original Case: {"seo": "SEO", "google": "Google"}
	PhoneticIndex map[string][]string
}

var (
	vocabCache = make(map[string]*DocVocabulary)
	vocabMu    sync.RWMutex
	cleanRegexp = regexp.MustCompile(`[^a-zA-Z0-9\s]`)
)

// Common English & Hindi stop words to exclude from domain doc vocabulary
var stopWords = map[string]bool{
	"the": true, "is": true, "at": true, "which": true, "on": true, "and": true,
	"a": true, "an": true, "in": true, "to": true, "for": true, "of": true,
	"with": true, "you": true, "your": true, "we": true, "our": true, "are": true,
	"this": true, "that": true, "or": true, "be": true, "as": true, "by": true,
	"ka": true, "ki": true, "ke": true, "hai": true, "hain": true, "kya": true,
	"ho": true, "se": true, "me": true, "mein": true, "par": true, "bhi": true,
}

// InvalidateVocabCache clears the cached vocabulary for a company so new docs are re-scanned automatically
func InvalidateVocabCache(companyName string) {
	vocabMu.Lock()
	defer vocabMu.Unlock()
	delete(vocabCache, companyName)
	log.Printf("🔄 [Query Normalizer] Cleared vocabulary cache for %q (will re-scan on next call)", companyName)
}

// BuildDocVocabulary parses all company RAG docs (.txt files) and builds the domain vocabulary index
func BuildDocVocabulary(companyName string) *DocVocabulary {
	vocabMu.RLock()
	if v, ok := vocabCache[companyName]; ok {
		vocabMu.RUnlock()
		return v
	}
	vocabMu.RUnlock()

	vocabMu.Lock()
	defer vocabMu.Unlock()

	// Double check cache under write lock
	if v, ok := vocabCache[companyName]; ok {
		return v
	}

	docsBase := config.Settings.DocsBasePath
	if docsBase == "" {
		docsBase = "D:/Voice AI/docs"
	}
	compDir := resolveCompanyDir(docsBase, companyName)

	v := &DocVocabulary{
		CompanyName:   companyName,
		Words:         make(map[string]bool),
		OriginalCase:  make(map[string]string),
		PhoneticIndex: make(map[string][]string),
	}

	entries, err := os.ReadDir(compDir)
	if err != nil {
		log.Printf("[Query Normalizer] Warning: Cannot read docs directory %s: %v", compDir, err)
		vocabCache[companyName] = v
		return v
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".txt" {
			continue
		}
		// Exclude non-RAG prompt files
		if entry.Name() == "system_prompt.txt" || entry.Name() == "greeting.txt" {
			continue
		}

		filePath := filepath.Join(compDir, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		// Tokenize words
		content := string(data)
		words := strings.Fields(content)
		for _, w := range words {
			cleanW := cleanRegexp.ReplaceAllString(w, "")
			if len(cleanW) < 2 {
				continue
			}

			lowerW := strings.ToLower(cleanW)
			if stopWords[lowerW] {
				continue
			}

			// Store in doc vocabulary
			v.Words[lowerW] = true
			if _, exists := v.OriginalCase[lowerW]; !exists || isAllUpper(cleanW) {
				v.OriginalCase[lowerW] = cleanW
			}

			// Add to phonetic index
			sndx := Soundex(lowerW)
			if sndx != "" {
				v.PhoneticIndex[sndx] = appendUnique(v.PhoneticIndex[sndx], lowerW)
			}
		}
	}

	log.Printf("📚 [Query Normalizer] Built RAG Doc Vocabulary for %q: %d unique domain terms indexed.",
		companyName, len(v.Words))
	vocabCache[companyName] = v
	return v
}

// NormalizeQuery converts Devanagari Hindi, corrects ASR phonetic misspellings ("aco" -> "SEO"),
// and maps terms strictly against the RAG document vocabulary.
func NormalizeQuery(query string, vocab *DocVocabulary) string {
	if query == "" || vocab == nil || len(vocab.Words) == 0 {
		return query
	}

	// 1. Transliterate Devanagari script to Latin phonetics (Sub-0.01ms CPU memory)
	transliterated := DevanagariToLatin(query)

	// 2. Tokenize words
	rawWords := strings.Fields(transliterated)
	normalizedWords := make([]string, len(rawWords))

	for i, word := range rawWords {
		cleanWord := cleanRegexp.ReplaceAllString(word, "")
		punctSuffix := ""
		if idx := strings.IndexAny(word, ".,!?"); idx != -1 {
			punctSuffix = word[idx:]
		}

		if len(cleanWord) < 2 {
			normalizedWords[i] = word
			continue
		}

		lowerWord := strings.ToLower(cleanWord)

		// 1. Skip stop words (conversational words: kya, hai, ka, is, the, what, etc.) -> Keep as-is
		if stopWords[lowerWord] {
			normalizedWords[i] = word
			continue
		}

		// 2. Direct Phonetic & ASR Misspelling Alias Overrides for Domain Terms
		if alias, ok := map[string]string{
			"aco": "SEO", "eso": "SEO", "esso": "SEO", "eseeo": "SEO", "eseeeo": "SEO",
			"geegel": "Google", "gegle": "Google", "gogle": "Google",
			"pakej": "package", "paikej": "package", "prakaj": "package", "pakaj": "package",
			"prais": "price", "prays": "price", "pris": "price",
			"pounder": "founder", "pounders": "founder", "faunder": "founder", "fonder": "founder", "bounder": "founder", "paunder": "founder",
		}[lowerWord]; ok {
			normalizedWords[i] = alias + punctSuffix
			continue
		}

		// Check A: Exact match in RAG doc vocabulary
		if vocab.Words[lowerWord] {
			orig := vocab.OriginalCase[lowerWord]
			if orig == "" {
				orig = lowerWord
			}
			normalizedWords[i] = orig + punctSuffix
			continue
		}

		// Check B: Levenshtein distance <= 2 against RAG doc vocabulary (for words >= 4 chars)
		if len(lowerWord) >= 4 {
			bestMatch := ""
			bestDist := 99
			for docWord := range vocab.Words {
				if stopWords[docWord] || len(docWord) < 4 {
					continue
				}
				if absInt(len(docWord)-len(lowerWord)) > 2 {
					continue
				}

				dist := LevenshteinDistance(lowerWord, docWord)
				maxAllowedDist := 1
				if len(lowerWord) >= 6 || len(docWord) >= 6 {
					maxAllowedDist = 2
				}

				if dist <= maxAllowedDist {
					if dist < bestDist {
						bestDist = dist
						bestMatch = docWord
					} else if dist == bestDist && bestMatch != "" {
						if Soundex(lowerWord) == Soundex(docWord) {
							bestMatch = docWord
						}
					}
				}
			}

			if bestMatch != "" {
				orig := vocab.OriginalCase[bestMatch]
				if orig == "" {
					orig = bestMatch
				}
				normalizedWords[i] = orig + punctSuffix
				continue
			}
		}

		// Check D: No match in RAG docs (e.g. conversational words like "kya", "hai") -> Keep as-is
		normalizedWords[i] = word
	}

	return strings.Join(normalizedWords, " ")
}

// DevanagariToLatin performs sub-microsecond in-memory character transliteration for Hindi text
func DevanagariToLatin(input string) string {
	hasDevanagari := false
	for _, r := range input {
		if unicode.Is(unicode.Devanagari, r) {
			hasDevanagari = true
			break
		}
	}
	if !hasDevanagari {
		return input
	}

	var sb strings.Builder
	for _, r := range input {
		strKey := string(r)
		if val, ok := devanagariMap[strKey]; ok {
			sb.WriteString(val)
		} else if unicode.Is(unicode.Devanagari, r) {
			// Skip unmapped Devanagari diacritics
			continue
		} else {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// Soundex converts a word into standard 4-character Soundex phonetic key
func Soundex(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	if len(s) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteByte(s[0])

	var prevCode byte
	switch s[0] {
	case 'B', 'F', 'P', 'V':
		prevCode = '1'
	case 'C', 'G', 'J', 'K', 'Q', 'S', 'X', 'Z':
		prevCode = '2'
	case 'D', 'T':
		prevCode = '3'
	case 'L':
		prevCode = '4'
	case 'M', 'N':
		prevCode = '5'
	case 'R':
		prevCode = '6'
	}

	for i := 1; i < len(s); i++ {
		var code byte
		switch s[i] {
		case 'B', 'F', 'P', 'V':
			code = '1'
		case 'C', 'G', 'J', 'K', 'Q', 'S', 'X', 'Z':
			code = '2'
		case 'D', 'T':
			code = '3'
		case 'L':
			code = '4'
		case 'M', 'N':
			code = '5'
		case 'R':
			code = '6'
		}

		if code != 0 && code != prevCode {
			sb.WriteByte(code)
			if sb.Len() == 4 {
				break
			}
		}
		if code != 0 {
			prevCode = code
		}
	}

	for sb.Len() < 4 {
		sb.WriteByte('0')
	}

	return sb.String()
}

// LevenshteinDistance computes edit distance between two strings
func LevenshteinDistance(s1, s2 string) int {
	r1, r2 := []rune(s1), []rune(s2)
	n, m := len(r1), len(r2)

	if n == 0 {
		return m
	}
	if m == 0 {
		return n
	}

	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
		dp[i][0] = i
	}
	for j := 0; j <= m; j++ {
		dp[0][j] = j
	}

	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			cost := 0
			if r1[i-1] != r2[j-1] {
				cost = 1
			}
			dp[i][j] = minInt(
				dp[i-1][j]+1,      // Deletion
				dp[i][j-1]+1,      // Insertion
				dp[i-1][j-1]+cost, // Substitution
			)
		}
	}
	return dp[n][m]
}

var devanagariMap = map[string]string{
	// Vowels
	"अ": "a", "आ": "aa", "इ": "i", "ई": "ee", "उ": "u", "ऊ": "oo",
	"ऋ": "ri", "ए": "e", "ऐ": "ai", "ओ": "o", "औ": "au", "अं": "an", "अः": "ah",
	// Vowel Signs
	"ा": "a", "ि": "i", "ी": "ee", "ु": "u", "ू": "oo",
	"ृ": "ri", "े": "e", "ै": "ai", "ो": "o", "ौ": "au", "ं": "n", "ँ": "n", "ः": "h",
	// Consonants
	"क": "k", "ख": "kh", "ग": "g", "घ": "gh", "ङ": "n",
	"च": "ch", "छ": "chh", "ज": "j", "झ": "jh", "ञ": "n",
	"ट": "t", "ठ": "th", "ड": "d", "ढ": "dh", "ण": "n",
	"त": "t", "थ": "th", "द": "d", "ध": "dh", "न": "n",
	"प": "p", "फ": "ph", "ब": "b", "भ": "bh", "म": "m",
	"य": "y", "र": "r", "ल": "l", "व": "v", "श": "sh", "ष": "sh", "स": "s", "ह": "h",
	"क्ष": "ksh", "त्र": "tr", "ज्ञ": "gya",
}

func resolveCompanyDir(basePath, companyName string) string {
	docsDir := filepath.Join(basePath, companyName)
	if _, err := os.Stat(docsDir); err == nil {
		return docsDir
	}
	targetDir := strings.ToLower(strings.TrimSpace(companyName))
	if entries, err := os.ReadDir(basePath); err == nil {
		for _, entry := range entries {
			if entry.IsDir() && strings.ToLower(entry.Name()) == targetDir {
				return filepath.Join(basePath, entry.Name())
			}
		}
	}
	return docsDir
}

func isAllUpper(s string) bool {
	for _, r := range s {
		if !unicode.IsUpper(r) {
			return false
		}
	}
	return len(s) > 0
}

func minInt(vals ...int) int {
	minV := vals[0]
	for _, v := range vals[1:] {
		if v < minV {
			minV = v
		}
	}
	return minV
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func appendUnique(slice []string, val string) []string {
	for _, item := range slice {
		if item == val {
			return slice
		}
	}
	return append(slice, val)
}
