# ADR 002: SAGA Pattern vs. 2PC for Distributed Transfers

## Status
Accepted — 2026-05-27

## Context

An interbank transfer spans at least three systems: our database, an external bank API, and our NATS event bus. We need atomicity across these — either the full transfer completes or it is fully rolled back. Two common approaches:

**2PC (Two-Phase Commit):** A coordinator locks resources across all participants, then either commits or aborts. Provides strong atomicity.

**SAGA:** A sequence of local transactions, each with a compensating transaction. If any step fails, compensations are run in reverse order to undo already-committed steps.

## Decision

Use the **SAGA pattern** with orchestration (a central `TransferSaga` coordinator).

## Alternatives considered

- **2PC** — rejected. External bank APIs do not participate in distributed transactions. We cannot acquire a lock on Flip's bank partner and hold it while we process. 2PC also requires all participants to be available simultaneously; any network partition causes a global lock.
- **Choreography SAGA** — considered. Events drive compensations via publish/subscribe. Rejected in favor of orchestration because: harder to trace the saga's current state, harder to resume after crash, higher debugging complexity. An orchestrator gives us a single `transfers.state` column as the resume point.

## Consequences

**Positive:**
- Works with systems that don't support 2PC (all external APIs).
- Crash-resilient: the orchestrator can resume from `transfers.state` after restart.
- Clear failure semantics: every step has a defined compensation.
- Audit trail: every state transition is recorded in `transfer_events`.

**Negative:**
- Intermediate states are visible (e.g., `DEBITED` before `CREDITED`). In practice this is acceptable because end users only see `PENDING` or `COMPLETED/FAILED`.
- Compensations must be correct — a bug in a compensating transaction leaves money in an inconsistent state. Mitigated by integration tests for every failure mode.
- Not serializable isolation across services — two concurrent transfers can interleave. Handled via `SELECT FOR UPDATE` at the debit step.
