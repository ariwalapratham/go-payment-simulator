// Package bank is the port for an external payment acquirer.
//
// Callers depend on Gateway, not a concrete adapter. Business declines are
// AuthorizeResult values; a Go error means the call itself failed (canceled
// context, misconfiguration). Retry vs FAILED is decided by the processor,
// not by this package.
package bank
