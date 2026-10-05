package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ariwalapratham/go-payment-simulator/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// merchantCols omits webhook_secret so list/get/auth never load it into memory.
	merchantCols      = `id, public_id, name, api_key_hash, webhook_url, created_at`
	merchantListLimit = 100
)

// MerchantRepository is the merchants SQL layer.
type MerchantRepository struct {
	pool *pgxpool.Pool
}

// NewMerchantRepository loads and mutates merchant rows.
func NewMerchantRepository(pool *pgxpool.Pool) *MerchantRepository {
	return &MerchantRepository{pool: pool}
}

// Create inserts a merchant. webhookURL nil stores SQL NULL.
func (r *MerchantRepository) Create(
	ctx context.Context,
	name, apiKeyHash string,
	webhookURL *string,
	webhookSecret string,
) (*model.DBMerchant, error) {
	const q = `
INSERT INTO merchants (name, api_key_hash, webhook_url, webhook_secret)
VALUES ($1, $2, $3, $4)
RETURNING ` + merchantCols

	m, err := scanMerchant(r.pool.QueryRow(ctx, q, name, apiKeyHash, webhookURL, webhookSecret))
	if err != nil {
		if isUniqueAPIKeyHash(err) {
			return nil, ErrDuplicateAPIKeyHash
		}
		return nil, fmt.Errorf("create merchant: %w", err)
	}
	return m, nil
}

// GetByPublicID loads one merchant by API-facing UUID.
func (r *MerchantRepository) GetByPublicID(ctx context.Context, publicID uuid.UUID) (*model.DBMerchant, error) {
	const q = `SELECT ` + merchantCols + ` FROM merchants WHERE public_id = $1`

	m, err := scanMerchant(r.pool.QueryRow(ctx, q, publicID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrMerchantNotFound
		}
		return nil, fmt.Errorf("merchant by public id: %w", err)
	}
	return m, nil
}

// GetByAPIKeyHash loads the merchant that owns this hashed API key.
func (r *MerchantRepository) GetByAPIKeyHash(ctx context.Context, apiKeyHash string) (*model.DBMerchant, error) {
	const q = `SELECT ` + merchantCols + ` FROM merchants WHERE api_key_hash = $1`

	m, err := scanMerchant(r.pool.QueryRow(ctx, q, apiKeyHash))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrMerchantNotFound
		}
		return nil, fmt.Errorf("merchant by api key hash: %w", err)
	}
	return m, nil
}

// List returns merchants ordered by internal id, capped at merchantListLimit.
func (r *MerchantRepository) List(ctx context.Context) ([]model.DBMerchant, error) {
	const q = `SELECT ` + merchantCols + ` FROM merchants ORDER BY id ASC LIMIT $1`

	rows, err := r.pool.Query(ctx, q, merchantListLimit)
	if err != nil {
		return nil, fmt.Errorf("list merchants: %w", err)
	}
	defer rows.Close()

	out := make([]model.DBMerchant, 0)
	for rows.Next() {
		m, scanErr := scanMerchant(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan merchant: %w", scanErr)
		}
		out = append(out, *m)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("list merchants: %w", rowsErr)
	}
	return out, nil
}

// Update patches name and/or webhook_url. webhookURLSet false leaves the URL unchanged;
// webhookURLSet true writes webhookURL (nil clears it).
func (r *MerchantRepository) Update(
	ctx context.Context,
	publicID uuid.UUID,
	name *string,
	webhookURLSet bool,
	webhookURL *string,
) (*model.DBMerchant, error) {
	const q = `
UPDATE merchants
SET name = COALESCE($2, name),
    webhook_url = CASE WHEN $3 THEN $4 ELSE webhook_url END
WHERE public_id = $1
RETURNING ` + merchantCols

	m, err := scanMerchant(r.pool.QueryRow(ctx, q, publicID, name, webhookURLSet, webhookURL))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrMerchantNotFound
		}
		return nil, fmt.Errorf("update merchant: %w", err)
	}
	return m, nil
}

// RotateAPIKeyHash writes newHash only if api_key_hash still equals oldHash (compare-and-set).
func (r *MerchantRepository) RotateAPIKeyHash(
	ctx context.Context,
	publicID uuid.UUID,
	oldHash, newHash string,
) (*model.DBMerchant, error) {
	const q = `
UPDATE merchants
SET api_key_hash = $3
WHERE public_id = $1 AND api_key_hash = $2
RETURNING ` + merchantCols

	m, err := scanMerchant(r.pool.QueryRow(ctx, q, publicID, oldHash, newHash))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrMerchantNotFound
		}
		if isUniqueAPIKeyHash(err) {
			return nil, ErrDuplicateAPIKeyHash
		}
		return nil, fmt.Errorf("rotate merchant api key: %w", err)
	}
	return m, nil
}

func scanMerchant(row pgx.Row) (*model.DBMerchant, error) {
	var m model.DBMerchant
	if err := row.Scan(
		&m.ID,
		&m.PublicID,
		&m.Name,
		&m.APIKeyHash,
		&m.WebhookURL,
		&m.CreatedAt,
	); err != nil {
		return nil, err
	}
	return &m, nil
}

func isUniqueAPIKeyHash(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return false
	}
	return strings.Contains(pgErr.ConstraintName, "api_key_hash")
}
