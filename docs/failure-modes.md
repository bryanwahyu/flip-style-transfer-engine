# Failure Modes

Every failure scenario this system is designed to handle, with the expected system response and the integration test that proves it.

| # | Scenario | Expected System Behavior | Test |
|---|---|---|---|
| 1 | Client retries `POST /transfers` with same `Idempotency-Key` | Redis cache hit — return original 202 response, no second transfer created, no second debit | [`TestIntegrationIdempotency`](../test/integration/idempotency_test.go) |
| 2 | API crashes after debit, before bank call | Outbox relay republishes `transfer.debited`; worker resumes saga from `DEBITED` state, calls bank | [`TestIntegrationHappyPath`](../test/integration/happy_path_test.go) (resume from any state) |
| 3 | Bank API times out | Circuit breaker records failure; saga moves to `COMPENSATING`, reverses debit, marks `FAILED` | [`TestIntegrationSagaCompensation_BankPermanentFailure`](../test/integration/saga_compensation_test.go) |
| 4 | Bank API returns 500 permanently | After N failures circuit opens; saga compensates: reverse debit → mark FAILED, source balance restored | [`TestIntegrationSagaCompensation_BankPermanentFailure`](../test/integration/saga_compensation_test.go) |
| 5 | Outbox-relay crashes mid-publish | On restart, `PENDING` outbox rows are re-fetched and re-published (at-least-once); `FOR UPDATE SKIP LOCKED` prevents duplicate processing across concurrent relay instances | Structural guarantee — `status = 'PENDING'` until NATS ack confirmed |
| 6 | NATS consumer processes same event twice | Worker checks `transfer.state` before executing each saga step; already-completed steps are no-ops (idempotent) | State machine guards in `TransferSaga.Execute()` |
| 7 | Two concurrent transfers from same account, insufficient balance for both | `LockForUpdate` acquires a `SELECT FOR UPDATE` row-level lock; balance check runs inside the lock — exactly one transfer succeeds, the other fails with `insufficient funds` | [`TestIntegrationSagaCompensation_InsufficientFunds`](../test/integration/saga_compensation_test.go) |
| 8 | Reconciler finds ledger sum ≠ expected | Emits `LEDGER DRIFT DETECTED` structured error log with currency and signed total; accounts flagged for manual review | Reconciler design in [`cmd/reconciler/main.go`](../cmd/reconciler/main.go) |
| 9 | Circuit breaker opens after N bank failures | Subsequent calls return `ErrCircuitOpen` immediately; saga compensates without calling bank | [`TestIntegrationCircuitBreaker`](../test/integration/saga_compensation_test.go) |
| 10 | Same `Idempotency-Key` with different request body | `SetIfAbsent` returns false (key exists); command handler detects body fingerprint mismatch → 422 `idempotency_key_reused_with_different_payload` | Middleware in [`internal/interfaces/http/middleware.go`](../internal/interfaces/http/middleware.go) |

## Key Invariants

- **Balance is never cached**: `GET /accounts/:id/balance` always computes `SUM(signed_amount WHERE account_id = X)` from ledger entries. No column can drift.
- **Entries are immutable**: The `ledger_entries` table has no `UPDATE` or `DELETE` paths in application code. All corrections use reversing entries.
- **Double-entry always holds**: `ReversalPosting` and `NewPosting` both validate that debit + credit sum = 0 before persisting.
- **Outbox is transactional**: `OutboxWriter.Write` executes in the same DB connection as the business write — both commit or both roll back.
