// Package config loads runtime settings from environment variables.
package config

import (
	"errors"
	"fmt"
	"strings"

	"github.com/kitsuneislife/paragram-api/internal/storage"
)

type Config struct {
	Addr        string // HTTP listen address
	DatabaseURL string // Postgres connection string
	RedisURL    string // Redis connection string
	JWTSecret   []byte // signs access tokens
	CORSOrigins []string
	S3          storage.Config
}

const minSecretLen = 32

// Load reads settings through getenv (os.Getenv in production) and fails on
// any missing required value, so a misconfigured process never starts.
func Load(getenv func(string) string) (Config, error) {
	c := Config{
		Addr:        getenv("PARAGRAM_ADDR"),
		DatabaseURL: getenv("DATABASE_URL"),
		RedisURL:    getenv("REDIS_URL"),
		JWTSecret:   []byte(getenv("PARAGRAM_JWT_SECRET")),
		S3: storage.Config{
			Endpoint:  getenv("PARAGRAM_S3_ENDPOINT"),
			AccessKey: getenv("PARAGRAM_S3_ACCESS_KEY"),
			SecretKey: getenv("PARAGRAM_S3_SECRET_KEY"),
			Bucket:    getenv("PARAGRAM_S3_BUCKET"),
			PublicURL: getenv("PARAGRAM_S3_PUBLIC_URL"),
		},
	}
	if c.Addr == "" {
		c.Addr = ":8080"
	}
	for _, o := range strings.Split(getenv("PARAGRAM_CORS_ORIGINS"), ",") {
		if o = strings.TrimSpace(o); o != "" {
			c.CORSOrigins = append(c.CORSOrigins, o)
		}
	}

	var errs []error
	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}
	if c.RedisURL == "" {
		errs = append(errs, errors.New("REDIS_URL is required"))
	}
	if len(c.JWTSecret) < minSecretLen {
		errs = append(errs, fmt.Errorf("PARAGRAM_JWT_SECRET must be at least %d bytes", minSecretLen))
	}
	for k, v := range map[string]string{
		"PARAGRAM_S3_ENDPOINT":   c.S3.Endpoint,
		"PARAGRAM_S3_ACCESS_KEY": c.S3.AccessKey,
		"PARAGRAM_S3_SECRET_KEY": c.S3.SecretKey,
		"PARAGRAM_S3_BUCKET":     c.S3.Bucket,
		"PARAGRAM_S3_PUBLIC_URL": c.S3.PublicURL,
	} {
		if v == "" {
			errs = append(errs, errors.New(k+" is required"))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return Config{}, fmt.Errorf("config: %w", err)
	}
	return c, nil
}
