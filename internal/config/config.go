// Package config loads and validates the mevius runtime configuration from
// environment variables. Validation is fail-fast: any missing or malformed
// value produces an error naming the offending variable.
package config

import (
	"encoding/hex"
	"fmt"
	"os"
)

const (
	EnvAPIToken  = "MEVIUS_API_TOKEN"
	EnvMasterKey = "MEVIUS_MASTER_KEY"
	EnvDBPath    = "MEVIUS_DB_PATH"
	EnvAddr      = "MEVIUS_ADDR"
)

const (
	DefaultDBPath = "./mevius.db"
	DefaultAddr   = ":8080"
)

// masterKeyHexLen is 32 raw bytes rendered as hex, i.e. a 256-bit key.
const masterKeyHexLen = 64

type Config struct {
	// APIToken guards every /api/v1 route except health. Never log it.
	APIToken string

	// MasterKey is the decoded 32-byte envelope encryption key.
	MasterKey []byte

	DBPath string
	Addr   string
}

func Load() (*Config, error) {
	cfg := &Config{
		DBPath: DefaultDBPath,
		Addr:   DefaultAddr,
	}

	token := os.Getenv(EnvAPIToken)
	if token == "" {
		return nil, fmt.Errorf("%s is required and must not be empty", EnvAPIToken)
	}
	cfg.APIToken = token

	rawKey := os.Getenv(EnvMasterKey)
	if rawKey == "" {
		return nil, fmt.Errorf("%s is required and must not be empty", EnvMasterKey)
	}
	if len(rawKey) != masterKeyHexLen {
		return nil, fmt.Errorf(
			"%s must be exactly %d hexadecimal characters, got %d",
			EnvMasterKey, masterKeyHexLen, len(rawKey),
		)
	}
	key, err := hex.DecodeString(rawKey)
	if err != nil {
		return nil, fmt.Errorf("%s must be valid hexadecimal: %w", EnvMasterKey, err)
	}
	cfg.MasterKey = key

	if v := os.Getenv(EnvDBPath); v != "" {
		cfg.DBPath = v
	}
	if v := os.Getenv(EnvAddr); v != "" {
		cfg.Addr = v
	}

	return cfg, nil
}
