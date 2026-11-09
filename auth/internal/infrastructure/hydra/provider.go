package hydra

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"github.com/ridesafety/auth-service/internal/platform/identity"
)

type hydraProvider struct {
	adminURL     string
	publicURL    string
	browserURL   string
	clientID     string
	clientSecret string
	redirectURI  string
	httpClient   *http.Client
	noRedirect   *http.Client
}

func NewHydraProvider(adminURL, publicURL, browserURL, clientID, clientSecret, redirectURI string) identity.TokenProvider {
	return &hydraProvider{
		adminURL:     adminURL,
		publicURL:    publicURL,
		browserURL:   browserURL,
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURI:  redirectURI,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		noRedirect: &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

type acceptLoginReq struct {
	Subject     string `json:"subject"`
	Remember    bool   `json:"remember"`
	RememberFor int    `json:"remember_for"`
}

type acceptConsentReq struct {
	GrantScope  []string            `json:"grant_scope"`
	Remember    bool                `json:"remember"`
	RememberFor int                 `json:"remember_for"`
	Session     acceptConsentSession `json:"session"`
}

type acceptConsentSession struct {
	AccessToken map[string]interface{} `json:"access_token"`
	IDToken     map[string]interface{} `json:"id_token"`
}

type hydraRedirectResp struct {
	RedirectTo string `json:"redirect_to"`
}

type tokenResp struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	IDToken      string `json:"id_token"`
}

func (p *hydraProvider) fixRedirectURL(redirect string) string {
	parsed, err := url.Parse(redirect)
	if err != nil {
		return redirect
	}
	publicParsed, err := url.Parse(p.publicURL)
	if err == nil {
		parsed.Scheme = publicParsed.Scheme
		parsed.Host = publicParsed.Host
	}
	return parsed.String()
}

func (p *hydraProvider) GenerateTokens(ctx context.Context, kratosID string, customClaims map[string]interface{}) (*identity.TokenPair, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	flowClient := &http.Client{
		Jar:     jar,
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// Step 1: Initiate OAuth2 Authorization Code Flow
	authURL := fmt.Sprintf("%s/oauth2/auth?client_id=%s&response_type=code&redirect_uri=%s&scope=openid+offline_access&state=state123",
		p.publicURL, p.clientID, url.QueryEscape(p.redirectURI))

	req, err := http.NewRequestWithContext(ctx, "GET", authURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := flowClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", identity.ErrOryUnreachable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound && resp.StatusCode != http.StatusSeeOther {
		return nil, fmt.Errorf("unexpected status code from hydra auth init: %d", resp.StatusCode)
	}

	locationURL := resp.Header.Get("Location")
	parsedLocation, err := url.Parse(locationURL)
	if err != nil {
		return nil, err
	}
	loginChallenge := parsedLocation.Query().Get("login_challenge")
	if loginChallenge == "" {
		return nil, fmt.Errorf("login_challenge not found in redirect location")
	}

	// Step 2: Accept Login Challenge
	acceptLoginURL := fmt.Sprintf("%s/oauth2/auth/requests/login/accept?login_challenge=%s", p.adminURL, loginChallenge)
	loginBody := acceptLoginReq{
		Subject:     kratosID,
		Remember:    true,
		RememberFor: 3600,
	}
	loginJSON, err := json.Marshal(loginBody)
	if err != nil {
		return nil, err
	}

	req, err = http.NewRequestWithContext(ctx, "PUT", acceptLoginURL, bytes.NewBuffer(loginJSON))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err = p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to accept login challenge: %d", resp.StatusCode)
	}

	var loginRedirect hydraRedirectResp
	if err := json.NewDecoder(resp.Body).Decode(&loginRedirect); err != nil {
		return nil, err
	}

	// Step 3: Follow redirect to get consent challenge
	redirectConsent := p.fixRedirectURL(loginRedirect.RedirectTo)
	req, err = http.NewRequestWithContext(ctx, "GET", redirectConsent, nil)
	if err != nil {
		return nil, err
	}

	resp, err = flowClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound && resp.StatusCode != http.StatusSeeOther {
		return nil, fmt.Errorf("unexpected status code after accepting login: %d", resp.StatusCode)
	}

	locationURL = resp.Header.Get("Location")
	parsedLocation, err = url.Parse(locationURL)
	if err != nil {
		return nil, err
	}
	consentChallenge := parsedLocation.Query().Get("consent_challenge")
	if consentChallenge == "" {
		return nil, fmt.Errorf("consent_challenge not found in redirect location")
	}

	// Step 4: Accept Consent Challenge
	acceptConsentURL := fmt.Sprintf("%s/oauth2/auth/requests/consent/accept?consent_challenge=%s", p.adminURL, consentChallenge)
	idTokenClaims := map[string]interface{}{
		"sub": kratosID,
	}
	for k, v := range customClaims {
		idTokenClaims[k] = v
	}
	consentBody := acceptConsentReq{
		GrantScope:  []string{"openid", "offline_access"},
		Remember:    true,
		RememberFor: 3600,
		Session: acceptConsentSession{
			AccessToken: customClaims,
			IDToken:     idTokenClaims,
		},
	}
	consentJSON, err := json.Marshal(consentBody)
	if err != nil {
		return nil, err
	}

	req, err = http.NewRequestWithContext(ctx, "PUT", acceptConsentURL, bytes.NewBuffer(consentJSON))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err = p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to accept consent challenge: %d", resp.StatusCode)
	}

	var consentRedirect hydraRedirectResp
	if err := json.NewDecoder(resp.Body).Decode(&consentRedirect); err != nil {
		return nil, err
	}

	// Step 5: Follow redirect to get authorization code
	redirectAuth := p.fixRedirectURL(consentRedirect.RedirectTo)
	req, err = http.NewRequestWithContext(ctx, "GET", redirectAuth, nil)
	if err != nil {
		return nil, err
	}

	resp, err = flowClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound && resp.StatusCode != http.StatusSeeOther {
		return nil, fmt.Errorf("unexpected status code after accepting consent: %d", resp.StatusCode)
	}

	locationURL = resp.Header.Get("Location")
	parsedLocation, err = url.Parse(locationURL)
	if err != nil {
		return nil, err
	}
	code := parsedLocation.Query().Get("code")
	if code == "" {
		return nil, fmt.Errorf("authorization code not found in final redirect location")
	}

	// Step 6: Exchange Code for Access and Refresh Tokens
	tokenURL := fmt.Sprintf("%s/oauth2/token", p.publicURL)
	formValues := url.Values{}
	formValues.Set("grant_type", "authorization_code")
	formValues.Set("code", code)
	formValues.Set("redirect_uri", p.redirectURI)
	// client credentials in form body might be ignored if client_secret_basic is configured,
	// so we will also set Basic Auth below.

	req, err = http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(formValues.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(p.clientID, p.clientSecret)

	resp, err = p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to exchange code for tokens: status %d, body %s", resp.StatusCode, string(bodyBytes))
	}

	var tok tokenResp
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return nil, err
	}

	return &identity.TokenPair{
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		TokenType:    tok.TokenType,
		ExpiresIn:    tok.ExpiresIn,
		IDToken:      tok.IDToken,
	}, nil
}

