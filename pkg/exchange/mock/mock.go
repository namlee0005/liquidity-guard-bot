// Package mock provides a deterministic, in-process ExchangeAdapter for unit tests.
// It never makes network calls. Inject error scenarios via exported fields before use.
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

// placedCall is a record of a PlaceLimitOrder invocation for test assertions.
type placedCall struct {
	Symbol string
	Side   exchange.OrderSide
	Price  decimal.Decimal
	Qty    decimal.Decimal
}

// Adapter is a fully controllable in-memory exchange adapter.
// Thread-safe: all state mutations are guarded by mu.
type Adapter struct {
	mu sync.Mutex

	// MidPrice is the centre price for synthetic order book generation.
	MidPrice decimal.Decimal
	// SpreadPct is applied around MidPrice (e.g. "0.005" = 0.5 %).
	SpreadPct decimal.Decimal

	// ForceOrderBookErr, if set, is returned from every OrderBook call.
	ForceOrderBookErr error
	// ForcePlaceErr, if set, is returned from every PlaceLimitOrder call.
	ForcePlaceErr error
	// ForceCancelErr, if set, is returned from every CancelOrder call.
	ForceCancelErr error
	// ForceBalancesErr, if set, is returned from every Balances call.
	ForceBalancesErr error

	// BalanceList is the synthetic list returned by Balances.
	BalanceList []exchange.Balance

	orders   map[string]*exchange.PlacedOrder
	orderSeq atomic.Int64

	// PlaceCalls records every PlaceLimitOrder invocation for assertions.
	PlaceCalls []placedCall
	// CancelCalls records every (symbol, orderID) pair passed to CancelOrder.
	CancelCalls [][2]string
}

// New returns a ready-to-use mock adapter.
func New() *Adapter {
	return &Adapter{
		MidPrice:  decimal.NewFromInt(100),
		SpreadPct: decimal.NewFromFloat(0.005),
		orders:    make(map[string]*exchange.PlacedOrder),
		BalanceList: []exchange.Balance{
			{Asset: "BTC", Available: decimal.NewFromInt(1), Locked: decimal.Zero},
			{Asset: "USDT", Available: decimal.NewFromInt(10000), Locked: decimal.Zero},
		},
	}
}

// Compile-time interface check.
var _ exchange.ExchangeAdapter = (*Adapter)(nil)

func (a *Adapter) Name() string { return "mock" }

// OrderBook returns a synthetic snapshot around MidPrice.
func (a *Adapter) OrderBook(_ context.Context, symbol string, depth int) (*exchange.OrderBook, error) {
	if a.ForceOrderBookErr != nil {
		return nil, a.ForceOrderBookErr
	}
	a.mu.Lock()
	mid, spread := a.MidPrice, a.SpreadPct
	a.mu.Unlock()

	if depth <= 0 {
		depth = 5
	}
	half := mid.Mul(spread).Div(decimal.NewFromInt(2))
	ob := &exchange.OrderBook{Symbol: symbol, Timestamp: time.Now().UTC()}
	for i := 0; i < depth; i++ {
		off := half.Mul(decimal.NewFromInt(int64(i + 1)))
		ob.Bids = append(ob.Bids, exchange.OrderBookLevel{Price: mid.Sub(off), Quantity: decimal.NewFromInt(10)})
		ob.Asks = append(ob.Asks, exchange.OrderBookLevel{Price: mid.Add(off), Quantity: decimal.NewFromInt(10)})
	}
	return ob, nil
}

// PlaceLimitOrder records the call and returns a synthetic PlacedOrder.
func (a *Adapter) PlaceLimitOrder(_ context.Context, symbol string, side exchange.OrderSide, price, qty decimal.Decimal) (*exchange.PlacedOrder, error) {
	if a.ForcePlaceErr != nil {
		return nil, a.ForcePlaceErr
	}
	id := fmt.Sprintf("MOCK-%d", a.orderSeq.Add(1))
	order := &exchange.PlacedOrder{
		ExchangeOrderID: id,
		Symbol:          symbol,
		Side:            side,
		Price:           price,
		Quantity:        qty,
		Timestamp:       time.Now().UTC(),
	}
	a.mu.Lock()
	a.orders[id] = order
	a.PlaceCalls = append(a.PlaceCalls, placedCall{Symbol: symbol, Side: side, Price: price, Qty: qty})
	a.mu.Unlock()
	return order, nil
}

// CancelOrder removes the order (idempotent).
func (a *Adapter) CancelOrder(_ context.Context, symbol, orderID string) error {
	if a.ForceCancelErr != nil {
		return a.ForceCancelErr
	}
	a.mu.Lock()
	delete(a.orders, orderID)
	a.CancelCalls = append(a.CancelCalls, [2]string{symbol, orderID})
	a.mu.Unlock()
	return nil
}

// Balances returns the configured synthetic balances.
func (a *Adapter) Balances(_ context.Context) ([]exchange.Balance, error) {
	if a.ForceBalancesErr != nil {
		return nil, a.ForceBalancesErr
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]exchange.Balance, len(a.BalanceList))
	copy(out, a.BalanceList)
	return out, nil
}

// OpenOrderCount is a test helper returning the count of tracked open orders.
func (a *Adapter) OpenOrderCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.orders)
}
