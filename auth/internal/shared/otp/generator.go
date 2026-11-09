package otp

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"math/big"
)

func GenerateOTP() (string, error) {
	max := big.NewInt(900000)
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", err
	}
	otpVal := n.Add(n, big.NewInt(100000)).String()
	return otpVal, nil
}

func HashOTP(otp string) string {
	hash := sha256.Sum256([]byte(otp))
	return hex.EncodeToString(hash[:])
}

// VerifyOTP uses constant-time comparison to prevent timing attacks.
// An attacker cannot measure response time differences to brute-force OTP digits.
func VerifyOTP(otp, hashedOTP string) bool {
	return subtle.ConstantTimeCompare([]byte(HashOTP(otp)), []byte(hashedOTP)) == 1
}
