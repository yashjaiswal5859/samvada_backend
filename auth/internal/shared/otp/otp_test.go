package otp

import (
	"strconv"
	"testing"
)

func TestGenerateOTP(t *testing.T) {
	for i := 0; i < 50; i++ {
		otpVal, err := GenerateOTP()
		if err != nil {
			t.Fatalf("GenerateOTP failed: %v", err)
		}
		if len(otpVal) != 6 {
			t.Fatalf("Expected OTP length 6, got %d (val: %s)", len(otpVal), otpVal)
		}
		n, err := strconv.Atoi(otpVal)
		if err != nil {
			t.Fatalf("Expected numeric OTP, got error: %v", err)
		}
		if n < 100000 || n > 999999 {
			t.Fatalf("OTP out of 6-digit range: %d", n)
		}
	}
}

func TestHashAndVerifyOTP(t *testing.T) {
	otpVal := "543210"
	hash := HashOTP(otpVal)

	if len(hash) != 64 {
		t.Fatalf("Expected 64-char sha256 hex, got %d chars: %s", len(hash), hash)
	}

	// Correct OTP
	if !VerifyOTP(otpVal, hash) {
		t.Fatalf("Expected VerifyOTP to succeed for matching OTP")
	}

	// Incorrect OTP
	if VerifyOTP("123456", hash) {
		t.Fatalf("Expected VerifyOTP to fail for non-matching OTP")
	}

	// Tampered OTP
	if VerifyOTP("543211", hash) {
		t.Fatalf("Expected VerifyOTP to fail for off-by-one OTP")
	}
}
