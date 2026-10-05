// =============================================================================
// Samvada Platform — S3 / MinIO Storage Infrastructure Adapter
// =============================================================================

package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

type S3Storage struct {
	endpoint  string
	bucket    string
	accessKey string
	secretKey string
	publicURL string
	client    *http.Client
	log       *zap.Logger
}

func NewS3Storage(
	endpoint, bucket, accessKey, secretKey, publicURL string,
	log *zap.Logger,
) *S3Storage {
	if publicURL == "" {
		publicURL = fmt.Sprintf("%s/%s", strings.TrimRight(endpoint, "/"), bucket)
	}
	return &S3Storage{
		endpoint:  strings.TrimRight(endpoint, "/"),
		bucket:    bucket,
		accessKey: accessKey,
		secretKey: secretKey,
		publicURL: strings.TrimRight(publicURL, "/"),
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		log: log.Named("s3_storage"),
	}
}

func (s *S3Storage) Upload(ctx context.Context, key string, data []byte, contentType string) (string, error) {
	url := fmt.Sprintf("%s/%s/%s", s.endpoint, s.bucket, strings.TrimLeft(key, "/"))
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("creating s3 upload request: %w", err)
	}

	if contentType == "" {
		contentType = "application/octet-stream"
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := s.client.Do(req)
	if err != nil {
		s.log.Error("Failed to upload object to S3/MinIO", zap.String("key", key), zap.Error(err))
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("s3 upload failed with status %d: %s", resp.StatusCode, string(body))
	}

	publicItemURL := fmt.Sprintf("%s/%s", s.publicURL, strings.TrimLeft(key, "/"))
	s.log.Debug("Uploaded to S3/MinIO", zap.String("key", key), zap.String("url", publicItemURL))
	return publicItemURL, nil
}

func (s *S3Storage) Download(ctx context.Context, key string) ([]byte, error) {
	url := fmt.Sprintf("%s/%s/%s", s.endpoint, s.bucket, strings.TrimLeft(key, "/"))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("s3 download failed with status: %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func (s *S3Storage) GetPresignedURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	return fmt.Sprintf("%s/%s", s.publicURL, strings.TrimLeft(key, "/")), nil
}

func (s *S3Storage) Delete(ctx context.Context, key string) error {
	url := fmt.Sprintf("%s/%s/%s", s.endpoint, s.bucket, strings.TrimLeft(key, "/"))
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("s3 delete failed with status: %d", resp.StatusCode)
	}
	return nil
}
