package conf

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"bilibili-lite/internal/platform/auth"
)

// Config owns mall runtime settings. Concurrency limits are starting values, not measured capacity.
type Config struct {
	DSN              string
	Address          string
	Auth             auth.Config
	RedisAddress     string
	RedisPassword    string
	RedisPrefix      string
	EntryRate        int
	EntryBurst       int
	InnerConcurrency int
	IPRate           int
	IPBurst          int
	IPLimit          bool
	SeedEnabled      bool
}

func Load() (*Config, error) {
	config := &Config{
		DSN:     os.Getenv("MALL_DATABASE_SOURCE"),
		Address: os.Getenv("MALL_HTTP_ADDR"),
		Auth: auth.Config{
			Issuer:     os.Getenv("MALL_AUTH_ISSUER"),
			Secret:     os.Getenv("MALL_AUTH_SECRET"),
			AccessTTL:  2 * time.Hour,
			RefreshTTL: 30 * 24 * time.Hour,
		},
	}
	if config.Address == "" {
		config.Address = ":8001"
	}
	if config.DSN == "" || config.Auth.Issuer == "" || len(config.Auth.Secret) < 32 {
		return nil, fmt.Errorf("MALL_DATABASE_SOURCE, MALL_AUTH_ISSUER and MALL_AUTH_SECRET (32+ characters) are required")
	}
	config.RedisAddress = os.Getenv("MALL_REDIS_ADDRESS")
	if config.RedisAddress == "" {
		config.RedisAddress = "127.0.0.1:6379"
	}
	config.RedisPassword = os.Getenv("MALL_REDIS_PASSWORD")
	config.RedisPrefix = os.Getenv("MALL_REDIS_PREFIX")
	if config.RedisPrefix == "" {
		config.RedisPrefix = "bili-mall"
	}
	for _, v := range []struct {
		key      string
		dest     *int
		fallback int
	}{
		{key: "MALL_ENTRY_RATE", dest: &config.EntryRate, fallback: 200},
		{key: "MALL_ENTRY_BURST", dest: &config.EntryBurst, fallback: 40},
		{key: "MALL_INNER_CONCURRENCY", dest: &config.InnerConcurrency, fallback: 8},
		{key: "MALL_IP_RATE", dest: &config.IPRate, fallback: 20},
		{key: "MALL_IP_BURST", dest: &config.IPBurst, fallback: 40},
	} {
		*v.dest = v.fallback
		if raw := os.Getenv(v.key); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 1 || n > 100000 {
				return nil, fmt.Errorf("invalid %s", v.key)
			}
			*v.dest = n
		}
	}
	for _, v := range []struct {
		key  string
		dest *bool
	}{
		{key: "MALL_IP_LIMIT_ENABLED", dest: &config.IPLimit},
		{key: "MALL_SEED_ENABLED", dest: &config.SeedEnabled},
	} {
		if raw := os.Getenv(v.key); raw != "" {
			enabled, err := strconv.ParseBool(raw)
			if err != nil {
				return nil, fmt.Errorf("invalid %s", v.key)
			}
			*v.dest = enabled
		}
	}
	return config, nil
}
