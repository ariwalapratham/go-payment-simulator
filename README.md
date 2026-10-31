# go-payment-simulator

Payment gateway simulator (Go/Postgres): idempotency, async authorize, capture/refund, webhooks. No real money.

## Prerequisites

- Docker (full stack)
- Go 1.27+ and [Task](https://taskfile.dev) (local development only)

## Run with Docker

```bash
cp .env.example .env
task up
```

API on `:8080`, Postgres on host `:5434`. Migrations run on API startup.

```bash
curl localhost:8080/healthz
curl localhost:8080/readyz
```

Stop: `task down`

## Develop on the host

Postgres in Compose, API with `go run`:

```bash
cp .env.example .env
docker compose -f deployments/docker-compose.yaml up postgres -d
task run
```

Use `PAYMENTS_DATABASE_PORT=5434` in `.env` (default in `.env.example`).
