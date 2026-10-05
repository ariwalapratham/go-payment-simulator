ALTER TABLE merchants
    ADD COLUMN webhook_url TEXT,
    ADD COLUMN webhook_secret TEXT;

UPDATE merchants
SET webhook_secret = encode(gen_random_bytes(32), 'hex')
WHERE webhook_secret IS NULL;

ALTER TABLE merchants
    ALTER COLUMN webhook_secret SET NOT NULL;
