# flip-style-transfer-engine

> Idempotent, fault-tolerant interbank transfer service. Go, PostgreSQL, NATS, Redis.
> Built to demonstrate production-grade money-movement patterns: SAGA, double-entry ledger, transactional outbox.

[![Go Report](https://goreportcard.com/badge/github.com/bryanwahyu/flip-style-transfer-engine)](https://goreportcard.com/report/github.com/bryanwahyu/flip-style-transfer-engine)

## Why this exists

Interbank transfers look simple from the outside (`POST /transfers`) but hide a brutal class of bugs:
duplicate charges from network retries, partial failures that leave money "missing," ledger drift
from cached balances, lost events when a service dies mid-write. This repo is my reference
implementation of the patterns that prevent each of those — modeled on the real architectural
shape of Indonesian fintechs like Flip.

## Quickstart

```bash
make up           # spins postgres + nats + redis + api + worker + outbox-relay + reconciler
make seed         # creates demo accounts (Alice + Bob)
make demo         # runs a happy-path transfer end-to-end
make demo-fail    # runs a failure scenario + shows compensation
make test         # full test suite (unit + integration)
```

## Architecture

```
Client
  │
  ▼
┌─────────────────────────────────────────┐
│              API (chi)                   │
│  POST /transfers    GET /transfers/:id   │
│  GET /accounts/:id/balance               │
└────────────────┬────────────────────────┘
                 │ writes Transfer + Outbox
                 ▼
┌────────────────────────────────────────┐
│           PostgreSQL                    │
│  transfers  ledger_entries  outbox      │
│  accounts   transfer_events             │
│  processed_events                       │
└────────────────┬───────────────────────┘
                 │ polls outbox
                 ▼
┌────────────────────────────────────────┐
│         Outbox Relay                    │
│  polls PENDING rows → publishes to NATS │
└────────────────┬───────────────────────┘
                 │
                 ▼
┌────────────────────────────────────────┐
│      NATS JetStream (durable stream)    │
└────────────────┬───────────────────────┘
                 │
                 ▼
┌────────────────────────────────────────┐
│           Worker (SAGA)                 │
│  ReserveDebit → CallBank → PostCredit   │
│  ← Compensation on failure             │
└────────────────┬───────────────────────┘
                 │
                 ▼
┌────────────────────────────────────────┐
│         Bank Mock Gateway              │
│  (configurable: success/timeout/500)   │
└────────────────────────────────────────┘

Redis: idempotency store (24h TTL per Idempotency-Key)
Reconciler: cron job, checks ΣLedger = 0 per currency
```

See [docs/architecture.md](docs/architecture.md) for the full breakdown,
[docs/failure-modes.md](docs/failure-modes.md) for the failure scenarios this system handles,
and [docs/adr/](docs/adr/) for the design decisions and their trade-offs.

## Key design choices

- **Double-entry ledger** — every cent traceable, balances always reconstructible from `SUM(entries)`
- **SAGA over 2PC** — no distributed locks across bank APIs we don't control
- **Outbox pattern** — exactly-once *intent*, at-least-once *delivery*, idempotent *consumers*
- **Idempotency keys** — every mutating endpoint, 24h dedup window in Redis
- **Circuit breaker** — bank calls are wrapped; after 5 consecutive failures the circuit opens

## API

```bash
# Create a transfer (idempotency key required)
curl -X POST http://localhost:8080/v1/transfers \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $(uuidgen)" \
  -d '{"source_account_id":"...","dest_account_id":"...","amount":100000,"currency":"IDR"}'

# Get transfer status
curl http://localhost:8080/v1/transfers/{id}

# Get account balance (always live, computed from ledger)
curl http://localhost:8080/v1/accounts/{id}/balance
```

Full API reference: [docs/api.md](docs/api.md)

## Project layout

```
cmd/                  Four runnable binaries
internal/domain/      Pure domain logic (no infrastructure)
  ledger/             Money, LedgerEntry, Posting — double-entry core
  transfer/           Transfer aggregate + state machine
  account/            Account aggregate
internal/application/ Use cases
  command/            CreateTransfer command handler
  saga/               TransferSaga orchestrator
  port/               Interface definitions (repositories, gateways)
internal/infrastructure/
  postgres/           pgx/v5 repositories + outbox writer
  nats/               JetStream publisher + consumer
  redis/              Idempotency store
  bankmock/           Fake bank + circuit breaker wrapper
  circuitbreaker/     State-machine circuit breaker
  observability/      slog, prometheus metrics
internal/interfaces/http/  chi router + handlers + middleware
migrations/           SQL migrations (golang-migrate format)
test/integration/     Integration tests (require docker compose)
docs/                 Architecture, failure modes, ADRs, API reference
```

## License

MIT
