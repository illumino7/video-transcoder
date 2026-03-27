package storage

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type S3Client struct {
	client *minio.Client
	logger *slog.Logger
}

type S3Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	UseSSL    bool
	Buckets   []string // list of buckets to auto-create
}

func NewS3Client(cfg S3Config, logger *slog.Logger) (*S3Client, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create minio client: %w", err)
	}

	ctx := context.Background()
	for _, bucket := range cfg.Buckets {
		exists, err := client.BucketExists(ctx, bucket)
		if err != nil {
			return nil, fmt.Errorf("failed to check bucket %s: %w", bucket, err)
		}
		if !exists {
			if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
				return nil, fmt.Errorf("failed to create bucket %s: %w", bucket, err)
			}
			logger.Info("created minio bucket", "bucket", bucket)
		}
	}

	s := &S3Client{
		client: client,
		logger: logger,
	}

	// Set read-only public policy on the streaming bucket so clients
	// can fetch HLS segments directly from MinIO without proxying.
	for _, bucket := range cfg.Buckets {
		if bucket == "streaming" {
			if err := s.SetBucketReadOnlyPolicy(ctx, bucket); err != nil {
				return nil, fmt.Errorf("failed to set read-only policy on %s: %w", bucket, err)
			}
			logger.Info("set read-only public policy on bucket", "bucket", bucket)
		}
	}

	return s, nil
}

// SetBucketReadOnlyPolicy applies a public read-only policy to a MinIO bucket.
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

// Upload uploads a single file to S3.
func (s *S3Client) Upload(ctx context.Context, bucket, objectKey, filePath, contentType string) error {
	_, err := s.client.FPutObject(ctx, bucket, objectKey, filePath, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return fmt.Errorf("failed to upload %s to %s: %w", objectKey, bucket, err)
	}
	s.logger.Info("uploaded to s3", "bucket", bucket, "key", objectKey)
	return nil
}

// UploadDir recursively uploads a local directory to S3 under the given prefix.
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
		// Use forward slashes for S3 keys
		objectKey = strings.ReplaceAll(objectKey, string(os.PathSeparator), "/")

		contentType := DetectContentType(path)
		return s.Upload(ctx, bucket, objectKey, path, contentType)
	})
}

// GetObject retrieves an object from S3 and returns it as a minio.Object reader.
func (s *S3Client) GetObject(ctx context.Context, bucket, objectKey string) (*minio.Object, error) {
	obj, err := s.client.GetObject(ctx, bucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get object %s from %s: %w", objectKey, bucket, err)
	}
	return obj, nil
}

// StatObject returns object info (useful for Content-Length, Content-Type).
func (s *S3Client) StatObject(ctx context.Context, bucket, objectKey string) (minio.ObjectInfo, error) {
	return s.client.StatObject(ctx, bucket, objectKey, minio.StatObjectOptions{})
}

// PresignedPutURL generates a presigned PUT URL for direct client uploads.
func (s *S3Client) PresignedPutURL(ctx context.Context, bucket, objectKey string, expiry time.Duration) (*url.URL, error) {
	presignedURL, err := s.client.PresignedPutObject(ctx, bucket, objectKey, expiry)
	if err != nil {
		return nil, fmt.Errorf("failed to generate presigned put URL for %s: %w", objectKey, err)
	}
	s.logger.Info("generated presigned put URL", "bucket", bucket, "key", objectKey)
	return presignedURL, nil
}

// Download downloads an object from S3 to a local file.
func (s *S3Client) Download(ctx context.Context, bucket, objectKey, destPath string) error {
	obj, err := s.client.GetObject(ctx, bucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return fmt.Errorf("failed to get object %s: %w", objectKey, err)
	}
	defer obj.Close()

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("failed to create directory for %s: %w", destPath, err)
	}

	file, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", destPath, err)
	}
	defer file.Close()

	if _, err := io.Copy(file, obj); err != nil {
		return fmt.Errorf("failed to download %s: %w", objectKey, err)
	}

	s.logger.Info("downloaded from s3", "bucket", bucket, "key", objectKey, "dest", destPath)
	return nil
}

// DetectContentType returns the appropriate MIME type for video and HLS files.
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
