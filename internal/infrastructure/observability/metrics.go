package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	TransfersTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "transfers_total",
		Help: "Total number of transfers by state",
	}, []string{"state"})

	TransferDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "transfer_duration_seconds",
		Help:    "Time from transfer creation to completion or failure",
		Buckets: prometheus.DefBuckets,
	}, []string{"outcome"})

	BankCallsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "bank_calls_total",
		Help: "Total bank API calls by outcome",
	}, []string{"outcome"})

	CircuitBreakerState = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "circuit_breaker_state",
		Help: "Current circuit breaker state (0=closed, 1=open, 2=half_open)",
	}, []string{"gateway"})

	OutboxPendingTotal = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "outbox_pending_total",
		Help: "Number of outbox events pending publication",
	})

	LedgerEntriesTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "ledger_entries_total",
		Help: "Total number of ledger entries posted",
	})
)
