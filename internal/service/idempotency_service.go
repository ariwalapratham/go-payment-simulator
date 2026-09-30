package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// RequestHash is SHA-256 of "amount:CURRENCY" for idempotency payload checks.
func RequestHash(amount int64, currency string) string {
	payload := fmt.Sprintf("%d:%s", amount, strings.ToUpper(currency))
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}
