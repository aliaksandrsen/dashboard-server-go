package config

import (
	"strings"
	"testing"
)

func TestLoadFromMap_Defaults(t *testing.T) {
	cfg, err := LoadFromMap(map[string]string{
		"DATABASE_URL": "postgresql://postgres:postgres@localhost:5432/dashboard",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.NodeEnv != EnvDevelopment {
		t.Fatalf("expected development, got %q", cfg.NodeEnv)
	}
	if cfg.Port != 3000 {
		t.Fatalf("expected port 3000, got %d", cfg.Port)
	}
	if cfg.StripeSyncCron != "5 00 * * *" {
		t.Fatalf("expected default cron, got %q", cfg.StripeSyncCron)
	}
	if cfg.StripeSyncTimezone != "America/New_York" {
		t.Fatalf("expected America/New_York, got %q", cfg.StripeSyncTimezone)
	}
	if cfg.RedisHost != "localhost" || cfg.RedisPort != 6379 {
		t.Fatalf("unexpected redis defaults: %s:%d", cfg.RedisHost, cfg.RedisPort)
	}
	if cfg.EffectiveRedisURL() != "redis://localhost:6379" {
		t.Fatalf("unexpected redis url: %s", cfg.EffectiveRedisURL())
	}
	if cfg.ThrottleTTLMS != 60_000 || cfg.ThrottleLimit != 100 {
		t.Fatalf("unexpected throttle defaults: %d / %d", cfg.ThrottleTTLMS, cfg.ThrottleLimit)
	}
}

func TestLoadFromMap_IgnoresWhitespaceValues(t *testing.T) {
	cfg, err := LoadFromMap(map[string]string{
		"DATABASE_URL": "postgresql://postgres:postgres@localhost:5432/dashboard",
		"PORT":         "   ",
		"REDIS_URL":    "  ",
		"FRONTEND_URL": "",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port != 3000 {
		t.Fatalf("expected fallback default port 3000, got %d", cfg.Port)
	}
	if cfg.RedisURL != "" || cfg.EffectiveRedisURL() != "redis://localhost:6379" {
		t.Fatalf("expected empty RedisURL and host/port fallback, got %q", cfg.EffectiveRedisURL())
	}
}

func TestLoadFromMap_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantSub string
	}{
		{
			name:    "missing DATABASE_URL",
			env:     map[string]string{},
			wantSub: "DATABASE_URL",
		},
		{
			name: "missing STRIPE_SECRET_KEY in production",
			env: map[string]string{
				"NODE_ENV":     "production",
				"DATABASE_URL": "postgresql://localhost/db",
			},
			wantSub: "STRIPE_SECRET_KEY",
		},
		{
			name: "invalid PORT",
			env: map[string]string{
				"DATABASE_URL": "postgresql://localhost/db",
				"PORT":         "70000",
			},
			wantSub: "PORT",
		},
		{
			name: "invalid NODE_ENV",
			env: map[string]string{
				"DATABASE_URL": "postgresql://localhost/db",
				"NODE_ENV":     "staging",
			},
			wantSub: "NODE_ENV",
		},
		{
			name: "invalid THROTTLE_TTL_MS",
			env: map[string]string{
				"DATABASE_URL":    "postgresql://localhost/db",
				"THROTTLE_TTL_MS": "500",
			},
			wantSub: "THROTTLE_TTL_MS",
		},
		{
			name: "invalid STRIPE_SYNC_TIMEZONE",
			env: map[string]string{
				"DATABASE_URL":         "postgresql://localhost/db",
				"STRIPE_SYNC_TIMEZONE": "Mars/Olympus",
			},
			wantSub: "STRIPE_SYNC_TIMEZONE",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadFromMap(tc.env)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantSub)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("expected error containing %q, got %v", tc.wantSub, err)
			}
		})
	}
}

func TestLoadFromMap_ProductionWithStripeKey(t *testing.T) {
	cfg, err := LoadFromMap(map[string]string{
		"NODE_ENV":          "production",
		"DATABASE_URL":      "postgresql://localhost/db",
		"STRIPE_SECRET_KEY": "sk_live_123",
		"REDIS_URL":         "redis://cache:6380",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.EffectiveRedisURL() != "redis://cache:6380" {
		t.Fatalf("expected REDIS_URL override, got %s", cfg.EffectiveRedisURL())
	}
}
