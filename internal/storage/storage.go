package storage

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type S3Client struct {
	client        *minio.Client
	presignClient *minio.Client
	logger        *slog.Logger
}

type S3Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	UseSSL    bool
	Buckets   []string
	PublicURL string
}

// NewS3Client initializes and configures the S3 client connection and buckets.
func NewS3Client(cfg S3Config, logger *slog.Logger) (*S3Client, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: "us-east-1",
	})
	if err != nil {
		return nil, fmt.Errorf("create client: %w", err)
	}

	ctx := context.Background()
	for _, bucket := range cfg.Buckets {
		exists, err := client.BucketExists(ctx, bucket)
		if err != nil {
			return nil, fmt.Errorf("check bucket %s: %w", bucket, err)
		}
		if !exists {
			if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
				return nil, fmt.Errorf("create bucket %s: %w", bucket, err)
			}
			logger.Info("created bucket", "bucket", bucket)
		}
	}

	presignClient := client
	if cfg.PublicURL != "" {
		u, err := url.Parse(cfg.PublicURL)
		if err != nil {
			return nil, fmt.Errorf("parse public URL: %w", err)
		}
		
		useSSL := u.Scheme == "https"
		pc, err := minio.New(u.Host, &minio.Options{
			Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
			Secure: useSSL,
			Region: "us-east-1",
		})
		if err != nil {
			return nil, fmt.Errorf("create presign client: %w", err)
		}
		presignClient = pc
	}

	s := &S3Client{
		client:        client,
		presignClient: presignClient,
		logger:        logger,
	}

	for _, bucket := range cfg.Buckets {
		if bucket == "streaming" {
			if err := s.SetBucketReadOnlyPolicy(ctx, bucket); err != nil {
				return nil, fmt.Errorf("set read-only policy on %s: %w", bucket, err)
			}
			logger.Info("set read-only policy on bucket", "bucket", bucket)
		}
	}

	return s, nil
}

// SetBucketReadOnlyPolicy sets an anonymous read-only policy on the bucket.
func (s *S3Client) SetBucketReadOnlyPolicy(ctx context.Context, bucket string) error {
	policy := fmt.Sprintf(`{
		"Version": "2012-10-17",
		"Statement": [{
			"Effect": "Allow",
			"Principal": {"AWS": ["*"]},
			"Action": ["s3:GetObject"],
			"Resource": ["arn:aws:s3:::%s/*"]
		}]
	}`, bucket)
	return s.client.SetBucketPolicy(ctx, bucket, policy)
}

// Upload transfers a local file to S3.
func (s *S3Client) Upload(ctx context.Context, bucket, objectKey, filePath, contentType string) error {
	_, err := s.client.FPutObject(ctx, bucket, objectKey, filePath, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return fmt.Errorf("upload %s: %w", objectKey, err)
	}
	s.logger.Info("uploaded to s3", "bucket", bucket, "key", objectKey)
	return nil
}

// UploadDir recursively uploads all files in a directory to S3.
func (s *S3Client) UploadDir(ctx context.Context, bucket, prefix, localDir string) error {
	return filepath.Walk(localDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(localDir, path)
		if err != nil {
			return err
		}

		objectKey := filepath.Join(prefix, relPath)
		objectKey = strings.ReplaceAll(objectKey, string(os.PathSeparator), "/")

		contentType := DetectContentType(path)
		return s.Upload(ctx, bucket, objectKey, path, contentType)
	})
}

// Delete removes an object from the bucket.
func (s *S3Client) Delete(ctx context.Context, bucket, objectKey string) error {
	err := s.client.RemoveObject(ctx, bucket, objectKey, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("delete %s from %s: %w", objectKey, bucket, err)
	}
	s.logger.Info("deleted from s3", "bucket", bucket, "key", objectKey)
	return nil
}

// GetObject retrieves an object payload from S3.
func (s *S3Client) GetObject(ctx context.Context, bucket, objectKey string) (*minio.Object, error) {
	obj, err := s.client.GetObject(ctx, bucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("get object %s: %w", objectKey, err)
	}
	return obj, nil
}

// StatObject checks object metadata.
func (s *S3Client) StatObject(ctx context.Context, bucket, objectKey string) (minio.ObjectInfo, error) {
	return s.client.StatObject(ctx, bucket, objectKey, minio.StatObjectOptions{})
}

// PresignedPutURL generates a temporary upload URL.
func (s *S3Client) PresignedPutURL(ctx context.Context, bucket, objectKey string, expiry time.Duration, contentType string) (*url.URL, error) {
	header := make(http.Header)
	header.Set("Content-Type", contentType)

	presignedURL, err := s.presignClient.PresignHeader(ctx, http.MethodPut, bucket, objectKey, expiry, nil, header)
	if err != nil {
		return nil, fmt.Errorf("generate presigned put URL: %w", err)
	}
	s.logger.Info("generated presigned put URL", "bucket", bucket, "key", objectKey, "url", presignedURL.String())
	return presignedURL, nil
}

// Download saves an S3 object to the local filesystem.
func (s *S3Client) Download(ctx context.Context, bucket, objectKey, destPath string) error {
	obj, err := s.client.GetObject(ctx, bucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return fmt.Errorf("get object: %w", err)
	}
	defer obj.Close()

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}

	file, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer file.Close()

	if _, err := io.Copy(file, obj); err != nil {
		return fmt.Errorf("copy object: %w", err)
	}

	s.logger.Info("downloaded from s3", "bucket", bucket, "key", objectKey, "dest", destPath)
	return nil
}

// DetectContentType determines the MIME type based on file extension.
func DetectContentType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".m3u8":
		return "application/vnd.apple.mpegurl"
	case ".ts":
		return "video/MP2T"
	case ".m4s":
		return "video/iso.segment"
	case ".mp4":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".mkv":
		return "video/x-matroska"
	case ".avi":
		return "video/x-msvideo"
	case ".mov":
		return "video/quicktime"
	case ".wmv":
		return "video/x-ms-wmv"
	case ".flv":
		return "video/x-flv"
	default:
		return "application/octet-stream"
	}
}
