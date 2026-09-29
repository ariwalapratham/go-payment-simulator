ALTER TABLE idempotency_keys
    ALTER COLUMN payment_id DROP NOT NULL,
    ADD COLUMN request_hash TEXT;

UPDATE idempotency_keys
SET request_hash = encode(sha256((merchant_id::text || ':' || idempotency_key)::bytea), 'hex')
WHERE request_hash IS NULL;

ALTER TABLE idempotency_keys
    ALTER COLUMN request_hash SET NOT NULL;
