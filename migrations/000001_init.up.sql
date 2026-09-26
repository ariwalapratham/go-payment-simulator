CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE merchants (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_id    UUID        NOT NULL DEFAULT gen_random_uuid(),
    name         TEXT        NOT NULL,
    api_key_hash TEXT        NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (public_id),
    UNIQUE (api_key_hash)
);

CREATE TABLE payments (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_id        UUID        NOT NULL DEFAULT gen_random_uuid(),
    merchant_id      BIGINT      NOT NULL REFERENCES merchants(id),
    amount           BIGINT      NOT NULL CHECK (amount > 0),
    currency         TEXT        NOT NULL,
    status           TEXT        NOT NULL CHECK (status IN
                       ('PENDING','AUTHORIZED','CAPTURED','FAILED','CANCELLED','REFUNDED')),
    attempt_count    INT         NOT NULL DEFAULT 0,
    next_attempt_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_error       TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (public_id)
);

CREATE INDEX idx_payments_status_next_attempt ON payments(status, next_attempt_at);
CREATE INDEX idx_payments_merchant ON payments(merchant_id);

CREATE TABLE refunds (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_id    UUID        NOT NULL DEFAULT gen_random_uuid(),
    payment_id   BIGINT      NOT NULL REFERENCES payments(id),
    amount       BIGINT      NOT NULL CHECK (amount > 0),
    status       TEXT        NOT NULL CHECK (status IN ('PENDING','SUCCEEDED','FAILED')),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (public_id)
);

CREATE INDEX idx_refunds_payment ON refunds(payment_id);

CREATE TABLE idempotency_keys (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    merchant_id      BIGINT      NOT NULL REFERENCES merchants(id),
    idempotency_key  TEXT        NOT NULL,
    payment_id       BIGINT      NOT NULL REFERENCES payments(id),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (merchant_id, idempotency_key)
);

CREATE TABLE webhook_events (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_id    UUID        NOT NULL DEFAULT gen_random_uuid(),
    payment_id   BIGINT      NOT NULL REFERENCES payments(id),
    type         TEXT        NOT NULL CHECK (type IN
                   ('payment.authorized','payment.captured','payment.failed','payment.refunded')),
    payload      JSONB       NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (public_id)
);

CREATE INDEX idx_webhook_events_payment ON webhook_events(payment_id);

CREATE TABLE webhook_deliveries (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    webhook_event_id BIGINT      NOT NULL REFERENCES webhook_events(id),
    status           TEXT        NOT NULL CHECK (status IN ('PENDING','DELIVERED','FAILED')),
    attempt_count    INT         NOT NULL DEFAULT 0,
    next_attempt_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_error       TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (webhook_event_id)
);

CREATE INDEX idx_webhook_deliveries_status ON webhook_deliveries(status, next_attempt_at);
