ALTER TABLE idempotency_keys
    DROP CONSTRAINT idempotency_keys_result_check;

ALTER TABLE idempotency_keys
    DROP CONSTRAINT idempotency_keys_merchant_scope_key_unique;

-- Same key is allowed across scopes; the old unique cannot coexist with those rows.
DELETE FROM idempotency_keys WHERE scope = 'payment.refund';

ALTER TABLE idempotency_keys
    ADD CONSTRAINT idempotency_keys_merchant_id_idempotency_key_key
        UNIQUE (merchant_id, idempotency_key);

ALTER TABLE idempotency_keys
    DROP COLUMN refund_id;

ALTER TABLE idempotency_keys
    DROP CONSTRAINT idempotency_keys_scope_check;

ALTER TABLE idempotency_keys
    DROP COLUMN scope;
