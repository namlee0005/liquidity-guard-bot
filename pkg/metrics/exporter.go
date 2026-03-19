// Package metrics exposes Prometheus metrics for the Liquidity Guard Bot.
// Register() must be called once at startup before the HTTP server starts.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// All metrics are registered under the "lgb" (LiquidityGuardBot) namespace.
const ns = "lgb"

var (
	// BotState tracks the FSM state per bot (label: bot_id, state).
	// Value is 1 for the active state, 0 for inactive.
	BotStateGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: ns,
		Name:      "bot_state",
		Help:      "Current bot state (1 = active for that state label).",
	}, []string{"bot_id", "state"})

	// SpreadPct tracks the observed bid/ask spread per bot.
	SpreadPct = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: ns,
		Name:      "spread_pct",
		Help:      "Current observed spread as a fraction (0.005 = 0.5%).",
	}, []string{"bot_id", "exchange", "pair"})

	// DrawdownPct24h tracks rolling 24-hour NAV drawdown per bot.
	DrawdownPct24h = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: ns,
		Name:      "drawdown_pct_24h",
		Help:      "Rolling 24-hour NAV drawdown as a fraction.",
	}, []string{"bot_id"})

	// NAV tracks current net asset value in quote currency.
	NAV = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: ns,
		Name:      "nav_quote",
		Help:      "Current NAV in quote currency.",
	}, []string{"bot_id"})

	// OpenOrders tracks the number of live open orders per bot.
	OpenOrders = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: ns,
		Name:      "open_orders",
		Help:      "Number of currently open orders.",
	}, []string{"bot_id", "exchange"})

	// OrdersPlacedTotal counts all PlaceOrder calls by outcome.
	OrdersPlacedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: ns,
		Name:      "orders_placed_total",
		Help:      "Total orders placed, labelled by result (ok / error).",
	}, []string{"bot_id", "exchange", "side", "result"})

	// FillsTotal counts filled orders.
	FillsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: ns,
		Name:      "fills_total",
		Help:      "Total fills received.",
	}, []string{"bot_id", "exchange", "side"})

	// VolumeBase counts total filled base-currency volume.
	VolumeBase = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: ns,
		Name:      "volume_base_total",
		Help:      "Total filled quantity in base currency.",
	}, []string{"bot_id", "exchange", "pair"})

	// WatchdogCycles counts risk watchdog poll cycles.
	WatchdogCycles = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: ns,
		Name:      "watchdog_cycles_total",
		Help:      "Total risk watchdog poll cycles.",
	}, []string{"result"}) // result: ok / error

	// GRPCRequests counts inbound gRPC calls.
	GRPCRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: ns,
		Name:      "grpc_requests_total",
		Help:      "Total gRPC requests by method and status code.",
	}, []string{"method", "code"})

	// TelemetrySubscribers tracks live StreamTelemetry subscriber count.
	TelemetrySubscribers = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: ns,
		Name:      "telemetry_subscribers",
		Help:      "Number of active gRPC StreamTelemetry subscribers.",
	})
)

// Handler returns an http.Handler that serves /metrics for Prometheus scraping.
func Handler() http.Handler {
	return promhttp.Handler()
}

// SetBotState updates the BotStateGauge to reflect a state transition.
// It zeroes all other state labels for the bot to avoid stale series.
func SetBotState(botID, newState string) {
	states := []string{"RUNNING", "SLOW", "PAUSED", "STOPPED"}
	for _, s := range states {
		v := 0.0
		if s == newState {
			v = 1.0
		}
		BotStateGauge.WithLabelValues(botID, s).Set(v)
	}
}
