ALTER TABLE idempotency_keys
    DROP COLUMN request_hash;

ALTER TABLE idempotency_keys
    ALTER COLUMN payment_id SET NOT NULL;
