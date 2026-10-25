// Package blob stores files in any S3-compatible service (SeaweedFS, Garage,
// AWS S3, ...). Keys are plain paths such as "media/<uuid>/display.jpg".
package blob

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

var ErrNotFound = errors.New("blob not found")

type Config struct {
	Endpoint  string // host:port, no scheme
	AccessKey string
	SecretKey string
	Bucket    string
	Region    string
	UseSSL    bool
}

type Store struct {
	client *minio.Client
	bucket string
}

// Object is a stored file opened for reading. The caller must Close it.
type Object struct {
	io.ReadSeekCloser
	Size         int64
	ContentType  string
	LastModified time.Time
	ETag         string
}

// New connects and creates the bucket if it does not exist.
func New(ctx context.Context, config Config) (*Store, error) {
	client, err := minio.New(config.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(config.AccessKey, config.SecretKey, ""),
		Secure: config.UseSSL,
		Region: config.Region,
		// Path-style ("host/bucket/key") works with self-hosted services that
		// have no wildcard DNS for "bucket.host".
		BucketLookup: minio.BucketLookupPath,
	})
	if err != nil {
		return nil, fmt.Errorf("creating S3 client: %w", err)
	}

	exists, err := client.BucketExists(ctx, config.Bucket)
	if err != nil {
		return nil, fmt.Errorf("checking bucket: %w", err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, config.Bucket, minio.MakeBucketOptions{Region: config.Region}); err != nil {
			return nil, fmt.Errorf("creating bucket: %w", err)
		}
	}
	return &Store{client: client, bucket: config.Bucket}, nil
}

func (s *Store) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	if _, err := s.client.PutObject(ctx, s.bucket, key, r, size, minio.PutObjectOptions{ContentType: contentType}); err != nil {
		return fmt.Errorf("putting object: %w", err)
	}
	return nil
}

// Get returns ErrNotFound when the key does not exist.
func (s *Store) Get(ctx context.Context, key string) (*Object, error) {
	object, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting object: %w", err)
	}
	// GetObject is lazy; Stat makes the first request and surfaces a missing key.
	info, err := object.Stat()
	if err != nil {
		object.Close()
		if minio.ToErrorResponse(err).Code == minio.NoSuchKey {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("getting object info: %w", err)
	}
	return &Object{
		ReadSeekCloser: object,
		Size:           info.Size,
		ContentType:    info.ContentType,
		LastModified:   info.LastModified,
		ETag:           info.ETag,
	}, nil
}

// Delete does not return an error when the key does not exist.
func (s *Store) Delete(ctx context.Context, key string) error {
	if err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("deleting object: %w", err)
	}
	return nil
}
