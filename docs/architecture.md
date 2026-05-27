# Architecture

## Overview

`flip-style-transfer-engine` implements an interbank money transfer service using four independently deployable binaries, all coordinated through PostgreSQL (source of truth), NATS JetStream (event bus), and Redis (idempotency).

## Component Diagram

```mermaid
graph TD
    Client -->|POST /transfers| API
    Client -->|GET /transfers/:id| API
    Client -->|GET /accounts/:id/balance| API

    API -->|writes transfer + outbox| DB[(PostgreSQL)]
    API -->|check/set idempotency| Redis[(Redis)]

    OutboxRelay -->|polls outbox table| DB
    OutboxRelay -->|publishes events| NATS[NATS JetStream]

    NATS -->|durable subscription| Worker
    Worker -->|reads/updates transfer| DB
    Worker -->|posts ledger entries| DB
    Worker -->|calls bank API| BankGW[Bank Gateway]
    Worker -->|writes outbox| DB

    Reconciler -->|reads all ledger entries| DB

    BankGW -->|circuit breaker| BankMock[Bank Mock]
```

## Sequence: Happy-Path Transfer

```mermaid
sequenceDiagram
    participant C as Client
    participant A as API
    participant DB as PostgreSQL
    participant R as Redis
    participant OR as OutboxRelay
    participant N as NATS
    participant W as Worker
    participant B as Bank

    C->>A: POST /transfers (Idempotency-Key: K)
    A->>R: GET idempotency:K → miss
    A->>DB: INSERT transfer (PENDING) + outbox row (ATOMIC)
    A->>R: SET idempotency:K = response (24h TTL)
    A-->>C: 202 Accepted {transfer_id, state: PENDING}

    OR->>DB: SELECT outbox WHERE status=PENDING FOR UPDATE SKIP LOCKED
    OR->>N: Publish "transfer.requested"
    OR->>DB: UPDATE outbox SET status=PUBLISHED

    N->>W: Deliver "transfer.requested"
    W->>DB: SELECT transfer FOR UPDATE (ReserveDebit step)
    W->>DB: INSERT ledger_entries (debit src, credit float)
    W->>DB: UPDATE transfer SET state=DEBITED

    W->>B: InitiateTransfer (with circuit breaker)
    B-->>W: {external_ref: "BANK-REF-..."}
    W->>DB: UPDATE transfer SET state=BANK_CALLED

    W->>DB: INSERT ledger_entries (debit float, credit dst)
    W->>DB: UPDATE transfer SET state=CREDITED → COMPLETED
    W-->>N: ACK message
```

## Transactional Outbox Pattern

The API and Worker never write directly to NATS. Instead:

1. **Business write** (INSERT transfer) and **event intent** (INSERT outbox) are committed in the same DB transaction.
2. The **OutboxRelay** polls the outbox table with `FOR UPDATE SKIP LOCKED` and publishes to NATS.
3. Only after NATS confirms delivery is the outbox row marked `PUBLISHED`.

This guarantees that a process crash between step 1 and step 2 will never lose an event — the relay will republish on restart.

## SAGA Orchestration

The `TransferSaga` is stateless and resumable. The transfer's `state` column in PostgreSQL is the single source of truth for where in the saga we are. The worker can crash and restart at any step — `Execute()` resumes from the current state.

```
PENDING → ReserveDebit → DEBITED
DEBITED → CallBankAPI → BANK_CALLED
BANK_CALLED → PostCredit → CREDITED
CREDITED → CompleteTransfer → COMPLETED

On failure at any step:
→ COMPENSATING → (reverse posted entries) → FAILED
```

## Double-Entry Ledger Invariants

1. Every posting creates exactly 2 entries: one DEBIT, one CREDIT.
2. The signed sum of all entries always equals zero per currency.
3. Entries are immutable — corrections use reversing entries.
4. Balance is always `SUM(signed_amount WHERE account_id = X)` — never a cached column.

## Circuit Breaker

Bank calls are wrapped in a circuit breaker (3-state: CLOSED → OPEN → HALF_OPEN):
- Opens after 5 consecutive failures.
- Resets after 30 seconds.
- In OPEN state, calls fail fast with `ErrCircuitOpen` — the saga compensates immediately.
