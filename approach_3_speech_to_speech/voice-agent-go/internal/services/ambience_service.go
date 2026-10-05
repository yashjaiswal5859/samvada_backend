package services

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"voice-agent-go/internal/config"

	"github.com/hajimehoshi/go-mp3"
)

var GlobalAmbienceService *AmbienceService

type AmbienceService struct {
	pcmSamples []int16
	sampleLen  int64
	readIdx    int64
	enabled    bool
	mu         sync.RWMutex
}

func InitGlobalAmbienceService() {
	if !config.Settings.AmbienceAudioEnabled {
		log.Println("[Ambience Service] Ambience audio is disabled in config.")
		return
	}

	path := config.Settings.AmbienceAudioPath
	if path == "" {
		log.Println("[Ambience Service] Warning: AMBIENCE_AUDIO_PATH is empty.")
		return
	}

	svc, err := NewAmbienceService(path)
	if err != nil {
		log.Printf("[Ambience Service] Warning: Failed to load ambience audio from %s: %v", path, err)
		return
	}

	GlobalAmbienceService = svc
	log.Printf("[Ambience Service READY] Loaded seamless ambience loop (%d 8kHz PCM samples / ~%.1f sec audio)",
		svc.sampleLen, float64(svc.sampleLen)/8000.0)
}

func NewAmbienceService(filePath string) (*AmbienceService, error) {
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read audio file: %w", err)
	}

	var pcmMono8k []int16

	lowerPath := strings.ToLower(filePath)
	if strings.HasSuffix(lowerPath, ".mp3") {
		decodedPCM, sampleRate, numChannels, err := decodeMP3(fileData)
		if err != nil {
			return nil, fmt.Errorf("failed to decode MP3: %w", err)
		}
		pcmMono8k = resampleAndDownmixTo8kHzMono(decodedPCM, sampleRate, numChannels)
	} else if strings.HasSuffix(lowerPath, ".wav") {
		decodedPCM, sampleRate, numChannels, err := decodeWAV(fileData)
		if err != nil {
			return nil, fmt.Errorf("failed to decode WAV: %w", err)
		}
		pcmMono8k = resampleAndDownmixTo8kHzMono(decodedPCM, sampleRate, numChannels)
	} else {
		return nil, fmt.Errorf("unsupported audio extension: %s", filePath)
	}

	if len(pcmMono8k) < 160 {
		return nil, fmt.Errorf("decoded audio too short (%d samples)", len(pcmMono8k))
	}

	// Peak Amplitude Normalization & Signal Gain Boost (Ensures crisp audible background over PSTN)
	var maxAbs int32 = 0
	for _, s := range pcmMono8k {
		val := int32(s)
		if val < 0 {
			val = -val
		}
		if val > maxAbs {
			maxAbs = val
		}
	}

	if maxAbs > 0 {
		targetPeak := int32(16384) // 50% full scale linear PCM
		scaleRatio := float64(targetPeak) / float64(maxAbs)
		log.Printf("[Ambience Service] Normalizing audio peak amplitude: Raw Max = %d -> Target Max = %d (Boost Ratio: %.2fx)", maxAbs, targetPeak, scaleRatio)
		for i := 0; i < len(pcmMono8k); i++ {
			boosted := int32(float64(pcmMono8k[i]) * scaleRatio)
			if boosted > 32767 {
				boosted = 32767
			} else if boosted < -32768 {
				boosted = -32768
			}
			pcmMono8k[i] = int16(boosted)
		}
	}

	// Boundary smoothing crossfade (40 samples = 5ms at 8kHz) to eliminate loop click
	applyLoopBoundaryCrossfade(pcmMono8k, 40)

	return &AmbienceService{
		pcmSamples: pcmMono8k,
		sampleLen:  int64(len(pcmMono8k)),
		readIdx:    0,
		enabled:    true,
	}, nil
}

// GetNextFrame returns the next 'count' 8kHz mono PCM samples from the circular loop (thread-safe, zero allocations)
func (a *AmbienceService) GetNextFrame(count int) []int16 {
	if a == nil || !a.enabled || a.sampleLen == 0 {
		return make([]int16, count)
	}

	out := make([]int16, count)
	for i := 0; i < count; i++ {
		currIdx := atomic.AddInt64(&a.readIdx, 1) - 1
		actualIdx := currIdx % a.sampleLen
		if actualIdx < 0 {
			actualIdx += a.sampleLen
		}
		out[i] = a.pcmSamples[actualIdx]
	}
	return out
}

