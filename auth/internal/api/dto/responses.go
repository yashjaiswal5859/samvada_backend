package dto

type RegisterResponse struct {
	RequestID string `json:"requestId"`
	ExpiresIn int64  `json:"expiresIn"`
}

type TokenResponse struct {
	AccessToken  string `json:"accessToken"`
	ExpiresIn    int64  `json:"expiresIn"`
	RefreshToken string `json:"refreshToken,omitempty"`
}
