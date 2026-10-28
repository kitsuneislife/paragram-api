package config

import (
	"strings"
	"testing"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadDefaultsAddr(t *testing.T) {
	c, err := Load(env(map[string]string{
		"DATABASE_URL": "postgres://x",
		"REDIS_URL":    "redis://x",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if c.Addr != ":8080" {
		t.Errorf("Addr = %q, want :8080", c.Addr)
	}
}

func TestLoadCORSOrigins(t *testing.T) {
	c, err := Load(env(map[string]string{
		"DATABASE_URL":          "postgres://x",
		"REDIS_URL":             "redis://x",
		"PARAGRAM_CORS_ORIGINS": " https://a.example, ,https://b.example ",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.CORSOrigins) != 2 || c.CORSOrigins[0] != "https://a.example" || c.CORSOrigins[1] != "https://b.example" {
		t.Errorf("CORSOrigins = %q", c.CORSOrigins)
	}
}

func TestLoadReportsAllMissing(t *testing.T) {
	_, err := Load(env(nil))
	if err == nil {
		t.Fatal("want error")
	}
	for _, k := range []string{"DATABASE_URL", "REDIS_URL"} {
		if !strings.Contains(err.Error(), k) {
			t.Errorf("error %q does not mention %s", err, k)
		}
	}
}
