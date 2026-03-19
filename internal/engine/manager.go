package engine

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shopspring/decimal"

	"liquidity-guard-bot/pkg/exchange"
)

// ManagedOrder tracks a live order placed by the OrderManager.
type ManagedOrder struct {
	ExchangeOrderID string
	ClientOID       string
	Layer           int
	Side            exchange.OrderSide
	Price           decimal.Decimal
	Quantity        decimal.Decimal
	PlacedAt        time.Time
}

// OrderManager maintains the set of live bid/ask orders for a single bot worker.
// It reconciles desired quotes (from SpreadCalculator) against actual open orders,
// cancelling stale orders and placing new ones as needed.
//
// All methods are safe for single-goroutine use; the worker owns this struct exclusively.
type OrderManager struct {
	adapter   exchange.ExchangeAdapter
	pair      string
	botID     string
	seqGen    func() string // returns unique ClientOID; injected for testability

	mu         sync.Mutex
	openOrders map[string]*ManagedOrder // key: ExchangeOrderID
}

// NewOrderManager constructs an OrderManager. seqGen must return globally unique strings.
func NewOrderManager(adapter exchange.ExchangeAdapter, botID, pair string, seqGen func() string) *OrderManager {
	return &OrderManager{
		adapter:    adapter,
		pair:       pair,
		botID:      botID,
		seqGen:     seqGen,
		openOrders: make(map[string]*ManagedOrder),
	}
}

// Reconcile is the core cycle method. Given a desired set of LayerQuotes, it:
//  1. Cancels all existing open orders (full refresh strategy — simplest for Phase 4).
//  2. Places new bid and ask orders for each layer.
//
// Full-refresh is intentional: spread recalculation happens every tick, and
// partial reconciliation logic adds complexity without measurable latency gain
// at low-volume targets. Upgrade to diff-based reconciliation in a later phase.
func (m *OrderManager) Reconcile(ctx context.Context, quotes []LayerQuote) error {
	if err := m.cancelAll(ctx); err != nil {
		return fmt.Errorf("OrderManager.Reconcile cancel: %w", err)
	}

	m.mu.Lock()
	m.openOrders = make(map[string]*ManagedOrder, len(quotes)*2)
	m.mu.Unlock()

	for _, q := range quotes {
		for _, side := range []exchange.OrderSide{exchange.SideBuy, exchange.SideSell} {
			price := q.BidPrice
			if side == exchange.SideSell {
				price = q.AskPrice
			}

			result, err := m.adapter.PlaceLimitOrder(ctx, m.pair, side, price, q.Size)
			if err != nil {
				return fmt.Errorf("OrderManager.Reconcile place layer %d %s: %w", q.Layer, side, err)
			}

			m.mu.Lock()
			m.openOrders[result.ExchangeOrderID] = &ManagedOrder{
				ExchangeOrderID: result.ExchangeOrderID,
				ClientOID:       m.seqGen(),
				Layer:           q.Layer,
				Side:            side,
				Price:           price,
				Quantity:        q.Size,
				PlacedAt:        time.Now().UTC(),
			}
			m.mu.Unlock()
		}
	}
	return nil
}

// CancelAll cancels every tracked open order. Called on PAUSE and shutdown.
func (m *OrderManager) CancelAll(ctx context.Context) error {
	return m.cancelAll(ctx)
}

func (m *OrderManager) cancelAll(ctx context.Context) error {
	m.mu.Lock()
	ids := make([]string, 0, len(m.openOrders))
	for id := range m.openOrders {
		ids = append(ids, id)
	}
	m.mu.Unlock()

	var firstErr error
	for _, id := range ids {
		if err := m.adapter.CancelOrder(ctx, m.pair, id); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			// continue cancelling remaining orders even on partial failure
		}
	}
	return firstErr
}

// OpenOrderCount returns the current number of tracked open orders.
func (m *OrderManager) OpenOrderCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.openOrders)
}
