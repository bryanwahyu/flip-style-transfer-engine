.PHONY: up down test lint seed demo demo-fail build tidy

API_URL ?= http://localhost:8080/v1

## up: start the full stack (postgres + nats + redis + all 4 services)
up:
	docker compose up --build -d
	@echo "Waiting for API to be ready..."
	@until curl -sf $(API_URL)/.. 2>/dev/null | grep -q ""; do sleep 1; done || \
		until curl -sf http://localhost:8080/healthz | grep -q ok; do sleep 1; done
	@echo "Stack is ready. API at http://localhost:8080"

## down: stop and remove all containers and volumes
down:
	docker compose down -v

## build: compile all binaries locally
build:
	go build ./cmd/api ./cmd/worker ./cmd/outbox-relay ./cmd/reconciler

## tidy: sync go.mod and go.sum
tidy:
	go mod tidy

## test: run unit + integration tests
test:
	go test -race -timeout 120s ./...

## test-unit: run only unit tests (no DB required)
test-unit:
	go test -race -short ./internal/...

## test-integration: run only integration tests (requires docker compose up)
test-integration:
	go test -race -timeout 120s -run Integration ./test/integration/...

## lint: run golangci-lint
lint:
	golangci-lint run ./...

## seed: create two demo accounts in the running stack
seed:
	@echo "Creating sender account..."
	@curl -sf -X POST http://localhost:8080/v1/accounts \
		-H "Content-Type: application/json" \
		-d '{"owner_name":"Alice","currency":"IDR"}' | tee /tmp/alice.json
	@echo "Creating receiver account..."
	@curl -sf -X POST http://localhost:8080/v1/accounts \
		-H "Content-Type: application/json" \
		-d '{"owner_name":"Bob","currency":"IDR"}' | tee /tmp/bob.json

## demo: run a happy-path transfer end to end
demo:
	@echo "=== DEMO: Happy-path transfer ==="
	@ALICE=$$(cat /tmp/alice.json | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])"); \
	BOB=$$(cat /tmp/bob.json | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])"); \
	KEY=$$(uuidgen | tr '[:upper:]' '[:lower:]'); \
	echo "Transfer IDR 10000 from $$ALICE to $$BOB (key=$$KEY)"; \
	curl -s -X POST $(API_URL)/transfers \
		-H "Content-Type: application/json" \
		-H "Idempotency-Key: $$KEY" \
		-d "{\"source_account_id\":\"$$ALICE\",\"dest_account_id\":\"$$BOB\",\"amount\":10000,\"currency\":\"IDR\"}" \
		| python3 -m json.tool; \
	echo "Retrying with same key (should return cached response):"; \
	curl -s -X POST $(API_URL)/transfers \
		-H "Content-Type: application/json" \
		-H "Idempotency-Key: $$KEY" \
		-d "{\"source_account_id\":\"$$ALICE\",\"dest_account_id\":\"$$BOB\",\"amount\":10000,\"currency\":\"IDR\"}" \
		| python3 -m json.tool

## demo-fail: trigger a failure scenario and show compensation
demo-fail:
	@echo "=== DEMO: Transfer with insufficient funds ==="
	@ALICE=$$(cat /tmp/alice.json | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])"); \
	BOB=$$(cat /tmp/bob.json | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])"); \
	KEY=$$(uuidgen | tr '[:upper:]' '[:lower:]'); \
	curl -s -X POST $(API_URL)/transfers \
		-H "Content-Type: application/json" \
		-H "Idempotency-Key: $$KEY" \
		-d "{\"source_account_id\":\"$$ALICE\",\"dest_account_id\":\"$$BOB\",\"amount\":99999999,\"currency\":\"IDR\"}" \
		| python3 -m json.tool
