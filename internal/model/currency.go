package model

import "strings"

// NormalizeCurrency uppercases a code and reports whether it is on the allowlist (FR30).
func NormalizeCurrency(code string) (string, bool) {
	c := strings.ToUpper(strings.TrimSpace(code))
	switch c {
	case "USD", "EUR", "GBP", "INR", "JPY", "CAD", "AUD":
		return c, true
	default:
		return c, false
	}
}
