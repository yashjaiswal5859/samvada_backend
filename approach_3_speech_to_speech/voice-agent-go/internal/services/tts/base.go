package tts

import (
	"context"
)

// TTSProvider defines the contract for all Text-To-Speech providers (SOLID: Interface Segregation & Dependency Inversion)
type TTSProvider interface {
	ProviderName() string
	SynthesizeSpeechMulaw(ctx context.Context, text, voice string) ([]byte, error)
}

// ConvertPCMToMulaw converts raw PCM bytes at sourceSampleRate to 8000Hz G.711 u-law bytes for Plivo telephony
func ConvertPCMToMulaw(pcmBytes []byte, sourceSampleRate, targetSampleRate int) []byte {
	if len(pcmBytes) < 2 {
		return nil
	}
	numSamples := len(pcmBytes) / 2
	step := float64(sourceSampleRate) / float64(targetSampleRate)
	if step < 1.0 {
		step = 1.0
	}

	targetCount := int(float64(numSamples) / step)
	mulawBytes := make([]byte, targetCount)

	for i := 0; i < targetCount; i++ {
		srcIdx := int(float64(i) * step)
		byteIdx := srcIdx * 2
		if byteIdx+1 < len(pcmBytes) {
			sample := int16(uint16(pcmBytes[byteIdx]) | uint16(pcmBytes[byteIdx+1])<<8)
			mulawBytes[i] = LinearToMulaw(sample)
		}
	}
	return mulawBytes
}

func LinearToMulaw(pcm int16) byte {
	const bias = 132
	const clip = 32635

	sign := byte((pcm >> 8) & 0x80)
	if pcm < 0 {
		pcm = -pcm
	}
	if pcm > clip {
		pcm = clip
	}
	pcm = pcm + bias

	exponent := byte(7)
	for mask := int16(0x4000); (pcm&mask) == 0 && exponent > 0; mask >>= 1 {
		exponent--
	}
	mantissa := byte((pcm >> (exponent + 3)) & 0x0F)
	mulawByte := sign | (exponent << 4) | mantissa
	return ^mulawByte
}
