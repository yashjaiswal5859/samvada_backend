package services

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"voice-agent-go/internal/config"
)

type FillerService struct {
	enFillers     [][]byte
	hiFillers     [][]byte
	enHistory     []int
	hiHistory     []int

	ackClips      [][]byte
	ackHistory    []int

	mu            sync.RWMutex
}

func NewFillerService(tts *TTSService) *FillerService {
	f := &FillerService{
		enFillers:  make([][]byte, 0),
		hiFillers:  make([][]byte, 0),
		enHistory:  make([]int, 0),
		hiHistory:  make([]int, 0),
		ackClips:   make([][]byte, 0),
		ackHistory: make([]int, 0),
	}
	if tts != nil {
		go f.initClips(tts)
	}
	log.Printf("[Filler Service] Initialized filler & acknowledgment clips loader.")
	return f
}

func (f *FillerService) initClips(tts *TTSService) {
	if tts == nil {
		log.Printf("[Filler Service] Warning: TTSService is nil. Skipping filler pre-synthesis.")
		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	// 1. Resolve Provider & Voice ID to build structured cache directory: audio/fillers/{provider}/{voice_id}/
	provider := strings.ToLower(config.Settings.TtsProvider)
	if provider == "" {
		provider = "fishaudio"
	}
	voiceID := config.Settings.FishAudioVoice
	if provider == "cartesia" {
		voiceID = config.Settings.CartesiaVoice
	} else if provider == "openai" {
		voiceID = config.Settings.OpenaiVoice
	} else if provider == "murf" || provider == "murf_cloud" || provider == "murf_falcon2" || provider == "falcon2" || provider == "falcon-2" {
		voiceID = config.Settings.MurfVoice
	}
	if voiceID == "" {
		voiceID = "default"
	}

	cacheDir := filepath.Join("audio", "fillers", provider, voiceID)
	// Preserve filler cache on disk permanently across server restarts!
	_ = os.MkdirAll(cacheDir, 0755)

	f.enFillers = make([][]byte, 0)
	f.hiFillers = make([][]byte, 0)
	f.ackClips = make([][]byte, 0)

	// Universal Filler Phrases at 0.90 speed matching active voice stream
	enPhrases := []string{
		"Yeah, got it!",
		"Ah, I see!",
		"Right, got it!",
		"Makes total sense!",
		"Alright, got it!",
		"Got it, perfect!",
		"Sure, got it!",
		"Hmm, got it!",
	}

	hiPhrases := []string{
		"हाँ, जी!",
		"जी, बिल्कुल!",
		"जी, समझ गई!",
		"सही बात है!",
		"जी, ठीक है!",
		"बिल्कुल, सही है!",
	}

	ackPhrases := []string{
		"Okay, got it!",
		"Yeah, got it!",
		"Sure, got it!",
	}

	var wg sync.WaitGroup
	maxConcurrency := 2
	if provider == "murf" || provider == "murf_cloud" || provider == "murf_falcon2" || provider == "falcon2" || provider == "falcon-2" {
		maxConcurrency = 1 // 1 request at a time sequentially for Murf to respect account concurrency limits
	}
	sem := make(chan struct{}, maxConcurrency)

	// Helper function: Load from disk cache if present, otherwise synthesize at 0.90 speed & save to disk
	getAudioClip := func(filename string, phrase string) []byte {
		filePath := filepath.Join(cacheDir, filename)
		if data, err := os.ReadFile(filePath); err == nil && len(data) > 0 {
			log.Printf("[Filler Cache HIT] Loaded from disk: %s (%d bytes)", filePath, len(data))
			return data
		}

		sem <- struct{}{}
		defer func() { <-sem }()

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		fillerSpeed := config.Settings.FillerAudioSpeed
		if fillerSpeed <= 0 {
			fillerSpeed = config.Settings.TtsSpeed
		}
		audio, err := tts.SynthesizeSpeechMulawWithSpeed(ctx, phrase, fillerSpeed)
		if err == nil && len(audio) > 0 {
			_ = os.WriteFile(filePath, audio, 0644)
			log.Printf("[Filler Cache SAVED] Synthesized & stored to disk at %.2f speed: %s (%d bytes)", fillerSpeed, filePath, len(audio))
			return audio
		}
		log.Printf("[Filler Service Warning] Failed to synthesize %q: %v", phrase, err)
		return nil
	}

	// Synthesize / Load English Tier 1 Fillers
	enClips := make([][]byte, len(enPhrases))
	for i, text := range enPhrases {
		wg.Add(1)
		time.Sleep(50 * time.Millisecond)
		go func(idx int, phrase string) {
			defer wg.Done()
			fn := fmt.Sprintf("en_filler_%02d.mulaw", idx+1)
			audio := getAudioClip(fn, phrase)
			if len(audio) > 0 {
				enClips[idx] = audio
			}
		}(i, text)
	}

	// Synthesize / Load Hinglish Tier 1 Fillers
	hiClips := make([][]byte, len(hiPhrases))
	for i, text := range hiPhrases {
		wg.Add(1)
		time.Sleep(50 * time.Millisecond)
		go func(idx int, phrase string) {
			defer wg.Done()
			fn := fmt.Sprintf("hi_filler_%02d.mulaw", idx+1)
			audio := getAudioClip(fn, phrase)
			if len(audio) > 0 {
				hiClips[idx] = audio
			}
		}(i, text)
	}

	// Synthesize / Load Tier 2 Micro-Acknowledgments
	ackClips := make([][]byte, len(ackPhrases))
	for i, text := range ackPhrases {
		wg.Add(1)
		time.Sleep(50 * time.Millisecond)
		go func(idx int, phrase string) {
			defer wg.Done()
			fn := fmt.Sprintf("ack_%02d.mulaw", idx+1)
			audio := getAudioClip(fn, phrase)
			if len(audio) > 0 {
				ackClips[idx] = audio
			}
		}(i, text)
	}

	wg.Wait()

	for _, clip := range enClips {
		if len(clip) > 0 {
			f.enFillers = append(f.enFillers, clip)
		}
	}
	for _, clip := range hiClips {
		if len(clip) > 0 {
			f.hiFillers = append(f.hiFillers, clip)
		}
	}
	for _, clip := range ackClips {
		if len(clip) > 0 {
			f.ackClips = append(f.ackClips, clip)
		}
	}

	log.Printf("[Filler Service READY] Pre-synthesized %d English fillers, %d Hinglish fillers, and %d Micro-Acks in active voice!",
		len(f.enFillers), len(f.hiFillers), len(f.ackClips))
}

// GetInitialHmmFiller retrieves an 'Okay, got it' micro-ack filler for initial turn latency absorption
func (f *FillerService) GetInitialHmmFiller(lang string) []byte {
	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.ackClips) > 0 {
		return f.ackClips[0]
	}

	var pool [][]byte
	if lang == "hi" || lang == "hinglish" {
		pool = f.hiFillers
	} else {
		pool = f.enFillers
	}

	if len(pool) == 0 {
		return nil
	}
	return pool[0]
}

