# go-payment-simulator

A Go payment gateway simulator for concurrent idempotency, state transitions, async processing, and webhook delivery. No real money movement.

## Prerequisites

- Go 1.27+
- [Task](https://taskfile.dev)
- Docker

## Run locally

```bash
cp .env.example .env
task up
task run
```

Postgres only runs in Compose. The API runs on the host (`:8080`).

```bash
curl localhost:8080/v1/health
```

Create a merchant (plaintext `api_key` and `webhook_secret` are returned once):

```bash
curl -sS -X POST localhost:8080/v1/admin/merchants \
  -H 'Content-Type: application/json' \
  -H 'X-Admin-Key: dev-admin-key' \
  -d '{"name":"Acme Corp","webhook_url":"https://example.com/webhooks"}'
```

Create a payment (`X-Merchant-Id` is the merchant `id` from create). Integration tests also seed `11111111-1111-1111-1111-111111111111`:

```bash
curl -sS -X POST localhost:8080/v1/payments \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: demo-1' \
  -H 'X-Merchant-Id: 11111111-1111-1111-1111-111111111111' \
  -d '{"amount":5000,"currency":"USD"}'
```

Self-service webhook URL (`X-Api-Key` is the merchant API key from create):

```bash
curl -sS localhost:8080/v1/merchant/me \
  -H 'X-Api-Key: sk_test_...'
```

Optional: apply migrations with the CLI instead of startup (`migrate` must be installed):

```bash
task migrate:up
```
