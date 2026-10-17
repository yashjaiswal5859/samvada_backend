// =============================================================================
// Samvada Platform — ABS Storage Infrastructure Adapter
// =============================================================================

package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
)

// Storage defines the uniform contract for object persistence (local filesystem or S3).
type Storage interface {
	Upload(ctx context.Context, key string, data []byte, contentType string) (string, error)
	Download(ctx context.Context, key string) ([]byte, error)
	GetPresignedURL(ctx context.Context, key string, ttl time.Duration) (string, error)
	GetPresignedUploadURL(ctx context.Context, key string, contentType string, ttl time.Duration) (string, error)
	StatObject(ctx context.Context, key string) (int64, string, error)
	Delete(ctx context.Context, key string) error
}

// LocalStorage implements Storage using the local filesystem.
type LocalStorage struct {
	baseDir string
	baseURL string
	log     *zap.Logger
}

// NewLocalStorage initializes a local directory storage driver.
func NewLocalStorage(baseDir string, baseURL string, log *zap.Logger) (*LocalStorage, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}

	return &LocalStorage{
		baseDir: baseDir,
		baseURL: baseURL,
		log:     log.Named("local_storage"),
	}, nil
}

func (s *LocalStorage) Upload(ctx context.Context, key string, data []byte, contentType string) (string, error) {
	filePath := filepath.Join(s.baseDir, filepath.FromSlash(key))
	dir := filepath.Dir(filePath)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create target folder: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	s.log.Debug("Uploaded file to local storage", zap.String("key", key), zap.Int("bytes", len(data)))
	return fmt.Sprintf("%s/storage/%s", s.baseURL, key), nil
}

func (s *LocalStorage) Download(ctx context.Context, key string) ([]byte, error) {
	filePath := filepath.Join(s.baseDir, filepath.FromSlash(key))
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	return data, nil
}

func (s *LocalStorage) GetPresignedURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	// For local storage, returns the direct serving URL
	return fmt.Sprintf("%s/storage/%s", s.baseURL, key), nil
}

func (s *LocalStorage) GetPresignedUploadURL(ctx context.Context, key string, contentType string, ttl time.Duration) (string, error) {
	return fmt.Sprintf("%s/storage/%s", s.baseURL, key), nil
}

func (s *LocalStorage) StatObject(ctx context.Context, key string) (int64, string, error) {
	filePath := filepath.Join(s.baseDir, filepath.FromSlash(key))
	info, err := os.Stat(filePath)
	if err != nil {
		return 0, "", err
	}
	return info.Size(), "application/pdf", nil
}

func (s *LocalStorage) Delete(ctx context.Context, key string) error {
	filePath := filepath.Join(s.baseDir, filepath.FromSlash(key))
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete file: %w", err)
	}
	return nil
}
