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

Optional: apply migrations with the CLI instead of startup (`migrate` must be installed):

```bash
task migrate:up
```
