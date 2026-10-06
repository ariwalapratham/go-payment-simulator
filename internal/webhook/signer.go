package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
)

// SignBody returns hex(HMAC-SHA256(body, secret)) for X-Webhook-Signature.
func SignBody(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifySignature reports whether signature matches SignBody(secret, body).
func VerifySignature(secret string, body []byte, signature string) bool {
	expected := SignBody(secret, body)
	if len(expected) != len(signature) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(signature)) == 1
}
