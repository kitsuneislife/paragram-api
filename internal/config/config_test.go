package config

import (
	"strings"
	"testing"
)

// env returns a getenv with every required variable set, overridden by m.
func env(m map[string]string) func(string) string {
	base := map[string]string{
		"DATABASE_URL":        "postgres://x",
		"REDIS_URL":           "redis://x",
		"PARAGRAM_JWT_SECRET": strings.Repeat("s", minSecretLen),
	}
	for k, v := range m {
		base[k] = v
	}
	return func(k string) string { return base[k] }
}

func TestLoadDefaultsAddr(t *testing.T) {
	c, err := Load(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if c.Addr != ":8080" {
		t.Errorf("Addr = %q, want :8080", c.Addr)
	}
}

func TestLoadCORSOrigins(t *testing.T) {
	c, err := Load(env(map[string]string{
		"PARAGRAM_CORS_ORIGINS": " https://a.example, ,https://b.example ",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.CORSOrigins) != 2 || c.CORSOrigins[0] != "https://a.example" || c.CORSOrigins[1] != "https://b.example" {
		t.Errorf("CORSOrigins = %q", c.CORSOrigins)
	}
}

func TestLoadRejectsShortSecret(t *testing.T) {
	if _, err := Load(env(map[string]string{"PARAGRAM_JWT_SECRET": "short"})); err == nil {
		t.Fatal("want error")
	}
}

func TestLoadReportsAllMissing(t *testing.T) {
	_, err := Load(func(string) string { return "" })
	if err == nil {
		t.Fatal("want error")
	}
	for _, k := range []string{"DATABASE_URL", "REDIS_URL", "PARAGRAM_JWT_SECRET"} {
		if !strings.Contains(err.Error(), k) {
			t.Errorf("error %q does not mention %s", err, k)
		}
	}
}
