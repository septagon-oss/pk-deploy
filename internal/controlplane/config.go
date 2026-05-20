package controlplane

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
)

// Config controls the HTTP control-plane process.
type Config struct {
	BindAddress         string
	KeyID               string
	Secret              []byte
	AdminToken          string
	EnvironmentID       string
	InventoryNamespaces []string
}

// ConfigFromEnv loads control-plane configuration from environment variables.
func ConfigFromEnv() (Config, error) {
	cfg := Config{
		BindAddress:         valueOr(os.Getenv("PK_DEPLOY_BIND"), ":8080"),
		KeyID:               valueOr(os.Getenv("PK_DEPLOY_KEY_ID"), "local"),
		AdminToken:          strings.TrimSpace(os.Getenv("PK_DEPLOY_ADMIN_TOKEN")),
		EnvironmentID:       valueOr(os.Getenv("PK_DEPLOY_ENVIRONMENT_ID"), "staging"),
		InventoryNamespaces: splitCSV(valueOr(os.Getenv("PK_DEPLOY_INVENTORY_NAMESPACES"), "platformkit-staging")),
	}
	secret, err := SharedSecretFromEnv()
	if err != nil {
		return Config{}, err
	}
	cfg.Secret = secret
	if cfg.AdminToken == "" {
		return Config{}, errors.New("PK_DEPLOY_ADMIN_TOKEN is required")
	}
	return cfg, nil
}

// SharedSecretFromEnv loads the shared job-signing secret without requiring
// control-plane-only settings.
func SharedSecretFromEnv() ([]byte, error) {
	return loadSecret(os.Getenv("PK_DEPLOY_SHARED_SECRET"))
}

func loadSecret(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("PK_DEPLOY_SHARED_SECRET is required")
	}
	if strings.HasPrefix(raw, "base64:") {
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(raw, "base64:"))
		if err != nil {
			return nil, fmt.Errorf("decode PK_DEPLOY_SHARED_SECRET: %w", err)
		}
		rawBytes := decoded
		if len(rawBytes) < 32 {
			return nil, errors.New("PK_DEPLOY_SHARED_SECRET must decode to at least 32 bytes")
		}
		return rawBytes, nil
	}
	if len(raw) < 32 {
		return nil, errors.New("PK_DEPLOY_SHARED_SECRET must be at least 32 bytes")
	}
	return []byte(raw), nil
}

func valueOr(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
