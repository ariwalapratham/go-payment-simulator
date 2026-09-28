# go-payment-simulator — agent context

## Goal
Payment gateway simulator (Go/Gin/Postgres). See `docs/ARD-payment-simulator.md`, `docs/api-db-design-payment-simulator.md`, and `docs/db-latest-design.md`.

## Phase
**Structure / platform (ARD days 1–2)** — no full payment business logic yet.

## Package map (actual vs doc)
- HTTP: `internal/handler` + `internal/middleware` (doc: `internal/http`)
- Shared DB shapes + enums: `internal/model` — `DB*` row structs with `db:` tags + `TableName()`, table name constants in `tables.go`, enums in `enum.go`
- Domain/services: `internal/service` (doc: `internal/payment`, `internal/refund`, `internal/idempotency`)
- Data: `internal/repository` (doc: sqlc/sqlx TBD)
- Workers: `internal/worker` (payment + webhook)
- Bank: **not created** (doc: `internal/bank`)
- DB/migrate: `internal/database`, `migrations/` (embedded SQL)

## Done
- [x] go.mod, Taskfile, golangci-lint
- [x] Config (koanf + PAYMENTS_ prefix), observability defaults
- [x] pgx pool, migration runner (embed), server shell
- [x] Middleware skeleton (request ID, log, NR tracing, recovery)
- [x] `internal/errs`, `internal/sqlerr` (Postgres mapping)
- [x] main wired to server
- [x] docker-compose (Postgres only) + Dockerfile
- [x] SQL migrations (`000001_init`, bigint + public_id)
- [x] /v1/health
- [x] Router stubs for /v1/payments, refunds (501)
- [x] .env.example ↔ PAYMENTS_ alignment (+ envKeyTransform in config)
- [x] Taskfile integration test path
- [x] API error envelope middleware (`internal/middleware/errors.go`)
- [x] Domain 409/422 helpers + `repository.MapDBError` for sqlerr layer

## In progress
- (none)

## Explicitly deferred (do not implement yet)
- State machine, idempotency concurrency, fake bank, workers, webhooks, prometheus metrics, full API handlers

## Commands
- `task up` / `task down`
- `task migrate:up` (needs migrate CLI + DATABASE_URL)
- `task test:unit` — `./tests/unit/...`
- `task test:integration` — `-tags=integration ./tests/integration/...` (Postgres)
- `task check` — fmt, vet, lint, race tests

## Testing
- No `*_test.go` under `internal/`; all tests live under `tests/`.
- **Unit:** `tests/unit/<area>/` — one file per area (`model/`, `middleware/`, `handlers/`, `service/`, …). Package name `*_test`, import `internal/...`.
- **Integration:** `tests/integration/` — `//go:build integration`, real Postgres, HTTP + DB.

## Conventions
- Amounts: int64 minor units
- Internal IDs: bigint; public IDs: UUID strings
- API prefix: `/v1`
- Idempotency: header `Idempotency-Key`, merchant `X-Merchant-Id`
