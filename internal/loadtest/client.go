package loadtest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	maxResponseBytes = 1 << 20
	idleConnTimeout  = 90 * time.Second
)

// Client is a net/http caller for create, capture, and refund. It is used for
// the refund burst, where per-request keys and response bodies matter.
type Client struct {
	http   *http.Client
	base   string
	apiKey string
}

// NewClient builds a client with idle connections well above the transport default of 2.
func NewClient(baseURL, apiKey string, timeout time.Duration, idlePerHost int) (*Client, error) {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		return nil, errors.New("base url is required")
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("api key is required")
	}
	if timeout <= 0 {
		timeout = defaultClientTimeout
	}
	if idlePerHost < minIdleConns {
		idlePerHost = minIdleConns
	}
	return &Client{
		http: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:        idlePerHost,
				MaxIdleConnsPerHost: idlePerHost,
				IdleConnTimeout:     idleConnTimeout,
			},
		},
		base:   base,
		apiKey: apiKey,
	}, nil
}

type paymentBody struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type refundBody struct {
	ID string `json:"id"`
}

// CreatePayment posts a payment with idempotencyKey and returns the public id.
func (c *Client) CreatePayment(ctx context.Context, idempotencyKey string) (uuid.UUID, int, error) {
	var body paymentBody
	code, err := c.json(ctx, http.MethodPost, c.base+createPath, idempotencyKey, []byte(PaymentJSON), &body)
	if err != nil {
		return uuid.Nil, code, err
	}
	id, err := uuid.Parse(body.ID)
	if err != nil {
		return uuid.Nil, code, fmt.Errorf("create payment id: %w", err)
	}
	return id, code, nil
}

// Capture posts an empty capture for paymentID.
func (c *Client) Capture(ctx context.Context, paymentID uuid.UUID) (int, error) {
	url := c.base + fmt.Sprintf(capturePath, paymentID.String())
	code, err := c.json(ctx, http.MethodPost, url, "", nil, nil)
	if err != nil {
		return code, fmt.Errorf("capture %s: %w", paymentID, err)
	}
	return code, nil
}

// Refund posts a full-amount refund and returns the refund public id when present.
func (c *Client) Refund(ctx context.Context, paymentID uuid.UUID, idempotencyKey string) (uuid.UUID, int, error) {
	url := c.base + fmt.Sprintf(refundPath, paymentID.String())
	var body refundBody
	code, err := c.json(ctx, http.MethodPost, url, idempotencyKey, []byte(RefundJSON), &body)
	if err != nil {
		return uuid.Nil, code, err
	}
	if body.ID == "" {
		return uuid.Nil, code, nil
	}
	id, err := uuid.Parse(body.ID)
	if err != nil {
		return uuid.Nil, code, fmt.Errorf("refund id: %w", err)
	}
	return id, code, nil
}

func (c *Client) json(ctx context.Context, method, url, idempotencyKey string, body []byte, dest any) (int, error) {
	var rdr io.Reader
	if len(body) > 0 {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rdr)
	if err != nil {
		return 0, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set(headerAPIKey, c.apiKey)
	if idempotencyKey != "" {
		req.Header.Set(headerIdempotencyKey, idempotencyKey)
	}
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("%s %s: %w", method, url, err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes))
	if err != nil {
		return res.StatusCode, fmt.Errorf("read response: %w", err)
	}
	if dest == nil || len(raw) == 0 {
		return res.StatusCode, nil
	}
	if unmarshalErr := json.Unmarshal(raw, dest); unmarshalErr != nil {
		return res.StatusCode, fmt.Errorf("decode response: %w", unmarshalErr)
	}
	return res.StatusCode, nil
}
