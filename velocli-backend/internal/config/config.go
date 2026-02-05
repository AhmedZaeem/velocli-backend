package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Env               string
	HTTPHost          string
	HTTPPort          int
	DatabaseURL       string
	JWTSigningKey     string
	LemonAPIKey       string
	LemonStoreID      int64
	LemonProductID    *int64
	LemonVariantID    *int64
	LemonWebhookSecret string
	VerifyLemonWebhookSignature bool
	TokenTTL          time.Duration
}

func Load() (Config, error) {
	var cfg Config
	cfg.Env = strings.TrimSpace(getenv("APP_ENV", "dev"))
	_ = loadDotEnvFor(cfg.Env)

	cfg.HTTPHost = strings.TrimSpace(getenv("HTTP_HOST", "0.0.0.0"))
	cfg.HTTPPort = mustInt(getenv("HTTP_PORT", "8080"))
	cfg.DatabaseURL = strings.TrimSpace(os.Getenv("DATABASE_URL"))
	cfg.JWTSigningKey = strings.TrimSpace(os.Getenv("JWT_SIGNING_KEY"))
	cfg.LemonAPIKey = strings.TrimSpace(os.Getenv("LEMON_SQUEEZY_API_KEY"))
	cfg.LemonStoreID = mustInt64(os.Getenv("LEMON_SQUEEZY_STORE_ID"))
	cfg.LemonProductID = optionalInt64(os.Getenv("LEMON_SQUEEZY_PRODUCT_ID"))
	cfg.LemonVariantID = optionalInt64(os.Getenv("LEMON_SQUEEZY_VARIANT_ID"))
	cfg.LemonWebhookSecret = strings.TrimSpace(os.Getenv("LEMON_WEBHOOK_SECRET"))
	cfg.VerifyLemonWebhookSignature = strings.ToLower(strings.TrimSpace(getenv("LEMON_WEBHOOK_VERIFY", ""))) != "0"
	if strings.EqualFold(cfg.Env, "dev") && getenv("LEMON_WEBHOOK_VERIFY", "") == "" {
		cfg.VerifyLemonWebhookSignature = false
	}
	if strings.EqualFold(cfg.Env, "dev") && cfg.LemonWebhookSecret == "" {
		cfg.LemonWebhookSecret = "VELOCLI_DEV_WEBHOOK_SECRET"
	}
	cfg.TokenTTL = mustDuration(getenv("TOKEN_TTL", "168h"))

	var missing []string
	if cfg.DatabaseURL == "" {
		missing = append(missing, "DATABASE_URL")
	}
	if cfg.JWTSigningKey == "" {
		missing = append(missing, "JWT_SIGNING_KEY")
	}
	if !strings.EqualFold(cfg.Env, "dev") {
		if cfg.LemonAPIKey == "" {
			missing = append(missing, "LEMON_SQUEEZY_API_KEY")
		}
		if cfg.LemonStoreID == 0 {
			missing = append(missing, "LEMON_SQUEEZY_STORE_ID")
		}
		if cfg.LemonWebhookSecret == "" {
			missing = append(missing, "LEMON_WEBHOOK_SECRET")
		}
	}

	if len(missing) > 0 {
		return Config{}, errors.New("missing required env vars: " + strings.Join(missing, ", "))
	}

	return cfg, nil
}

func (c Config) HTTPAddr() string {
	return fmt.Sprintf("%s:%d", c.HTTPHost, c.HTTPPort)
}

func getenv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}

func mustInt(v string) int {
	i, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return 0
	}
	return i
}

func mustInt64(v string) int64 {
	i, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	if err != nil {
		return 0
	}
	return i
}

func optionalInt64(v string) *int64 {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	i, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return nil
	}
	return &i
}

func mustDuration(v string) time.Duration {
	d, err := time.ParseDuration(strings.TrimSpace(v))
	if err != nil {
		return 0
	}
	return d
}
