ALTER TABLE idempotency_keys
    ADD COLUMN scope TEXT;

UPDATE idempotency_keys
SET scope = 'payment.create'
WHERE scope IS NULL;

ALTER TABLE idempotency_keys
    ALTER COLUMN scope SET NOT NULL;

ALTER TABLE idempotency_keys
    ADD CONSTRAINT idempotency_keys_scope_check
        CHECK (scope IN ('payment.create', 'payment.refund'));

ALTER TABLE idempotency_keys
    ADD COLUMN refund_id BIGINT REFERENCES refunds(id);

ALTER TABLE idempotency_keys
    DROP CONSTRAINT idempotency_keys_merchant_id_idempotency_key_key;

ALTER TABLE idempotency_keys
    ADD CONSTRAINT idempotency_keys_merchant_scope_key_unique
        UNIQUE (merchant_id, scope, idempotency_key);

-- Claim rows may have both FKs null; after attach, the FK must match scope.
ALTER TABLE idempotency_keys
    ADD CONSTRAINT idempotency_keys_result_check
        CHECK (
            (scope = 'payment.create' AND refund_id IS NULL)
            OR (scope = 'payment.refund' AND payment_id IS NULL)
        );
