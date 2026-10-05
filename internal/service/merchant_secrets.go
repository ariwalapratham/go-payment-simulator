package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

const (
	apiKeyPrefix = "sk_test_"
	secretBytes  = 32
)

// HashAPIKey is SHA-256 hex of the plaintext merchant API key (including sk_test_ prefix).
func HashAPIKey(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

func newMerchantSecrets() (string, string, string, error) {
	apiKey, apiKeyHash, err := newAPIKey()
	if err != nil {
		return "", "", "", err
	}
	webhookSecret, err := randomHex(secretBytes)
	if err != nil {
		return "", "", "", fmt.Errorf("webhook secret: %w", err)
	}
	return apiKey, apiKeyHash, webhookSecret, nil
}

func newAPIKey() (string, string, error) {
	keyHex, err := randomHex(secretBytes)
	if err != nil {
		return "", "", fmt.Errorf("api key: %w", err)
	}
	apiKey := apiKeyPrefix + keyHex
	return apiKey, HashAPIKey(apiKey), nil
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("read random: %w", err)
	}
	return hex.EncodeToString(b), nil
}
