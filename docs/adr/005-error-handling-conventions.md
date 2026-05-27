# ADR 005: Error Handling Conventions

## Status
Accepted — 2026-05-27

## Context

In a distributed system with multiple layers (domain, application, infrastructure, HTTP), errors need to:
1. Preserve full context for debugging (where did the error originate?)
2. Be distinguishable by type so callers can make decisions
3. Not leak internal details to API consumers
4. Be consistent across the codebase

## Decision

### Domain errors are typed sentinel values

```go
// domain/transfer/errors.go
var (
    ErrInsufficientFunds        = errors.New("insufficient funds")
    ErrTransferAlreadyCompleted = errors.New("transfer already completed")
    ErrInvalidStateTransition   = errors.New("invalid state transition")
)
```

Callers use `errors.Is()` to distinguish them. Never use string matching.

### All errors are wrapped at layer boundaries

```go
// wrap with context at each layer crossing
return nil, fmt.Errorf("load transfer %s: %w", id, err)
```

The `%w` verb preserves the original error for `errors.Is()`/`errors.As()` checks. Wrapping adds the operation context without hiding the type.

### No `panic` outside `main`

All errors are returned as values. Panics are reserved for `main()` initialization failures and `MustXxx` constructor helpers in tests only.

### HTTP handler: translate domain errors, hide internals

```go
// handler.go
switch {
case errors.Is(err, transfer.ErrInsufficientFunds):
    writeError(w, 422, "insufficient_funds", err.Error())
default:
    writeError(w, 500, "internal_error", "internal server error")
}
```

Domain errors map to specific HTTP status codes and error codes. All other errors return a generic 500 — no stack traces or internal details leak.

### Structured logging, not error-only logs

Every error is logged with full context before it is translated or discarded:

```go
log.Error("saga execution failed",
    "error", err,
    "transfer_id", transferID.String(),
    "trace_id", traceID,
)
```

## Alternatives considered

- **Custom error types with `As()`** — overkill for this scope; sentinel errors with `Is()` are sufficient.
- **Error codes as integers** — rejected; string codes are more debuggable and don't require a registry.
- **Panic-on-error** — rejected; panics crash the entire process and bypass graceful shutdown.

## Consequences

**Positive:**
- Full call chain preserved in logs via wrapped errors.
- HTTP consumers get machine-readable error codes, not internal details.
- `errors.Is()` guards make domain error handling explicit and testable.
- Consistent pattern across all packages reduces cognitive overhead.

**Negative:**
- Verbose error wrapping adds boilerplate.
- Must remember to wrap at every layer boundary — not enforced by the compiler.