func (p *hydraProvider) RotateTokens(ctx context.Context, refreshToken string) (*identity.TokenPair, error) {
	tokenURL := fmt.Sprintf("%s/oauth2/token", p.publicURL)
	formValues := url.Values{}
	formValues.Set("grant_type", "refresh_token")
	formValues.Set("refresh_token", refreshToken)

	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(formValues.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(p.clientID, p.clientSecret)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", identity.ErrOryUnreachable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnauthorized {
		return nil, identity.ErrInvalidRefreshToken
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status from hydra during refresh: %d", resp.StatusCode)
	}

	var tok tokenResp
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return nil, err
	}

	return &identity.TokenPair{
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		TokenType:    tok.TokenType,
		ExpiresIn:    tok.ExpiresIn,
	}, nil
}

func (p *hydraProvider) RevokeTokens(ctx context.Context, refreshToken string) error {
	revokeURL := fmt.Sprintf("%s/oauth2/revoke", p.publicURL)
	formValues := url.Values{}
	formValues.Set("token", refreshToken)

	req, err := http.NewRequestWithContext(ctx, "POST", revokeURL, strings.NewReader(formValues.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(p.clientID, p.clientSecret)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", identity.ErrOryUnreachable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code during token revocation: %d", resp.StatusCode)
	}

	return nil
}

