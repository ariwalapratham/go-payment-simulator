package repository

import "github.com/ariwalapratham/go-payment-simulator/internal/sqlerr"

// MapDBError translates DB errors; services use domain *errs.HTTPError without this.
func MapDBError(err error) error {
	return sqlerr.HandleError(err)
}
