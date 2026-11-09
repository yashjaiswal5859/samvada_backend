package login

type LoginRequest struct {
	Email string `json:"email" binding:"required,email"`
}

type LoginResponse struct {
	RequestID string `json:"requestId"`
	ExpiresIn int64  `json:"expiresIn"`
}

type VerifyLoginRequest struct {
	RequestID string `json:"requestId" binding:"required,uuid"`
	OTP       string `json:"otp" binding:"required,len=6"`
}

type VerifyLoginResponse struct {
	AccessToken  string `json:"accessToken"`
	ExpiresIn    int64  `json:"expiresIn"`
	RefreshToken string `json:"-"`
}
