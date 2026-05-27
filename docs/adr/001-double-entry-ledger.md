# ADR 001: Double-Entry Ledger

## Status
Accepted — 2026-05-27

## Context

We need a balance representation for customer accounts. The simplest option is a single `balance` column on the `accounts` table. However, in a money-movement system that handles transfers, compensation, and reconciliation, a cached balance column creates serious risks:

- If a bug posts one leg of a transfer without the other, the balance drifts silently.
- There is no audit trail for how a balance reached its current value.
- Reconciliation requires comparing two different data stores.
- Corrections require direct UPDATE of financial data, which is dangerous and unauditable.

## Decision

Implement a double-entry ledger where **every financial movement posts at least two entries that sum to zero**. Account balance is always computed as `SUM(signed_amount WHERE account_id = X)` from the immutable `ledger_entries` table — never stored as a column.

Rules:
1. Every posting must balance (debit + credit = 0 in signed representation).
2. Entries are immutable — the `ledger_entries` table has no `UPDATE` or `DELETE` paths.
3. Corrections happen by posting reversing entries (not by modifying existing ones).
4. Balances are always computed on-demand from entries.

## Alternatives considered

- **Single balance column** — rejected. Silent drift risk, no audit trail, no self-verification.
- **Balance column + separate audit log** — rejected. Two sources of truth diverge under failure; extra complexity with no correctness guarantee.
- **Event sourcing with snapshots** — considered; too complex for this scope. The double-entry ledger already provides event sourcing for financial data.

## Consequences

**Positive:**
- Balance is always derivable from first principles; drift is mathematically impossible.
- Full audit trail for every cent.
- Reconciliation is trivially `SUM(all entries) = 0`.
- Correctness property is enforced by the `NewPosting` constructor, not by discipline.

**Negative:**
- `GET /balance` requires a SQL aggregation, not a column read. Acceptable at current scale; can be solved with a materialized view or snapshot if needed.
- Slightly more complex writes (always two rows per movement).
