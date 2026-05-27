# API Reference

Base URL: `http://localhost:8080/v1`

All state-mutating endpoints require the `Idempotency-Key` header.

---

## POST /v1/transfers

Initiates a new interbank transfer. Returns `202 Accepted` immediately — the transfer is processed asynchronously by the worker.

**Headers**

| Header | Required | Description |
|---|---|---|
| `Content-Type` | ✓ | `application/json` |
| `Idempotency-Key` | ✓ | UUID; deduplication window 24h |

**Request body**

```json
{
  "source_account_id": "uuid",
  "dest_account_id":   "uuid",
  "amount":            100000,
  "currency":          "IDR",
  "description":       "optional note"
}
```

`amount` is in the smallest unit (e.g., rupiah for IDR — no decimal).

**Responses**

| Status | Meaning |
|---|---|
| `202 Accepted` | Transfer accepted and queued |
| `200 OK` with `X-Idempotent-Replayed: true` | Duplicate request — original response replayed |
| `400 Bad Request` | Missing `Idempotency-Key` header (`idempotency_key_required`) |
| `404 Not Found` | Source or destination account not found |
| `422 Unprocessable Entity` | Insufficient funds / frozen account / key reused with different body |

**Example**

```bash
curl -X POST http://localhost:8080/v1/transfers \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $(uuidgen | tr '[:upper:]' '[:lower:]')" \
  -d '{
    "source_account_id": "11111111-1111-1111-1111-111111111111",
    "dest_account_id":   "22222222-2222-2222-2222-222222222222",
    "amount":            100000,
    "currency":          "IDR"
  }'
```

**Response**

```json
{
  "transfer_id": "abc12345-...",
  "state": "PENDING",
  "cached": false
}
```

---

## GET /v1/transfers/{id}

Fetches the current state of a transfer.

**Path parameters**

| Param | Description |
|---|---|
| `id` | Transfer UUID |

**Responses**

| Status | Meaning |
|---|---|
| `200 OK` | Transfer found |
| `404 Not Found` | Transfer not found |

**Example**

```bash
curl http://localhost:8080/v1/transfers/abc12345-0000-0000-0000-000000000000
```

**Response**

```json
{
  "id":                "abc12345-...",
  "state":             "COMPLETED",
  "source_account_id": "11111111-...",
  "dest_account_id":   "22222222-...",
  "amount":            100000,
  "currency":          "IDR",
  "external_ref":      "BANK-REF-abc12345-...",
  "failure_reason":    "",
  "created_at":        "2026-01-01T12:00:00Z",
  "updated_at":        "2026-01-01T12:00:05Z"
}
```

**Transfer states**

| State | Description |
|---|---|
| `PENDING` | Accepted, not yet processed |
| `DEBITED` | Source account debited |
| `BANK_CALLED` | Bank API acknowledged |
| `CREDITED` | Destination account credited |
| `COMPLETED` | ✅ Transfer finished |
| `COMPENSATING` | Failure detected, reversing |
| `FAILED` | ❌ Transfer failed, debit reversed |

---

## GET /v1/accounts/{id}/balance

Returns the live account balance computed from ledger entries.
This is never a cached value — it reflects every posted entry.

**Path parameters**

| Param | Description |
|---|---|
| `id` | Account UUID |

**Example**

```bash
curl http://localhost:8080/v1/accounts/11111111-1111-1111-1111-111111111111/balance
```

**Response**

```json
{
  "account_id": "11111111-...",
  "balance":    900000,
  "currency":   "IDR"
}
```

---

## GET /healthz

Returns `200 OK` with body `ok` when the service is healthy.

## GET /metrics

Returns Prometheus metrics in text format.
