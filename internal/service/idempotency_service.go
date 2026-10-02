package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// RequestHash is SHA-256 of "amount:CURRENCY" for payment-create idempotency payload checks.
func RequestHash(amount int64, currency string) string {
	payload := fmt.Sprintf("%d:%s", amount, strings.ToUpper(currency))
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

// RefundRequestHash is SHA-256 of "refund:<paymentPublicID>:<amount>" so refund keys
// cannot collide with payment-create hashes even if the same Idempotency-Key string is reused.
func RefundRequestHash(paymentPublicID uuid.UUID, amount int64) string {
	payload := fmt.Sprintf("refund:%s:%d", paymentPublicID, amount)
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}
