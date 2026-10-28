// Package storage talks to the S3-compatible bucket that holds media
// (MinIO in development, R2 in production).
package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

var ErrNotFound = errors.New("storage: object not found")

type Config struct {
	Endpoint  string // e.g. http://localhost:9000
	AccessKey string
	SecretKey string
	Bucket    string
	PublicURL string // base URL clients fetch objects from
}

type Store struct {
	c         *minio.Client
	bucket    string
	publicURL string
}

func New(cfg Config) (*Store, error) {
	u, err := url.Parse(cfg.Endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("storage: bad endpoint %q", cfg.Endpoint)
	}
	c, err := minio.New(u.Host, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: u.Scheme == "https",
		// A fixed region skips a bucket-location lookup on every presign; R2 accepts it.
		Region:       "us-east-1",
		BucketLookup: minio.BucketLookupPath,
	})
	if err != nil {
		return nil, fmt.Errorf("storage: %w", err)
	}
	return &Store{c: c, bucket: cfg.Bucket, publicURL: strings.TrimRight(cfg.PublicURL, "/")}, nil
}

// PresignPut returns a URL the client uploads the object to with a plain PUT.
func (s *Store) PresignPut(ctx context.Context, key string, ttl time.Duration) (string, error) {
	u, err := s.c.PresignedPutObject(ctx, s.bucket, key, ttl)
	if err != nil {
		return "", fmt.Errorf("storage: presign %s: %w", key, err)
	}
	return u.String(), nil
}

// Size returns the object's size, or ErrNotFound.
func (s *Store) Size(ctx context.Context, key string) (int64, error) {
	info, err := s.c.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if minio.ToErrorResponse(err).Code == minio.NoSuchKey {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("storage: stat %s: %w", key, err)
	}
	return info.Size, nil
}

func (s *Store) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	obj, err := s.c.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("storage: get %s: %w", key, err)
	}
	return obj, nil
}

func (s *Store) Put(ctx context.Context, key string, data []byte, contentType string) error {
	_, err := s.c.PutObject(ctx, s.bucket, key, bytes.NewReader(data), int64(len(data)),
		minio.PutObjectOptions{ContentType: contentType, CacheControl: "public, max-age=31536000, immutable"})
	if err != nil {
		return fmt.Errorf("storage: put %s: %w", key, err)
	}
	return nil
}

// URL is where clients fetch a public object.
func (s *Store) URL(key string) string { return s.publicURL + "/" + key }

// Ping checks the bucket is reachable, for /readyz.
func (s *Store) Ping(ctx context.Context) error {
	ok, err := s.c.BucketExists(ctx, s.bucket)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("storage: bucket %q does not exist", s.bucket)
	}
	return nil
}
