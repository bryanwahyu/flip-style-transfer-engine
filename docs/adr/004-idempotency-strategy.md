# ADR 004: Idempotency Strategy

## Status
Accepted — 2026-05-27

## Context

Mobile clients and payment orchestrators retry HTTP requests on network failure. Without idempotency, a retry to `POST /transfers` creates a second transfer and double-charges the customer. This is one of the most common bugs in payment systems.

We need:
1. A way for clients to signal "this is a retry of a previous request."
2. A way for the server to detect and deduplicate retries.
3. Detection of misuse: same key, different payload.

## Decision

Require an `Idempotency-Key: <uuid>` header on all state-mutating endpoints.

**Server behavior:**
- **Miss (first call):** Process the request normally. Cache `{key → response}` in Redis with a 24-hour TTL. Return the real response.
- **Hit (retry with same body):** Return the cached response immediately. Do not re-process. Add `X-Idempotent-Replayed: true` header.
- **Conflict (same key, different body):** Return 422 `idempotency_key_reused_with_different_payload`. Clients must use a new key per unique request.
- **Missing key on mutating endpoint:** Return 400 `idempotency_key_required`.

**Storage:** Redis with TTL-based expiry. Redis `SETNX` (set-if-not-exists) provides atomic first-writer-wins semantics without a distributed lock.

## Alternatives considered

- **Database-based deduplication** (unique constraint on `idempotency_key` in `transfers` table) — viable backup, but slower than Redis and couples dedup logic to the data model.
- **No idempotency** — rejected. Double-charges are a P0 incident in any fintech.
- **Client-side dedup only** — rejected. Network retry behavior is not controlled by the server.

## Consequences

**Positive:**
- Clients can retry safely on any network failure.
- 24-hour window covers all reasonable retry scenarios.
- Redis `SETNX` is O(1) and adds <1ms overhead per request.
- Conflict detection (same key, different body) prevents accidental key reuse.

**Negative:**
- Requires Redis as an additional dependency.
- 24-hour TTL means the same key cannot be reused for a legitimately different transaction within that window.
- Redis restart loses the cache — mitigated by also writing the `transfers` table, which allows recovery in most cases.
