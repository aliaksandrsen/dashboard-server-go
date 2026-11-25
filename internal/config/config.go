package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/robfig/cron/v3"
)

type Environment string

const (
	EnvDevelopment Environment = "development"
	EnvProduction  Environment = "production"
	EnvTest        Environment = "test"
)

type Config struct {
	NodeEnv            Environment
	Port               int
	DatabaseURL        string
	StripeSecretKey    string
	StripeSyncCron     string
	StripeSyncTimezone string
	RedisHost          string
	RedisPort          int
	RedisURL           string
	FrontendURL        string
	ThrottleTTLMS      int
	ThrottleLimit      int
}

// EffectiveRedisURL returns REDIS_URL when configured, or constructs redis://host:port.
func (c *Config) EffectiveRedisURL() string {
	if c.RedisURL != "" {
		return c.RedisURL
	}
	return fmt.Sprintf("redis://%s:%d", c.RedisHost, c.RedisPort)
}

// Load reads .env (if present) and validates environment variables.
func Load() (*Config, error) {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("load .env file: %w", err)
	}

	envMap := make(map[string]string)
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		envMap[key] = value
	}
	return LoadFromMap(envMap)
}

// LoadFromMap parses and validates configuration from an environment key-value map.
func LoadFromMap(raw map[string]string) (*Config, error) {
	sanitized := make(map[string]string, len(raw))
	for k, v := range raw {
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			continue
		}
		sanitized[k] = trimmed
	}

	var errs []string

	cfg := &Config{
		NodeEnv:            EnvDevelopment,
		Port:               3000,
		StripeSyncCron:     "5 00 * * *",
		StripeSyncTimezone: "America/New_York",
		RedisHost:          "localhost",
		RedisPort:          6379,
		ThrottleTTLMS:      60_000,
		ThrottleLimit:      100,
	}

	if val, ok := sanitized["NODE_ENV"]; ok {
		env := Environment(val)
		switch env {
		case EnvDevelopment, EnvProduction, EnvTest:
			cfg.NodeEnv = env
		default:
			errs = append(errs, "NODE_ENV: must be one of development, production, test")
		}
	}

	if val, ok := sanitized["PORT"]; ok {
		port, err := strconv.Atoi(val)
		if err != nil || port < 1 || port > 65535 {
			errs = append(errs, "PORT: must be an integer between 1 and 65535")
		} else {
			cfg.Port = port
		}
	}

	if val, ok := sanitized["DATABASE_URL"]; ok {
		cfg.DatabaseURL = val
	} else {
		errs = append(errs, "DATABASE_URL: must not be empty")
	}

	if val, ok := sanitized["STRIPE_SECRET_KEY"]; ok {
		cfg.StripeSecretKey = val
	} else if cfg.NodeEnv == EnvProduction {
		errs = append(errs, "STRIPE_SECRET_KEY: is required in production")
	}

	if val, ok := sanitized["STRIPE_SYNC_CRON"]; ok {
		cfg.StripeSyncCron = val
	}
	if _, err := cron.ParseStandard(cfg.StripeSyncCron); err != nil {
		errs = append(errs, fmt.Sprintf("STRIPE_SYNC_CRON: invalid cron expression (%v)", err))
	}

	if val, ok := sanitized["STRIPE_SYNC_TIMEZONE"]; ok {
		cfg.StripeSyncTimezone = val
	}
	if _, err := time.LoadLocation(cfg.StripeSyncTimezone); err != nil {
		errs = append(errs, fmt.Sprintf("STRIPE_SYNC_TIMEZONE: invalid timezone (%v)", err))
	}

	if val, ok := sanitized["REDIS_HOST"]; ok {
		cfg.RedisHost = val
	}

	if val, ok := sanitized["REDIS_PORT"]; ok {
		port, err := strconv.Atoi(val)
		if err != nil || port < 1 || port > 65535 {
			errs = append(errs, "REDIS_PORT: must be an integer between 1 and 65535")
		} else {
			cfg.RedisPort = port
		}
	}

	if val, ok := sanitized["REDIS_URL"]; ok {
		cfg.RedisURL = val
	}

	if val, ok := sanitized["FRONTEND_URL"]; ok {
		cfg.FrontendURL = val
	}

	if val, ok := sanitized["THROTTLE_TTL_MS"]; ok {
		ttl, err := strconv.Atoi(val)
		if err != nil || ttl < 1000 {
			errs = append(errs, "THROTTLE_TTL_MS: must be an integer >= 1000")
		} else {
			cfg.ThrottleTTLMS = ttl
		}
	}

	if val, ok := sanitized["THROTTLE_LIMIT"]; ok {
		limit, err := strconv.Atoi(val)
		if err != nil || limit < 1 {
			errs = append(errs, "THROTTLE_LIMIT: must be an integer >= 1")
		} else {
			cfg.ThrottleLimit = limit
		}
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("invalid environment configuration: %s", strings.Join(errs, "; "))
	}

	return cfg, nil
}
