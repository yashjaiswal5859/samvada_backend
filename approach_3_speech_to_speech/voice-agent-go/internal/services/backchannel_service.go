package services

import (
	"context"
	"log"
	"math/rand"
	"os"
	"sync"
	"time"
)

type BackchannelService struct {
	audioClips [][]byte
	mu         sync.RWMutex
}

func NewBackchannelService(tts *TTSService) *BackchannelService {
	b := &BackchannelService{
		audioClips: make([][]byte, 0),
	}
	b.initClips(tts)
	return b
}

func (b *BackchannelService) initClips(tts *TTSService) {
	b.mu.Lock()
	defer b.mu.Unlock()

	dir := "audio/backchannels"
	_ = os.RemoveAll(dir) // Clean old cached files

	// English active listening cues
	cues := []string{"hmm", "okay", "right", "yeah"}

	// Synthesize default backchannel cues via active TTS voice at 100% full volume
	if tts != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		for _, cue := range cues {
			audio, err := tts.SynthesizeSpeechMulaw(ctx, cue)
			if err == nil && len(audio) > 0 {
				b.audioClips = append(b.audioClips, audio)
				log.Printf("[Backchannel] Synthesized active listening cue at 100%% volume with active voice: %q (%d bytes)", cue, len(audio))
			}
		}
	}
}

func (b *BackchannelService) GetRandomClip() []byte {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if len(b.audioClips) == 0 {
		return nil
	}
	idx := rand.Intn(len(b.audioClips))
	return b.audioClips[idx]
}

// ReduceMulawVolume reduces G.711 mu-law audio volume to scale (e.g. 0.20 for 20% volume soft low voice)
func ReduceMulawVolume(mulawData []byte, scale float32) []byte {
	if len(mulawData) == 0 {
		return mulawData
	}
	scaled := make([]byte, len(mulawData))
	for i, uVal := range mulawData {
		sample := MulawSampleToPcm16(uVal)
		newPcmFloat := float32(sample) * scale
		if newPcmFloat > 32767 {
			newPcmFloat = 32767
		} else if newPcmFloat < -32768 {
			newPcmFloat = -32768
		}
		scaled[i] = linearToMulaw(int16(newPcmFloat))
	}
	return scaled
}
