package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port          string
	DBPath        string
	Password      string
	Domains       []string
	GeoIPPath     string
	FlushInterval time.Duration
	BufferSize    int
	CacheSize     int
}

func Load() (*Config, error) {
	password := os.Getenv("DUBLY_PASSWORD")
	if password == "" {
		return nil, fmt.Errorf("DUBLY_PASSWORD is required")
	}

	domainsRaw := os.Getenv("DUBLY_DOMAINS")
	if domainsRaw == "" {
		return nil, fmt.Errorf("DUBLY_DOMAINS is required")
	}
	var domains []string
	for _, d := range strings.Split(domainsRaw, ",") {
		d = strings.TrimSpace(d)
		if d != "" {
			domains = append(domains, d)
		}
	}

	cfg := &Config{
		Port:          envOrDefault("DUBLY_PORT", "8080"),
		DBPath:        envOrDefault("DUBLY_DB_PATH", "./dubly.db"),
		Password:      password,
		Domains:       domains,
		GeoIPPath:     os.Getenv("DUBLY_GEOIP_PATH"),
		FlushInterval: parseDuration("DUBLY_FLUSH_INTERVAL", 30*time.Second),
		BufferSize:    parseInt("DUBLY_BUFFER_SIZE", 50000),
		CacheSize:     parseInt("DUBLY_CACHE_SIZE", 10000),
	}

	if cfg.FlushInterval <= 0 {
		return nil, fmt.Errorf("DUBLY_FLUSH_INTERVAL must be positive")
	}
	if cfg.BufferSize <= 0 {
		return nil, fmt.Errorf("DUBLY_BUFFER_SIZE must be positive")
	}
	if cfg.CacheSize <= 0 {
		return nil, fmt.Errorf("DUBLY_CACHE_SIZE must be positive")
	}

	return cfg, nil
}

func (c *Config) IsDomainAllowed(domain string) bool {
	for _, d := range c.Domains {
		if strings.EqualFold(d, domain) {
			return true
		}
	}
	return false
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func parseDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}
