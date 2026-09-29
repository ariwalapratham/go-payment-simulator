# go-payment-simulator — agent context

## Goal
Payment gateway simulator (Go/Gin/Postgres). See `docs/ARD-payment-simulator.md`, `docs/api-db-design-payment-simulator.md`, and `docs/db-latest-design.md`.

## Phase
**State machine + idempotency (ARD days 3–4)** — POST/GET payments; capture/cancel/refund HTTP still 501.

## Package map (actual vs doc)
- HTTP: `internal/handler` + `internal/middleware` (doc: `internal/http`)
- Shared DB shapes + enums: `internal/model` — `DB*` rows in `db_models.go`, API DTOs in `service_*.go`, enums + `Transition` in `enum.go`
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
- [x] Payment status machine (`model.Transition`)
- [x] POST/GET `/v1/payments` with DB idempotency + request hash
- [x] Integration concurrency tests for idempotent create

## In progress
- (none)

## Explicitly deferred (do not implement yet)
- Fake bank, workers, webhooks, prometheus metrics
- Capture, cancel, refund HTTP (still 501)
- API-key auth (`api_key_hash` unused)

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

## Layers
Client → Router → Middleware → Handler → Service → Repository → DB.

- **Middleware:** cross-cutting only (request ID, tracing, recovery, access log, error envelope, `X-Merchant-Id`, `Idempotency-Key`). No business logic.
- **Handler:** HTTP only — bind JSON, status codes (201 vs 200), map service errors to the error envelope, endpoint logs.
- **Service:** orchestration, hashing, state rules. No Gin, no HTTP DTOs, no `*errs.HTTPError`.
- **Repository:** SQL / transactions. No HTTP types.
- **Model:** `DB*` rows in `db_models.go`; API request/response in `service_*.go`; enums + `Transition` in `enum.go`.
- Wire dependencies in `cmd/api/main.go` (composition root), not inside handlers.

## Migrations
- Schema only — no seed `INSERT`s.
- Add nullable column → backfill existing rows → `SET NOT NULL`. Do not use `NOT NULL DEFAULT ''` as a fake backfill.

## Conventions
- Amounts: int64 minor units
- Internal IDs: bigint; public IDs: UUID strings
- API prefix: `/v1`
- Idempotency: header `Idempotency-Key`, merchant `X-Merchant-Id`
- Seed merchant public_id: `11111111-1111-1111-1111-111111111111`