func (p *hydraProvider) AcceptLoginChallenge(ctx context.Context, challenge string, subject string) (string, error) {
	acceptLoginURL := fmt.Sprintf("%s/oauth2/auth/requests/login/accept?login_challenge=%s", p.adminURL, challenge)
	loginBody := acceptLoginReq{
		Subject:     subject,
		Remember:    true,
		RememberFor: 3600,
	}
	loginJSON, err := json.Marshal(loginBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", acceptLoginURL, bytes.NewBuffer(loginJSON))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %v", identity.ErrOryUnreachable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to accept login challenge (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var loginRedirect hydraRedirectResp
	if err := json.NewDecoder(resp.Body).Decode(&loginRedirect); err != nil {
		return "", err
	}

	return loginRedirect.RedirectTo, nil
}

func (p *hydraProvider) AcceptConsentChallenge(ctx context.Context, challenge string, subject string, scopes []string, customClaims map[string]interface{}) (string, error) {
	acceptConsentURL := fmt.Sprintf("%s/oauth2/auth/requests/consent/accept?consent_challenge=%s", p.adminURL, challenge)
	idTokenClaims := map[string]interface{}{
		"sub": subject,
	}
	for k, v := range customClaims {
		idTokenClaims[k] = v
	}
	consentBody := acceptConsentReq{
		GrantScope:  scopes,
		Remember:    true,
		RememberFor: 3600,
		Session: acceptConsentSession{
			AccessToken: customClaims,
			IDToken:     idTokenClaims,
		},
	}
	consentJSON, err := json.Marshal(consentBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", acceptConsentURL, bytes.NewBuffer(consentJSON))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %v", identity.ErrOryUnreachable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to accept consent challenge (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var consentRedirect hydraRedirectResp
	if err := json.NewDecoder(resp.Body).Decode(&consentRedirect); err != nil {
		return "", err
	}

	return consentRedirect.RedirectTo, nil
}

func (p *hydraProvider) ExchangeCodeForTokens(ctx context.Context, code string, codeVerifier string) (*identity.TokenPair, error) {
	tokenURL := fmt.Sprintf("%s/oauth2/token", p.publicURL)
	formValues := url.Values{}
	formValues.Set("grant_type", "authorization_code")
	formValues.Set("code", code)
	formValues.Set("redirect_uri", p.redirectURI)
	if codeVerifier != "" {
		formValues.Set("code_verifier", codeVerifier)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(formValues.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(p.clientID, p.clientSecret)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", identity.ErrOryUnreachable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to exchange code for tokens (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var tok tokenResp
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return nil, err
	}

	return &identity.TokenPair{
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		TokenType:    tok.TokenType,
		ExpiresIn:    tok.ExpiresIn,
		IDToken:      tok.IDToken,
	}, nil
}

type introspectResp struct {
	Active bool   `json:"active"`
	Sub    string `json:"sub"`
	Exp    int64  `json:"exp"`
}

func (p *hydraProvider) IntrospectToken(ctx context.Context, token string) (bool, string, int64, error) {
	introspectURL := fmt.Sprintf("%s/oauth2/introspect", p.adminURL)
	formValues := url.Values{}
	formValues.Set("token", token)

	req, err := http.NewRequestWithContext(ctx, "POST", introspectURL, strings.NewReader(formValues.Encode()))
	if err != nil {
		return false, "", 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return false, "", 0, fmt.Errorf("%w: %v", identity.ErrOryUnreachable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return false, "", 0, fmt.Errorf("failed to introspect token (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var intro introspectResp
	if err := json.NewDecoder(resp.Body).Decode(&intro); err != nil {
		return false, "", 0, err
	}

	return intro.Active, intro.Sub, intro.Exp, nil
}

type consentRequestResp struct {
	Subject        string `json:"subject"`
	LoginChallenge string `json:"login_challenge"`
}

func (p *hydraProvider) GetConsentRequest(ctx context.Context, challenge string) (*identity.ConsentRequest, error) {
	consentURL := fmt.Sprintf("%s/oauth2/auth/requests/consent?consent_challenge=%s", p.adminURL, challenge)

	req, err := http.NewRequestWithContext(ctx, "GET", consentURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", identity.ErrOryUnreachable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get consent request details (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var consentInfo consentRequestResp
	if err := json.NewDecoder(resp.Body).Decode(&consentInfo); err != nil {
		return nil, err
	}

	return &identity.ConsentRequest{
		Subject:        consentInfo.Subject,
		LoginChallenge: consentInfo.LoginChallenge,
	}, nil
}

// ValidateIDToken verifies the RS256 signature of the ID Token using Hydra's JWKS,
// then validates nonce, audience, issuer, and expiration claims.
func (p *hydraProvider) ValidateIDToken(ctx context.Context, rawJWT string, expectedNonce string, expectedAudience string) (*identity.IDTokenClaims, error) {
	// Fetch JWKS from Hydra's well-known endpoint
	jwksURL := fmt.Sprintf("%s/.well-known/jwks.json", p.publicURL)
	req, err := http.NewRequestWithContext(ctx, "GET", jwksURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create JWKS request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to fetch JWKS: %v", identity.ErrOryUnreachable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("JWKS endpoint returned status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read JWKS response: %w", err)
	}

	// Parse the JWKS
	var jwks struct {
		Keys []json.RawMessage `json:"keys"`
	}
	if err := json.Unmarshal(bodyBytes, &jwks); err != nil {
		return nil, fmt.Errorf("failed to parse JWKS: %w", err)
	}

	if len(jwks.Keys) == 0 {
		return nil, fmt.Errorf("JWKS contains no keys")
	}

	// Split the JWT and decode header to find the kid
	parts := strings.Split(rawJWT, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT format")
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("failed to decode JWT header: %w", err)
	}

	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, fmt.Errorf("failed to parse JWT header: %w", err)
	}

	if header.Alg != "RS256" {
		return nil, fmt.Errorf("unsupported JWT algorithm: %s, expected RS256", header.Alg)
	}

	// Find the matching key by kid
	var matchingKeyJSON json.RawMessage
	for _, keyJSON := range jwks.Keys {
		var keyMeta struct {
			Kid string `json:"kid"`
			Use string `json:"use"`
		}
		if err := json.Unmarshal(keyJSON, &keyMeta); err != nil {
			continue
		}
		if keyMeta.Kid == header.Kid {
			matchingKeyJSON = keyJSON
			break
		}
	}

	if matchingKeyJSON == nil {
		return nil, fmt.Errorf("no matching key found in JWKS for kid: %s", header.Kid)
	}

	// Parse the RSA public key from JWK
	var jwk struct {
		N string `json:"n"`
		E string `json:"e"`
	}
	if err := json.Unmarshal(matchingKeyJSON, &jwk); err != nil {
		return nil, fmt.Errorf("failed to parse JWK: %w", err)
	}

	nBytes, err := base64.RawURLEncoding.DecodeString(jwk.N)
	if err != nil {
		return nil, fmt.Errorf("failed to decode JWK modulus: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(jwk.E)
	if err != nil {
		return nil, fmt.Errorf("failed to decode JWK exponent: %w", err)
	}

	// Convert exponent bytes to int
	var eVal int
	for _, b := range eBytes {
		eVal = eVal<<8 + int(b)
	}

	pubKey := &rsa.PublicKey{
		N: new(big.Int).SetBytes(nBytes),
		E: eVal,
	}

	// Verify the RS256 signature
	signedContent := parts[0] + "." + parts[1]
	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("failed to decode JWT signature: %w", err)
	}

	hash := sha256.Sum256([]byte(signedContent))
	if err := rsa.VerifyPKCS1v15(pubKey, crypto.SHA256, hash[:], sigBytes); err != nil {
		return nil, fmt.Errorf("JWT signature verification failed: %w", err)
	}

	// Decode and validate claims
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to decode JWT payload: %w", err)
	}

	var claims struct {
		Sub   string      `json:"sub"`
		Aud   interface{} `json:"aud"` // Can be string or []string
		Iss   string      `json:"iss"`
		Nonce string      `json:"nonce"`
		Exp   int64       `json:"exp"`
	}
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, fmt.Errorf("failed to parse JWT claims: %w", err)
	}

	// Validate nonce
	if claims.Nonce != expectedNonce {
		return nil, fmt.Errorf("nonce mismatch: expected %s, got %s", expectedNonce, claims.Nonce)
	}

	// Validate audience (can be string or []string)
	audValid := false
	switch v := claims.Aud.(type) {
	case string:
		audValid = v == expectedAudience
	case []interface{}:
		for _, a := range v {
			if s, ok := a.(string); ok && s == expectedAudience {
				audValid = true
				break
			}
		}
	}
	if !audValid {
		return nil, fmt.Errorf("audience mismatch: expected %s", expectedAudience)
	}

	// Validate issuer
	expectedIssuer := p.publicURL + "/"
	expectedBrowserIssuer := p.browserURL + "/"
	if claims.Iss != expectedIssuer && claims.Iss != p.publicURL &&
		claims.Iss != expectedBrowserIssuer && claims.Iss != p.browserURL {
		return nil, fmt.Errorf("issuer mismatch: expected %s or %s, got %s", expectedIssuer, expectedBrowserIssuer, claims.Iss)
	}

	// Validate expiration
	if time.Now().Unix() > claims.Exp {
		return nil, fmt.Errorf("ID token has expired")
	}

	// Extract audience as string
	var audStr string
	switch v := claims.Aud.(type) {
	case string:
		audStr = v
	case []interface{}:
		if len(v) > 0 {
			audStr, _ = v[0].(string)
		}
	}

	return &identity.IDTokenClaims{
		Sub:   claims.Sub,
		Aud:   audStr,
		Iss:   claims.Iss,
		Nonce: claims.Nonce,
		Exp:   claims.Exp,
	}, nil
}

// GetLoginRequest verifies that a login challenge is valid and pending by querying the Hydra Admin API.
// Returns true if the challenge is valid, false otherwise.
func (p *hydraProvider) GetLoginRequest(ctx context.Context, challenge string) (bool, error) {
	loginURL := fmt.Sprintf("%s/oauth2/auth/requests/login?login_challenge=%s", p.adminURL, challenge)

	req, err := http.NewRequestWithContext(ctx, "GET", loginURL, nil)
	if err != nil {
		return false, err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("%w: %v", identity.ErrOryUnreachable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		return false, nil
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("failed to get login request (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	return true, nil
}
