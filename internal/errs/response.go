package errs

// API envelope from docs/api-db-design-payment-simulator.md §4.
type ErrorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

func NewErrorResponse(e *HTTPError, requestID string) ErrorResponse {
	return ErrorResponse{
		Error: ErrorBody{
			Code:      e.Code,
			Message:   e.Message,
			RequestID: requestID,
		},
	}
}
