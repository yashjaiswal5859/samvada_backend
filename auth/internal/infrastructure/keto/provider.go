package keto

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

type ketoProvider struct {
	writeURL string
	readURL  string
	client   *http.Client
}

func NewKetoProvider(writeURL, readURL string) identity.AuthorizationProvider {
	return &ketoProvider{
		writeURL: writeURL,
		readURL:  readURL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
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

type checkResp struct {
	Allowed bool `json:"allowed"`
}

func (p *ketoProvider) CreateRelationship(ctx context.Context, req identity.RelationshipRequest) error {
	patchURL := fmt.Sprintf("%s/relation-tuples", p.writeURL)
	actions := []patchAction{
		{
			Action: "insert",
			RelationTuple: relationTuple{
				Namespace: req.Namespace,
				Object:    req.Object,
				Relation:  req.Relation,
				SubjectID: req.Subject,
			},
		},
	}

	jsonBytes, err := json.Marshal(actions)
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "PATCH", patchURL, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("%w: %v", identity.ErrOryUnreachable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code from keto write: %d", resp.StatusCode)
	}

	return nil
}

func (p *ketoProvider) DeleteRelationship(ctx context.Context, req identity.RelationshipRequest) error {
	patchURL := fmt.Sprintf("%s/relation-tuples", p.writeURL)
	actions := []patchAction{
		{
			Action: "delete",
			RelationTuple: relationTuple{
				Namespace: req.Namespace,
				Object:    req.Object,
				Relation:  req.Relation,
				SubjectID: req.Subject,
			},
		},
	}

	jsonBytes, err := json.Marshal(actions)
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "PATCH", patchURL, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("%w: %v", identity.ErrOryUnreachable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code from keto delete: %d", resp.StatusCode)
	}

	return nil
}

func (p *ketoProvider) CheckPermission(ctx context.Context, req identity.PermissionRequest) (bool, error) {
	checkURL := fmt.Sprintf("%s/check?namespace=%s&object=%s&relation=%s&subject_id=%s",
		p.readURL,
		url.QueryEscape(req.Namespace),
		url.QueryEscape(req.Object),
		url.QueryEscape(req.Relation),
		url.QueryEscape(req.Subject),
	)

	httpReq, err := http.NewRequestWithContext(ctx, "GET", checkURL, nil)
	if err != nil {
		return false, err
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return false, fmt.Errorf("%w: %v", identity.ErrOryUnreachable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		return false, nil
	}

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("unexpected status code from keto check: %d", resp.StatusCode)
	}

	var check checkResp
	if err := json.NewDecoder(resp.Body).Decode(&check); err != nil {
		return false, err
	}

	return check.Allowed, nil
}

type relationTuplesResp struct {
	RelationTuples []relationTuple `json:"relation_tuples"`
}

func (p *ketoProvider) IsDeviceOwned(ctx context.Context, deviceID string) (bool, error) {
	queryURL := fmt.Sprintf("%s/relation-tuples?namespace=devices&object=%s&relation=owner",
		p.readURL,
		url.QueryEscape(deviceID),
	)

	httpReq, err := http.NewRequestWithContext(ctx, "GET", queryURL, nil)
	if err != nil {
		return false, err
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return false, fmt.Errorf("%w: %v", identity.ErrOryUnreachable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("unexpected status code from keto query: %d", resp.StatusCode)
	}

	var tuples relationTuplesResp
	if err := json.NewDecoder(resp.Body).Decode(&tuples); err != nil {
		return false, err
	}

	return len(tuples.RelationTuples) > 0, nil
}

func (p *ketoProvider) DeleteAllRelationshipsForSubject(ctx context.Context, subject string) error {
	queryURL := fmt.Sprintf("%s/relation-tuples?subject_id=%s", p.readURL, url.QueryEscape(subject))
	httpReq, err := http.NewRequestWithContext(ctx, "GET", queryURL, nil)
	if err != nil {
		return err
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("%w: %v", identity.ErrOryUnreachable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to query relationships for subject: status %d", resp.StatusCode)
	}

	var result relationTuplesResp
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	for _, tuple := range result.RelationTuples {
		req := identity.RelationshipRequest{
			Namespace: tuple.Namespace,
			Object:    tuple.Object,
			Relation:  tuple.Relation,
			Subject:   tuple.SubjectID,
		}
		if err := p.DeleteRelationship(ctx, req); err != nil {
			return err
		}
	}

	return nil
}

