package dto

type RegisterRequest struct {
	Email       string `json:"email" binding:"required,email"`
	DeviceID    string `json:"deviceId" binding:"required,min=3,max=64"`
	DisplayName string `json:"displayName" binding:"omitempty,max=100"`
	PhoneNumber string `json:"phoneNumber" binding:"omitempty"`
}

type RegisterAdminRequest struct {
	Email       string `json:"email" binding:"required,email"`
	DisplayName string `json:"displayName" binding:"omitempty,max=100"`
	PhoneNumber string `json:"phoneNumber" binding:"omitempty"`
}

type VerifyRegisterRequest struct {
	RequestID      string `json:"requestId" binding:"required,uuid"`
	OTP            string `json:"otp" binding:"required,len=6,numeric"`
	LoginChallenge string `json:"loginChallenge" binding:"omitempty"`
}

type LoginRequest struct {
	Email string `json:"email" binding:"required,email"`
}

type VerifyLoginRequest struct {
	RequestID      string `json:"requestId" binding:"required,uuid"`
	OTP            string `json:"otp" binding:"required,len=6,numeric"`
	LoginChallenge string `json:"loginChallenge" binding:"omitempty"`
}
