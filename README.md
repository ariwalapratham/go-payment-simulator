# go-payment-simulator

Payment gateway simulator (Go/Postgres): idempotency, async authorize, capture/refund, webhooks. No real money.

## Quick start (Docker only)

For reviewers and anyone without Go installed: both Postgres and the API run in containers.

```bash
cp .env.example .env
docker compose -f deployments/docker-compose.yaml up --build
```

API: `http://localhost:8080` · Postgres (optional host access): `localhost:5434` · Migrations run when the API starts.

```bash
curl localhost:8080/healthz
curl localhost:8080/readyz
```

Stop: `Ctrl+C`, or `docker compose -f deployments/docker-compose.yaml down`

## Local development (Go on your machine)

If you are changing Go code: run **only Postgres** in Docker and start the API on the host with `go run` (hot reload via your editor). Same `.env`, but the app connects to Postgres on `localhost:5434`.

```bash
cp .env.example .env
docker compose -f deployments/docker-compose.yaml up postgres -d
go run ./cmd/api
```

Optional: [Task](https://taskfile.dev) wraps the same flows — `task up` / `task down` (full stack), `task run` (API on host).
