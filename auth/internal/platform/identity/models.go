package identity

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	IDToken      string `json:"id_token"`
}

type IDTokenClaims struct {
	Sub   string `json:"sub"`
	Aud   string `json:"aud"`
	Iss   string `json:"iss"`
	Nonce string `json:"nonce"`
	Exp   int64  `json:"exp"`
}

type RelationshipRequest struct {
	Namespace string `json:"namespace"`
	Object    string `json:"object"`
	Relation  string `json:"relation"`
	Subject   string `json:"subject"`
}

type PermissionRequest struct {
	Namespace string `json:"namespace"`
	Object    string `json:"object"`
	Relation  string `json:"relation"`
	Subject   string `json:"subject"`
}
