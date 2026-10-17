// =============================================================================
// Samvada Platform — ABS Ory Keto ReBAC Client
// =============================================================================
// Package: keto
//
// Responsibilities:
//   1. Checks relationship-based access control (ReBAC) policies via Ory Keto read API.
//   2. Creates and manages relation tuples via Ory Keto write API.
//   3. Seeds and validates the special Admin role (platform#admin).
// =============================================================================

package keto

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"go.uber.org/zap"
)

// Client defines the ReBAC interface for Ory Keto authorization.
type Client interface {
	CheckPermission(ctx context.Context, namespace, object, relation, subjectID string) (bool, error)
	CreateRelationship(ctx context.Context, namespace, object, relation, subjectID string) error
	EnsureAdminRole(ctx context.Context, subjectID string) error
}

type ketoClient struct {
	readURL    string
	writeURL   string
	httpClient *http.Client
	log        *zap.Logger
}

// NewKetoClient creates a new Ory Keto client.
func NewKetoClient(readURL, writeURL string, log *zap.Logger) Client {
	return &ketoClient{
		readURL:  readURL,
		writeURL: writeURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		log: log.Named("keto_client"),
	}
}

type relationTuple struct {
	Namespace string `json:"namespace"`
	Object    string `json:"object"`
	Relation  string `json:"relation"`
	SubjectID string `json:"subject_id"`
}

type patchAction struct {
	Action        string        `json:"action"`
	RelationTuple relationTuple `json:"relation_tuple"`
}

type checkResponse struct {
	Allowed bool `json:"allowed"`
}

// CheckPermission queries Keto's check API: GET /check?namespace=...&object=...&relation=...&subject_id=...
func (c *ketoClient) CheckPermission(ctx context.Context, namespace, object, relation, subjectID string) (bool, error) {
	checkURL := fmt.Sprintf("%s/check?namespace=%s&object=%s&relation=%s&subject_id=%s",
		c.readURL,
		url.QueryEscape(namespace),
		url.QueryEscape(object),
		url.QueryEscape(relation),
		url.QueryEscape(subjectID),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, checkURL, nil)
	if err != nil {
		return false, fmt.Errorf("failed to construct keto check request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.log.Error("Ory Keto read endpoint unreachable", zap.Error(err), zap.String("url", checkURL))
		return false, fmt.Errorf("keto read service unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		return false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("unexpected status from keto check: %d", resp.StatusCode)
	}

	var res checkResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return false, fmt.Errorf("failed to decode keto check response: %w", err)
	}

	return res.Allowed, nil
}

// CreateRelationship inserts a relation tuple via PATCH /relation-tuples.
func (c *ketoClient) CreateRelationship(ctx context.Context, namespace, object, relation, subjectID string) error {
	patchURL := fmt.Sprintf("%s/relation-tuples", c.writeURL)
	actions := []patchAction{
		{
			Action: "insert",
			RelationTuple: relationTuple{
				Namespace: namespace,
				Object:    object,
				Relation:  relation,
				SubjectID: subjectID,
			},
		},
	}

	payload, err := json.Marshal(actions)
	if err != nil {
		return fmt.Errorf("failed to marshal keto relation action: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, patchURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to construct keto patch request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.log.Error("Ory Keto write endpoint unreachable", zap.Error(err), zap.String("url", patchURL))
		return fmt.Errorf("keto write service unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status from keto write: %d", resp.StatusCode)
	}

	return nil
}

// EnsureAdminRole registers the special Admin role relation tuples for a subject in Ory Keto.
func (c *ketoClient) EnsureAdminRole(ctx context.Context, subjectID string) error {
	// Primary convention: namespace "platform", object "platform", relation "admin"
	if err := c.CreateRelationship(ctx, "platform", "platform", "admin", subjectID); err != nil {
		c.log.Warn("Failed to insert platform/platform/admin tuple", zap.Error(err))
	}

	// Zanzibar role convention: namespace "platform", object "roles:admin", relation "member"
	if err := c.CreateRelationship(ctx, "platform", "roles:admin", "member", subjectID); err != nil {
		c.log.Warn("Failed to insert platform/roles:admin/member tuple", zap.Error(err))
	}

	return nil
}
