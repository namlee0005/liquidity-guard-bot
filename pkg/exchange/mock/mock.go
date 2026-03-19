// Package mock provides a deterministic, in-process ExchangeAdapter for unit tests.
// It never makes network calls. Callers inject price sequences and error scenarios
// via the exported fields before calling WatchOrderBook / PlaceOrder.
package mock

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shopspring/decimal"

	"liquidity-guard-bot/pkg/exchange"
)

const exchangeName = "MOCK"

// Adapter is a fully controllable in-memory exchange adapter.
// All exported fields must be set before the adapter is used in tests.
//
// Thread-safety: PlaceOrder, CancelOrder, and GetBalances are guarded by mu.
// WatchOrderBook uses a separate goroutine per call.
type Adapter struct {
	mu sync.Mutex

	// OrderBookTick is the interval between synthetic order book emissions.
	// Defaults to 100ms if zero.
	OrderBookTick time.Duration

	// MidPrice is the center price for synthetic order book generation.
	// Each tick emits 5 bid levels below and 5 ask levels above this price.
	MidPrice decimal.Decimal

	// SpreadPct is the synthetic spread applied around MidPrice (e.g. "0.005" = 0.5%).
	SpreadPct decimal.Decimal

	// ForceOrderBookErr, if non-nil, is returned immediately from WatchOrderBook.
	ForceOrderBookErr error

	// ForcePlaceOrderErr, if non-nil, is returned from every PlaceOrder call.
	ForcePlaceOrderErr error

	// ForceCancelErr, if non-nil, is returned from every CancelOrder call.
	ForceCancelErr error

	// ForceBalanceErr, if non-nil, is returned from every GetBalances call.
	ForceBalanceErr error

	// Balances is the synthetic balance returned by GetBalances.
	Balances []exchange.Balance

	// orders tracks open orders keyed by ExchangeOrderID.
	orders map[string]exchange.PlaceOrderResult
	// orderSeq is the auto-incrementing order ID counter.
	orderSeq atomic.Int64

	// PlaceOrderCalls records every call to PlaceOrder for assertion in tests.
	PlaceOrderCalls []exchange.PlaceOrderRequest
	// CancelOrderCalls records every (pair, orderID) pair passed to CancelOrder.
	CancelOrderCalls [][2]string
}

// New returns a ready-to-use mock adapter with sensible defaults.
func New() *Adapter {
	return &Adapter{
		MidPrice:      decimal.NewFromInt(100),
		SpreadPct:     decimal.NewFromFloat(0.005),
		OrderBookTick: 100 * time.Millisecond,
		orders:        make(map[string]exchange.PlaceOrderResult),
		Balances: []exchange.Balance{
			{Asset: "BTC", Free: decimal.NewFromInt(1), Locked: decimal.Zero},
			{Asset: "USDT", Free: decimal.NewFromInt(10000), Locked: decimal.Zero},
		},
	}
}

func (a *Adapter) Exchange() string { return exchangeName }

// WatchOrderBook emits synthetic order book snapshots on a ticker until ctx is done.
func (a *Adapter) WatchOrderBook(ctx context.Context, tradingPair string) (<-chan exchange.OrderBook, error) {
	if a.ForceOrderBookErr != nil {
		return nil, a.ForceOrderBookErr
	}

	tick := a.OrderBookTick
	if tick == 0 {
		tick = 100 * time.Millisecond
	}

	ch := make(chan exchange.OrderBook, 10)
	go func() {
		defer close(ch)
		t := time.NewTicker(tick)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-t.C:
				a.mu.Lock()
				mid := a.MidPrice
				spread := a.SpreadPct
				a.mu.Unlock()

				half := mid.Mul(spread).Div(decimal.NewFromInt(2))
				book := exchange.OrderBook{
					TradingPair: tradingPair,
					CapturedAt:  now.UTC(),
				}
				for i := 0; i < 5; i++ {
					offset := half.Mul(decimal.NewFromInt(int64(i + 1)))
					book.Bids = append(book.Bids, exchange.OrderBookLevel{
						Price:    mid.Sub(offset),
						Quantity: decimal.NewFromInt(10),
					})
					book.Asks = append(book.Asks, exchange.OrderBookLevel{
						Price:    mid.Add(offset),
						Quantity: decimal.NewFromInt(10),
					})
				}
				select {
				case ch <- book:
				default: // drop if consumer is slow; real adapters do the same
				}
			}
		}
	}()
	return ch, nil
}

// PlaceOrder records the request, stores the order, and returns a synthetic result.
func (a *Adapter) PlaceOrder(_ context.Context, req exchange.PlaceOrderRequest) (exchange.PlaceOrderResult, error) {
	if a.ForcePlaceOrderErr != nil {
		return exchange.PlaceOrderResult{}, a.ForcePlaceOrderErr
	}

	id := fmt.Sprintf("MOCK-%d", a.orderSeq.Add(1))
	result := exchange.PlaceOrderResult{
		ExchangeOrderID: id,
		ClientOID:       req.ClientOID,
		Status:          "OPEN",
		FilledQty:       decimal.Zero,
		AvgFillPrice:    req.Price,
		Fee:             decimal.Zero,
		FeeAsset:        "USDT",
		Timestamp:       time.Now().UTC(),
	}

	a.mu.Lock()
	a.orders[id] = result
	a.PlaceOrderCalls = append(a.PlaceOrderCalls, req)
	a.mu.Unlock()

	return result, nil
}

// CancelOrder removes the order from the in-memory store (idempotent).
func (a *Adapter) CancelOrder(_ context.Context, tradingPair, exchangeOrderID string) error {
	if a.ForceCancelErr != nil {
		return a.ForceCancelErr
	}
	a.mu.Lock()
	delete(a.orders, exchangeOrderID)
	a.CancelOrderCalls = append(a.CancelOrderCalls, [2]string{tradingPair, exchangeOrderID})
	a.mu.Unlock()
	return nil
}

// GetBalances returns the configured synthetic balances.
func (a *Adapter) GetBalances(_ context.Context) ([]exchange.Balance, error) {
	if a.ForceBalanceErr != nil {
		return nil, a.ForceBalanceErr
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]exchange.Balance, len(a.Balances))
	copy(out, a.Balances)
	return out, nil
}

// OpenOrderCount returns the number of orders currently tracked (test helper).
func (a *Adapter) OpenOrderCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.orders)
}
