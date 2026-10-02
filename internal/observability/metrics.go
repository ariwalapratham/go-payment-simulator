package observability

// AuthorizeMetrics is a hook for authorize outcomes. Prometheus can implement this later.
type AuthorizeMetrics interface {
	ObserveAuthorizeAttempt(bankOutcome, toStatus string)
	ObserveAuthorizeRetry()
	ObserveClaimLost()
}

// NopAuthorizeMetrics discards observations.
type NopAuthorizeMetrics struct{}

func (NopAuthorizeMetrics) ObserveAuthorizeAttempt(string, string) {}
func (NopAuthorizeMetrics) ObserveAuthorizeRetry()                 {}
func (NopAuthorizeMetrics) ObserveClaimLost()                      {}
