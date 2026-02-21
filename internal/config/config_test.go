package config

import (
	"testing"
	"time"
)

func clearEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"DUBLY_PASSWORD", "DUBLY_DOMAINS", "DUBLY_PORT", "DUBLY_DB_PATH",
		"DUBLY_GEOIP_PATH", "DUBLY_FLUSH_INTERVAL", "DUBLY_BUFFER_SIZE", "DUBLY_CACHE_SIZE",
	} {
		t.Setenv(key, "")
	}
}

func TestLoad_MinimalValid(t *testing.T) {
	clearEnv(t)
	t.Setenv("DUBLY_PASSWORD", "secret")
	t.Setenv("DUBLY_DOMAINS", "example.com")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port != "8080" {
		t.Errorf("port = %q, want %q", cfg.Port, "8080")
	}
	if cfg.DBPath != "./dubly.db" {
		t.Errorf("dbpath = %q, want %q", cfg.DBPath, "./dubly.db")
	}
	if cfg.FlushInterval != 30*time.Second {
		t.Errorf("flush interval = %v, want %v", cfg.FlushInterval, 30*time.Second)
	}
	if cfg.BufferSize != 50000 {
		t.Errorf("buffer size = %d, want %d", cfg.BufferSize, 50000)
	}
	if cfg.CacheSize != 10000 {
		t.Errorf("cache size = %d, want %d", cfg.CacheSize, 10000)
	}
}

func TestLoad_AllFieldsOverridden(t *testing.T) {
	clearEnv(t)
	t.Setenv("DUBLY_PASSWORD", "s3cret")
	t.Setenv("DUBLY_DOMAINS", "a.co,b.co")
	t.Setenv("DUBLY_PORT", "9090")
	t.Setenv("DUBLY_DB_PATH", "/tmp/test.db")
	t.Setenv("DUBLY_GEOIP_PATH", "/data/geo.mmdb")
	t.Setenv("DUBLY_FLUSH_INTERVAL", "10s")
	t.Setenv("DUBLY_BUFFER_SIZE", "500")
	t.Setenv("DUBLY_CACHE_SIZE", "200")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port != "9090" {
		t.Errorf("port = %q, want %q", cfg.Port, "9090")
	}
	if cfg.DBPath != "/tmp/test.db" {
		t.Errorf("dbpath = %q, want %q", cfg.DBPath, "/tmp/test.db")
	}
	if cfg.Password != "s3cret" {
		t.Errorf("password = %q, want %q", cfg.Password, "s3cret")
	}
	if len(cfg.Domains) != 2 || cfg.Domains[0] != "a.co" || cfg.Domains[1] != "b.co" {
		t.Errorf("domains = %v, want [a.co b.co]", cfg.Domains)
	}
	if cfg.GeoIPPath != "/data/geo.mmdb" {
		t.Errorf("geoip = %q, want %q", cfg.GeoIPPath, "/data/geo.mmdb")
	}
	if cfg.FlushInterval != 10*time.Second {
		t.Errorf("flush = %v, want %v", cfg.FlushInterval, 10*time.Second)
	}
	if cfg.BufferSize != 500 {
		t.Errorf("buffer = %d, want %d", cfg.BufferSize, 500)
	}
	if cfg.CacheSize != 200 {
		t.Errorf("cache = %d, want %d", cfg.CacheSize, 200)
	}
}

func TestLoad_MissingPassword(t *testing.T) {
	clearEnv(t)
	t.Setenv("DUBLY_DOMAINS", "example.com")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing password")
	}
	if err.Error() != "DUBLY_PASSWORD is required" {
		t.Errorf("error = %q, want %q", err.Error(), "DUBLY_PASSWORD is required")
	}
}

func TestLoad_MissingDomains(t *testing.T) {
	clearEnv(t)
	t.Setenv("DUBLY_PASSWORD", "secret")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing domains")
	}
	if err.Error() != "DUBLY_DOMAINS is required" {
		t.Errorf("error = %q, want %q", err.Error(), "DUBLY_DOMAINS is required")
	}
}

func TestLoad_ZeroBufferSize(t *testing.T) {
	clearEnv(t)
	t.Setenv("DUBLY_PASSWORD", "secret")
	t.Setenv("DUBLY_DOMAINS", "example.com")
	t.Setenv("DUBLY_BUFFER_SIZE", "0")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for zero buffer size")
	}
	if err.Error() != "DUBLY_BUFFER_SIZE must be positive" {
		t.Errorf("error = %q, want %q", err.Error(), "DUBLY_BUFFER_SIZE must be positive")
	}
}

func TestLoad_NegativeFlushInterval(t *testing.T) {
	clearEnv(t)
	t.Setenv("DUBLY_PASSWORD", "secret")
	t.Setenv("DUBLY_DOMAINS", "example.com")
	t.Setenv("DUBLY_FLUSH_INTERVAL", "-1s")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for negative flush interval")
	}
	if err.Error() != "DUBLY_FLUSH_INTERVAL must be positive" {
		t.Errorf("error = %q, want %q", err.Error(), "DUBLY_FLUSH_INTERVAL must be positive")
	}
}

func TestLoad_ZeroCacheSize(t *testing.T) {
	clearEnv(t)
	t.Setenv("DUBLY_PASSWORD", "secret")
	t.Setenv("DUBLY_DOMAINS", "example.com")
	t.Setenv("DUBLY_CACHE_SIZE", "0")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for zero cache size")
	}
	if err.Error() != "DUBLY_CACHE_SIZE must be positive" {
		t.Errorf("error = %q, want %q", err.Error(), "DUBLY_CACHE_SIZE must be positive")
	}
}

func TestLoad_DomainsTrimsWhitespace(t *testing.T) {
	clearEnv(t)
	t.Setenv("DUBLY_PASSWORD", "secret")
	t.Setenv("DUBLY_DOMAINS", " d1.co , d2.co , ")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Domains) != 2 {
		t.Fatalf("domains count = %d, want 2", len(cfg.Domains))
	}
	if cfg.Domains[0] != "d1.co" || cfg.Domains[1] != "d2.co" {
		t.Errorf("domains = %v, want [d1.co d2.co]", cfg.Domains)
	}
}

func TestLoad_InvalidDurationFallsBackToDefault(t *testing.T) {
	clearEnv(t)
	t.Setenv("DUBLY_PASSWORD", "secret")
	t.Setenv("DUBLY_DOMAINS", "example.com")
	t.Setenv("DUBLY_FLUSH_INTERVAL", "notaduration")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.FlushInterval != 30*time.Second {
		t.Errorf("flush = %v, want %v (default)", cfg.FlushInterval, 30*time.Second)
	}
}

func TestIsDomainAllowed_CaseInsensitive(t *testing.T) {
	cfg := &Config{Domains: []string{"Example.COM"}}
	if !cfg.IsDomainAllowed("example.com") {
		t.Error("expected example.com to match Example.COM")
	}
	if !cfg.IsDomainAllowed("EXAMPLE.COM") {
		t.Error("expected EXAMPLE.COM to match Example.COM")
	}
}

func TestIsDomainAllowed_NotInList(t *testing.T) {
	cfg := &Config{Domains: []string{"allowed.com"}}
	if cfg.IsDomainAllowed("notallowed.com") {
		t.Error("expected notallowed.com to not be allowed")
	}
}
