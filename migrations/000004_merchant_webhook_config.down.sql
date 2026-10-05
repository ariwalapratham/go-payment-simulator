ALTER TABLE merchants
    DROP COLUMN IF EXISTS webhook_secret;

ALTER TABLE merchants
    DROP COLUMN IF EXISTS webhook_url;
