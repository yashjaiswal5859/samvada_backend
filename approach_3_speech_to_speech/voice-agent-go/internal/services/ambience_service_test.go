package services

import (
	"testing"
)

func TestAmbienceServiceCircularLoop(t *testing.T) {
	// Create mock 8kHz mono PCM buffer of 800 samples (100ms)
	mockPCM := make([]int16, 800)
	for i := 0; i < len(mockPCM); i++ {
		mockPCM[i] = int16(i * 10)
	}

	svc := &AmbienceService{
		pcmSamples: mockPCM,
		sampleLen:  int64(len(mockPCM)),
		readIdx:    0,
		enabled:    true,
	}

	// Read 5 frames of 160 samples (total 800 samples = exactly 1 loop)
	for f := 0; f < 5; f++ {
		frame := svc.GetNextFrame(160)
		if len(frame) != 160 {
			t.Fatalf("Expected 160 samples, got %d", len(frame))
		}
	}

	// 6th frame should wrap around to index 0 seamlessly without index out of bounds
	frame6 := svc.GetNextFrame(160)
	if len(frame6) != 160 {
		t.Fatalf("Expected 160 samples on loop wrap, got %d", len(frame6))
	}
}

func TestLoopBoundaryCrossfade(t *testing.T) {
	pcm := make([]int16, 1000)
	for i := 0; i < len(pcm); i++ {
		pcm[i] = 10000
	}
	// Tail samples set to distinct value
	for i := len(pcm) - 40; i < len(pcm); i++ {
		pcm[i] = -10000
	}

	applyLoopBoundaryCrossfade(pcm, 40)

	// Head sample 0 should now be blended smoothly between tail (-10000) and head (10000)
	if pcm[0] == 10000 {
		t.Fatalf("Expected head sample to be crossfaded, got %d", pcm[0])
	}
}

func TestAudioMixerSidechainDucking(t *testing.T) {
	ambiencePCM := make([]int16, 160)
	for i := 0; i < 160; i++ {
		ambiencePCM[i] = 10000
	}

	ttsMuLaw := make([]byte, 160)
	for i := 0; i < 160; i++ {
		ttsMuLaw[i] = linearToMulaw(5000)
	}

	// Test Idle Mix (Gain = 0.05 / 5%)
	idleMixed := mixAudioFrame(ambiencePCM, nil, 0.05)
	if len(idleMixed) != 160 {
		t.Fatalf("Expected 160 mixed bytes, got %d", len(idleMixed))
	}

	// Test Speaking Mix (Gain = 0.02 / 2% + TTS Speech)
	speakingMixed := mixAudioFrame(ambiencePCM, ttsMuLaw, 0.02)
	if len(speakingMixed) != 160 {
		t.Fatalf("Expected 160 mixed bytes, got %d", len(speakingMixed))
	}
}
