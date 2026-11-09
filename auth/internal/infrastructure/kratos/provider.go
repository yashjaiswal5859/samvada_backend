package kratos

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/ridesafety/auth-service/internal/platform/identity"
)

type kratosProvider struct {
	adminURL string
	client   *http.Client
}

func NewKratosProvider(adminURL string) identity.IdentityProvider {
	return &kratosProvider{
		adminURL: adminURL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

type createIdentityReq struct {
	SchemaID string      `json:"schema_id"`
	Traits   traitsEmail `json:"traits"`
	State    string      `json:"state"`
}

type traitsEmail struct {
	Email string `json:"email"`
}

type identityResp struct {
	ID string `json:"id"`
}

type oryError struct {
	Error struct {
		Code    int    `json:"code"`
		Status  string `json:"status"`
		Message string `json:"message"`
	} `json:"error"`
}

func (p *kratosProvider) CreateIdentity(ctx context.Context, email string) (string, error) {
	url := fmt.Sprintf("%s/admin/identities", p.adminURL)
	reqBody := createIdentityReq{
		SchemaID: "default",
		Traits: traitsEmail{
			Email: email,
		},
		State: "active",
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %v", identity.ErrOryUnreachable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		return "", identity.ErrIdentityAlreadyExists
	}

	if resp.StatusCode != http.StatusCreated {
		var oErr oryError
		if err := json.NewDecoder(resp.Body).Decode(&oErr); err == nil && oErr.Error.Message != "" {
			// Kratos can return 400 if user email format is duplicate too
			if oErr.Error.Code == http.StatusConflict || oErr.Error.Status == "Conflict" {
				return "", identity.ErrIdentityAlreadyExists
			}
			return "", fmt.Errorf("kratos error: %s", oErr.Error.Message)
		}
		return "", fmt.Errorf("unexpected status from kratos: %d", resp.StatusCode)
	}

	var idResp identityResp
	if err := json.NewDecoder(resp.Body).Decode(&idResp); err != nil {
		return "", err
	}

	return idResp.ID, nil
}

func (p *kratosProvider) DeleteIdentity(ctx context.Context, kratosID string) error {
	url := fmt.Sprintf("%s/admin/identities/%s", p.adminURL, kratosID)
	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", identity.ErrOryUnreachable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("unexpected status from kratos during delete: %d", resp.StatusCode)
	}

	return nil
}

func (p *kratosProvider) GetIdentityByEmail(ctx context.Context, email string) (string, error) {
	urlVal := fmt.Sprintf("%s/admin/identities?credentials_identifier=%s", p.adminURL, url.QueryEscape(email))
	req, err := http.NewRequestWithContext(ctx, "GET", urlVal, nil)
	if err != nil {
		return "", err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %v", identity.ErrOryUnreachable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status from kratos: %d", resp.StatusCode)
	}

	var identities []identityResp
	if err := json.NewDecoder(resp.Body).Decode(&identities); err != nil {
		return "", err
	}

	if len(identities) == 0 {
		return "", identity.ErrIdentityNotFound
	}

	return identities[0].ID, nil
}
