package bankmock

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/bryanwahyu/flip-style-transfer-engine/internal/application/port"
)

// FailureMode configures how the mock bank responds.
type FailureMode int

const (
	ModeSuccess      FailureMode = iota
	ModeTimeout
	ModePermanentErr
	ModeFlaky // fails for the first N calls, then succeeds
)

// Gateway is a fake bank implementation for tests. Thread-safe.
type Gateway struct {
	mode       FailureMode
	flakyLimit int32
	callCount  atomic.Int32
	latency    time.Duration
}

func New(mode FailureMode) *Gateway {
	return &Gateway{mode: mode, latency: 50 * time.Millisecond}
}

func NewFlaky(failFirstN int) *Gateway {
	return &Gateway{mode: ModeFlaky, flakyLimit: int32(failFirstN), latency: 50 * time.Millisecond}
}

func (g *Gateway) InitiateTransfer(ctx context.Context, req port.BankCallRequest) (port.BankCallResult, error) {
	n := g.callCount.Add(1)

	select {
	case <-time.After(g.latency):
	case <-ctx.Done():
		return port.BankCallResult{}, ctx.Err()
	}

	switch g.mode {
	case ModeSuccess:
		return port.BankCallResult{ExternalRef: fmt.Sprintf("BANK-REF-%s", req.TransferID.String())}, nil
	case ModeTimeout:
		<-ctx.Done()
		return port.BankCallResult{}, fmt.Errorf("bank timeout: %w", ctx.Err())
	case ModePermanentErr:
		return port.BankCallResult{}, errors.New("bank returned HTTP 500")
	case ModeFlaky:
		if n <= g.flakyLimit {
			return port.BankCallResult{}, fmt.Errorf("bank flaky error (call %d/%d)", n, g.flakyLimit)
		}
		return port.BankCallResult{ExternalRef: fmt.Sprintf("BANK-REF-%s", req.TransferID.String())}, nil
	}
	return port.BankCallResult{}, errors.New("unknown failure mode")
}

func (g *Gateway) CallCount() int { return int(g.callCount.Load()) }
