package bankmock

import (
	"context"
	"fmt"

	"github.com/bryanwahyu/flip-style-transfer-engine/internal/application/port"
	"github.com/bryanwahyu/flip-style-transfer-engine/internal/infrastructure/circuitbreaker"
)

// CircuitBreakerGateway wraps a BankGateway with circuit breaker protection.
type CircuitBreakerGateway struct {
	inner port.BankGateway
	cb    *circuitbreaker.CircuitBreaker
}

func NewCircuitBreakerGateway(inner port.BankGateway, cb *circuitbreaker.CircuitBreaker) *CircuitBreakerGateway {
	return &CircuitBreakerGateway{inner: inner, cb: cb}
}

func (g *CircuitBreakerGateway) InitiateTransfer(ctx context.Context, req port.BankCallRequest) (port.BankCallResult, error) {
	var result port.BankCallResult
	err := g.cb.Call(func() error {
		var callErr error
		result, callErr = g.inner.InitiateTransfer(ctx, req)
		return callErr
	})
	if err != nil {
		return port.BankCallResult{}, fmt.Errorf("bank gateway: %w", err)
	}
	return result, nil
}

func (g *CircuitBreakerGateway) State() string {
	return g.cb.CurrentState().String()
}