// GetRandomFiller retrieves a Tier-1 thinking filler, avoiding the last 3 used indices
func (f *FillerService) GetRandomFiller(lang string) []byte {
	f.mu.Lock()
	defer f.mu.Unlock()

	var pool [][]byte
	var history *[]int

	if lang == "hi" || lang == "hinglish" {
		pool = f.hiFillers
		history = &f.hiHistory
	} else {
		pool = f.enFillers
		history = &f.enHistory
	}

	if len(pool) == 0 {
		return nil
	}

	// Find available index not in recent history
	available := make([]int, 0, len(pool))
	for i := 0; i < len(pool); i++ {
		inHist := false
		for _, hIdx := range *history {
			if hIdx == i {
				inHist = true
				break
			}
		}
		if !inHist {
			available = append(available, i)
		}
	}

	if len(available) == 0 {
		// Reset history if all used
		*history = make([]int, 0)
		for i := 0; i < len(pool); i++ {
			available = append(available, i)
		}
	}

	chosenIdx := available[rand.Intn(len(available))]
	*history = append(*history, chosenIdx)
	if len(*history) > 3 {
		*history = (*history)[1:]
	}

	return pool[chosenIdx]
}

// GetRandomAck retrieves a Tier-2 micro-acknowledgment ("Hmm", "Okay", "Got it"), avoiding the last 2 used indices
func (f *FillerService) GetRandomAck() []byte {
	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.ackClips) == 0 {
		return nil
	}

	available := make([]int, 0, len(f.ackClips))
	for i := 0; i < len(f.ackClips); i++ {
		inHist := false
		for _, hIdx := range f.ackHistory {
			if hIdx == i {
				inHist = true
				break
			}
		}
		if !inHist {
			available = append(available, i)
		}
	}

	if len(available) == 0 {
		f.ackHistory = make([]int, 0)
		for i := 0; i < len(f.ackClips); i++ {
			available = append(available, i)
		}
	}

	chosenIdx := available[rand.Intn(len(available))]
	f.ackHistory = append(f.ackHistory, chosenIdx)
	if len(f.ackHistory) > 2 {
		f.ackHistory = f.ackHistory[1:]
	}

	return f.ackClips[chosenIdx]
}
