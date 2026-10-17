// =============================================================================
// Samvada Platform — S3 / MinIO Storage Infrastructure Adapter
// =============================================================================

package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"go.uber.org/zap"
)

type S3Storage struct {
	endpoint    string
	bucket      string
	accessKey   string
	secretKey   string
	publicURL   string
	minioClient *minio.Client
	log         *zap.Logger
}

func NewS3Storage(
	endpoint, bucket, accessKey, secretKey, publicURL string,
	useSSL bool,
	log *zap.Logger,
) (*S3Storage, error) {
	cleanEndpoint := strings.TrimRight(endpoint, "/")
	cleanEndpoint = strings.TrimPrefix(cleanEndpoint, "http://")
	cleanEndpoint = strings.TrimPrefix(cleanEndpoint, "https://")

	minioClient, err := minio.New(cleanEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize S3/MinIO client: %w", err)
	}

	if publicURL == "" {
		scheme := "http"
		if useSSL {
			scheme = "https"
		}
		publicURL = fmt.Sprintf("%s://%s/%s", scheme, cleanEndpoint, bucket)
	}

	return &S3Storage{
		endpoint:    cleanEndpoint,
		bucket:      bucket,
		accessKey:   accessKey,
		secretKey:   secretKey,
		publicURL:   strings.TrimRight(publicURL, "/"),
		minioClient: minioClient,
		log:         log.Named("s3_storage"),
	}, nil
}

func (s *S3Storage) Upload(ctx context.Context, key string, data []byte, contentType string) (string, error) {
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	cleanKey := strings.TrimLeft(key, "/")

	_, err := s.minioClient.PutObject(ctx, s.bucket, cleanKey, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		s.log.Error("Failed to upload object to S3/MinIO", zap.String("key", cleanKey), zap.Error(err))
		return "", err
	}

	publicItemURL := fmt.Sprintf("%s/%s", s.publicURL, cleanKey)
	s.log.Debug("Uploaded to S3/MinIO", zap.String("key", cleanKey), zap.String("url", publicItemURL))
	return publicItemURL, nil
}

func (s *S3Storage) Download(ctx context.Context, key string) ([]byte, error) {
	cleanKey := strings.TrimLeft(key, "/")
	obj, err := s.minioClient.GetObject(ctx, s.bucket, cleanKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer obj.Close()

	return io.ReadAll(obj)
}

func (s *S3Storage) GetPresignedURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	cleanKey := strings.TrimLeft(key, "/")
	reqParams := make(url.Values)
	presignedURL, err := s.minioClient.PresignedGetObject(ctx, s.bucket, cleanKey, ttl, reqParams)
	if err != nil {
		return "", fmt.Errorf("generating presigned download url: %w", err)
	}

	if s.publicURL != "" {
		if pubURL, parseErr := url.Parse(s.publicURL); parseErr == nil && pubURL.Host != "" {
			presignedURL.Scheme = pubURL.Scheme
			presignedURL.Host = pubURL.Host
		}
	}

	return presignedURL.String(), nil
}

func (s *S3Storage) GetPresignedUploadURL(ctx context.Context, key string, contentType string, ttl time.Duration) (string, error) {
	cleanKey := strings.TrimLeft(key, "/")
	presignedURL, err := s.minioClient.PresignedPutObject(ctx, s.bucket, cleanKey, ttl)
	if err != nil {
		return "", fmt.Errorf("generating presigned upload url: %w", err)
	}

	if s.publicURL != "" {
		if pubURL, parseErr := url.Parse(s.publicURL); parseErr == nil && pubURL.Host != "" {
			presignedURL.Scheme = pubURL.Scheme
			presignedURL.Host = pubURL.Host
		}
	}

	return presignedURL.String(), nil
}

func (s *S3Storage) StatObject(ctx context.Context, key string) (int64, string, error) {
	cleanKey := strings.TrimLeft(key, "/")
	info, err := s.minioClient.StatObject(ctx, s.bucket, cleanKey, minio.StatObjectOptions{})
	if err != nil {
		return 0, "", fmt.Errorf("failed to stat S3 object %s: %w", cleanKey, err)
	}
	return info.Size, info.ContentType, nil
}

func (s *S3Storage) Delete(ctx context.Context, key string) error {
	cleanKey := strings.TrimLeft(key, "/")
	return s.minioClient.RemoveObject(ctx, s.bucket, cleanKey, minio.RemoveObjectOptions{})
}
