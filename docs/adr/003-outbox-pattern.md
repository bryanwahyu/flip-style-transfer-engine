# ADR 003: Transactional Outbox Pattern

## Status
Accepted — 2026-05-27

## Context

The API writes a `Transfer` record to PostgreSQL and must also publish a `transfer.requested` event to NATS so the worker can process the SAGA. The naive approach — write to DB then publish to NATS — has a critical failure window: if the process crashes between the two operations, the event is lost and the worker never picks up the transfer.

## Decision

Use the **Transactional Outbox** pattern:

1. The API writes the `Transfer` row AND an `outbox` row in a **single database transaction**.
2. A separate `outbox-relay` process polls `outbox WHERE status = 'PENDING'`, publishes each event to NATS JetStream, and marks the row `PUBLISHED` only after NATS confirms delivery.
3. The outbox relay uses `FOR UPDATE SKIP LOCKED` to allow multiple relay instances to run in parallel without double-publishing.

## Alternatives considered

- **Dual write (DB then NATS)** — rejected. Lost event if crash between writes. No recovery path.
- **Event sourcing / Kafka as source of truth** — over-engineered for this scope; Kafka is not in the stack.
- **Change Data Capture (Debezium → NATS)** — valid alternative, but adds Kafka/Debezium infrastructure. The outbox relay is simpler and sufficient.

## Consequences

**Positive:**
- Guaranteed delivery: if the relay crashes after publishing but before marking `PUBLISHED`, it re-publishes on restart. Consumers must be idempotent (they are — via `transfer.state` check).
- No dual-write risk: the outbox row and the business row commit atomically.
- Simple recovery: inspect `outbox WHERE status = 'PENDING'` to see stuck events.

**Negative:**
- Higher latency vs. direct NATS publish: adds one polling interval (≤500ms) before the event is delivered.
- Additional table to maintain (`outbox`).
- Relay must run continuously; if it's down, events queue in the DB rather than being lost.
