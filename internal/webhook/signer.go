package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strconv"
	"time"
)

// DefaultSignatureSkew is the allowed |now - timestamp| window for VerifySignature.
const DefaultSignatureSkew = 5 * time.Minute

// SignBody returns hex(HMAC-SHA256(secret, unix + "." + body)) for X-Webhook-Signature.
func SignBody(secret string, ts int64, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(ts, 10)))
	mac.Write([]byte("."))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifySignature reports whether signature matches SignBody and ts is within skew of now.
func VerifySignature(secret string, ts int64, body []byte, signature string, now time.Time, skew time.Duration) bool {
	if ts < 1 {
		return false
	}
	if skew <= 0 {
		skew = DefaultSignatureSkew
	}
	if now.Sub(time.Unix(ts, 0)).Abs() > skew {
		return false
	}
	expected := SignBody(secret, ts, body)
	if len(expected) != len(signature) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(signature)) == 1
}
