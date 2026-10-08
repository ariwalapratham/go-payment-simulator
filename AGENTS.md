# go-payment-simulator — agent context

## Goal
Payment gateway simulator (Go/Gin/Postgres). See `docs/ARD-payment-simulator.md`, `docs/api-db-design-payment-simulator.md`, `docs/db-latest-design.md`, and `docs/api-db-new-changes.md` (merchant admin + webhook delivery contract).

## Phase
**Health + validation envelope complete** — `/healthz` (liveness), `/readyz` (DB ping), `error.field` on 400s, currency allowlist. API-key auth on payments/refunds already done. Outbound shape: `docs/api-db-new-changes.md` §3.11 (not older ARD §3.8).

## Package map (actual vs doc)
- HTTP: `internal/handler` + `internal/middleware` (doc: `internal/http`)
- Shared DB shapes + enums: `internal/model` — `DB*` rows in `db_models.go`, API DTOs in `service_*.go`, enums + `Transition` in `enum.go`
- Domain/services: `internal/service` (doc: `internal/payment`, `internal/refund`, `internal/idempotency`)
- Data: `internal/repository` (doc: sqlc/sqlx TBD)
- Workers: `internal/worker` (payment + webhook pools)
- Webhook: `internal/webhook` (payload §3.11, HMAC signer)
- Bank: `internal/bank` (`Gateway` + in-process `Simulator`; `bank.NewGateway`)
- Observability: `internal/observability` (`worker_id` context, `AuthorizeMetrics`)
- DB/migrate: `internal/database`, `migrations/` (embedded SQL)

## Done
- [x] go.mod, Taskfile, golangci-lint
- [x] Config (koanf + PAYMENTS_ prefix), observability defaults
- [x] pgx pool, migration runner (embed), server shell
- [x] Middleware skeleton (request ID, log, NR tracing, recovery)
- [x] `internal/errs`, `internal/sqlerr` (Postgres mapping)
- [x] main wired to server
- [x] docker-compose (Postgres only) + Dockerfile
- [x] SQL migrations (`000001_init`, `000002` request_hash, `000003` idempotency scope + refund_id, `000004` merchant webhook_url/secret)
- [x] /v1/health
- [x] Router stubs for /v1/payments, refunds (capture/cancel/refund implemented)
- [x] .env.example ↔ PAYMENTS_ alignment (+ envKeyTransform in config)
- [x] Taskfile integration test path
- [x] API error envelope middleware (`internal/middleware/errors.go`)
- [x] Domain 409/422 helpers + `repository.MapDBError` for sqlerr layer
- [x] Payment status machine (`model.Transition`)
- [x] POST/GET `/v1/payments` with DB idempotency + request hash
- [x] Integration concurrency tests for idempotent create
- [x] Fake bank port (`bank.Gateway`, simulator, `PAYMENTS_BANK_*` config)
- [x] Authorize worker pool (DB queue, `SKIP LOCKED` lease, retries/backoff)
- [x] Authorize worker logs (`worker_id`, `bank_outcome`, transition events) + `AuthorizeMetrics` nop hook
- [x] POST `/v1/payments/:id/capture` and `/cancel` with `FOR UPDATE` + `model.Transition`
- [x] Authorize finalize abandons write when the row is no longer `PENDING` (cancel vs lease)
- [x] POST `/v1/payments/:id/refund` + GET `/v1/refunds/:id` with `FOR UPDATE` balance math and scoped idempotency
- [x] Merchant admin + self-service (FR23–24, FR35 rotate, FR36): `PAYMENTS_ADMIN_API_KEY`, `X-Admin-Key`, `POST/GET/PATCH /v1/admin/merchants`, `POST …/rotate-key`, `GET/PATCH /v1/merchant/me` (`X-Api-Key`); `api_key` + `webhook_secret` returned only on create (key also on rotate)
- [x] Webhook outbox (FR18, FR25–26): enqueue in authorize/capture/refund txns when `webhook_url` set; `internal/webhook` signer + §3.11 body; `WebhookWorker` POST → `DELIVERED` on 2xx; `PAYMENTS_WEBHOOK_*` config
- [x] Webhook reliability (FR19–20): exponential backoff, max attempts, terminal `FAILED`; redelivery keeps the same `X-Webhook-Event-Id`
- [x] API-key auth on payments/refunds (Addendum 2 §2): `X-Api-Key` → `merchant_id` in context; missing/invalid → 401; `X-Merchant-Id` rejected; rotate-key invalidates immediately
- [x] `GET /healthz` (FR32, no DB) + `GET /readyz` (FR33, DB ping 503); `/v1/health` kept as liveness alias
- [x] Validation envelope `error.field` (FR29–31, §4): amount > 0; currency allowlist USD/EUR/GBP/INR/JPY/CAD/AUD

## In progress
- (none)

## Explicitly deferred (do not implement yet)
- Prometheus `/metrics`
- `GET /v1/payments` list/cursor (`api-db-new-changes` §3.8)

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

- **Middleware:** cross-cutting only (request ID, tracing, recovery, access log, error envelope, `Idempotency-Key`, `X-Admin-Key`, `X-Api-Key`). No business logic.
- **Handler:** HTTP only — bind JSON, status codes (201 vs 200), map service errors to the error envelope, endpoint logs. Never log `api_key` or `webhook_secret`.
- **Service:** orchestration, hashing, state rules. No Gin, no HTTP DTOs, no `*errs.HTTPError`.
- **Repository:** SQL / transactions. No HTTP types.
- **Model:** `DB*` rows in `db_models.go`; API request/response in `service_*.go`; enums + `Transition` in `enum.go`.
- **Worker:** polls `PENDING` rows (`SKIP LOCKED` lease); calls `bank.Gateway`; applies retry policy. No HTTP.
- Wire dependencies in `cmd/api/main.go` (composition root), not inside handlers.

## Migrations
- Schema only — no seed `INSERT`s.
- Add nullable column → backfill existing rows → `SET NOT NULL`. Do not use `NOT NULL DEFAULT ''` as a fake backfill.

## Conventions
- Amounts: int64 minor units
- Internal IDs: bigint; public IDs: UUID strings
- API prefix: `/v1`
- Idempotency: header `Idempotency-Key`; merchant from `X-Api-Key` (payments/refunds and `/merchant/me`). Do not accept `X-Merchant-Id`, body, or query `merchant_id`
- Admin: header `X-Admin-Key` = `PAYMENTS_ADMIN_API_KEY` (fail closed if unset)
- Merchant API key: header `X-Api-Key` (plaintext `sk_test_…`; store only `api_key_hash`; never log plaintext)
- Webhook outbound: `X-Webhook-Signature`, `X-Webhook-Event-Id`, body `{id,type,created_at,data}` per `api-db-new-changes` §3.11
- Integration tests create a merchant via `MerchantService.Create` (same as admin) and use the returned `api_key`