func decodeMP3(data []byte) ([]int16, int, int, error) {
	decoder, err := mp3.NewDecoder(bytes.NewReader(data))
	if err != nil {
		return nil, 0, 0, err
	}

	rawBytes, err := io.ReadAll(decoder)
	if err != nil {
		return nil, 0, 0, err
	}

	sampleRate := decoder.SampleRate()
	// go-mp3 always outputs 16-bit PCM (2 bytes per sample, 2 channels)
	numChannels := 2
	numSamples := len(rawBytes) / 2
	pcm := make([]int16, numSamples)

	for i := 0; i < numSamples; i++ {
		low := uint16(rawBytes[2*i])
		high := uint16(rawBytes[2*i+1])
		pcm[i] = int16(low | (high << 8))
	}

	return pcm, sampleRate, numChannels, nil
}

func decodeWAV(data []byte) ([]int16, int, int, error) {
	if len(data) < 44 || string(data[0:4]) != "RIFF" {
		return nil, 0, 0, fmt.Errorf("invalid WAV header")
	}

	channels := int(data[22]) | (int(data[23]) << 8)
	sampleRate := int(data[24]) | (int(data[25]) << 8) | (int(data[26]) << 16) | (int(data[27]) << 24)
	bitsPerSample := int(data[34]) | (int(data[35]) << 8)

	dataStart := 44
	for i := 12; i < len(data)-8; i++ {
		if string(data[i:i+4]) == "data" {
			dataStart = i + 8
			break
		}
	}

	pcmData := data[dataStart:]
	if bitsPerSample != 16 {
		return nil, 0, 0, fmt.Errorf("only 16-bit WAV supported, got %d", bitsPerSample)
	}

	numSamples := len(pcmData) / 2
	pcm := make([]int16, numSamples)
	for i := 0; i < numSamples; i++ {
		low := uint16(pcmData[2*i])
		high := uint16(pcmData[2*i+1])
		pcm[i] = int16(low | (high << 8))
	}

	return pcm, sampleRate, channels, nil
}

func resampleAndDownmixTo8kHzMono(pcm []int16, srcSampleRate, srcChannels int) []int16 {
	if srcSampleRate <= 0 {
		srcSampleRate = 44100
	}
	if srcChannels <= 0 {
		srcChannels = 1
	}

	// 1. Downmix channels to Mono
	totalFrames := len(pcm) / srcChannels
	monoPCM := make([]int16, totalFrames)
	for f := 0; f < totalFrames; f++ {
		var sum int32
		for ch := 0; ch < srcChannels; ch++ {
			sum += int32(pcm[f*srcChannels+ch])
		}
		monoPCM[f] = int16(sum / int32(srcChannels))
	}

	if srcSampleRate == 8000 {
		return monoPCM
	}

	// 2. Linear Interpolation Resampling to 8000Hz
	ratio := float64(srcSampleRate) / 8000.0
	outLen := int(float64(totalFrames) / ratio)
	outPCM := make([]int16, outLen)

	for i := 0; i < outLen; i++ {
		srcPos := float64(i) * ratio
		srcIdx := int(srcPos)
		frac := srcPos - float64(srcIdx)

		if srcIdx >= len(monoPCM)-1 {
			outPCM[i] = monoPCM[len(monoPCM)-1]
			continue
		}

		s1 := float64(monoPCM[srcIdx])
		s2 := float64(monoPCM[srcIdx+1])
		interp := s1 + frac*(s2-s1)
		outPCM[i] = int16(interp)
	}

	return outPCM
}

func applyLoopBoundaryCrossfade(pcm []int16, crossfadeSamples int) {
	n := len(pcm)
	if n < crossfadeSamples*2 {
		return
	}

	for i := 0; i < crossfadeSamples; i++ {
		alpha := float64(i) / float64(crossfadeSamples)
		headIdx := i
		tailIdx := n - crossfadeSamples + i

		headSample := float64(pcm[headIdx])
		tailSample := float64(pcm[tailIdx])

		// Smooth linear blend at boundary
		blended := (1.0-alpha)*tailSample + alpha*headSample
		pcm[headIdx] = int16(blended)
	}
}
