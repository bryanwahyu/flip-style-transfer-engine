# syntax=docker/dockerfile:1
FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /bin/api        ./cmd/api
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /bin/worker     ./cmd/worker
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /bin/outbox-relay ./cmd/outbox-relay
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /bin/reconciler ./cmd/reconciler

# ── api ──────────────────────────────────────────────────────────────────────
FROM gcr.io/distroless/static:nonroot AS api
COPY --from=builder /bin/api /api
EXPOSE 8080
ENTRYPOINT ["/api"]

# ── worker ───────────────────────────────────────────────────────────────────
FROM gcr.io/distroless/static:nonroot AS worker
COPY --from=builder /bin/worker /worker
ENTRYPOINT ["/worker"]

# ── outbox-relay ─────────────────────────────────────────────────────────────
FROM gcr.io/distroless/static:nonroot AS outbox-relay
COPY --from=builder /bin/outbox-relay /outbox-relay
ENTRYPOINT ["/outbox-relay"]

# ── reconciler ───────────────────────────────────────────────────────────────
FROM gcr.io/distroless/static:nonroot AS reconciler
COPY --from=builder /bin/reconciler /reconciler
ENTRYPOINT ["/reconciler"]
