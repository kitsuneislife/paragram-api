// Package config loads runtime settings from environment variables.
package config

import (
	"errors"
	"fmt"
)

type Config struct {
	Addr        string // HTTP listen address
	DatabaseURL string // Postgres connection string
	RedisURL    string // Redis connection string
}

// Load reads settings through getenv (os.Getenv in production) and fails on
// any missing required value, so a misconfigured process never starts.
func Load(getenv func(string) string) (Config, error) {
	c := Config{
		Addr:        getenv("PARAGRAM_ADDR"),
		DatabaseURL: getenv("DATABASE_URL"),
		RedisURL:    getenv("REDIS_URL"),
	}
	if c.Addr == "" {
		c.Addr = ":8080"
	}

	var errs []error
	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}
	if c.RedisURL == "" {
		errs = append(errs, errors.New("REDIS_URL is required"))
	}
	if err := errors.Join(errs...); err != nil {
		return Config{}, fmt.Errorf("config: %w", err)
	}
	return c, nil
}
