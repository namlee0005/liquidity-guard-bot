package exchange

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

type OrderSide string

const (
	SideBuy  OrderSide = "buy"
	SideSell OrderSide = "sell"
)

type OrderBookLevel struct {
	Price    decimal.Decimal
	Quantity decimal.Decimal
}

type OrderBook struct {
	Symbol    string
	Bids      []OrderBookLevel
	Asks      []OrderBookLevel
	Timestamp time.Time
}

type PlacedOrder struct {
	ExchangeOrderID string
	Symbol          string
	Side            OrderSide
	Price           decimal.Decimal
	Quantity        decimal.Decimal
	Timestamp       time.Time
}

type Balance struct {
	Asset     string
	Available decimal.Decimal
	Locked    decimal.Decimal
}

// ExchangeAdapter is the unified interface every exchange sub-package must satisfy.
// float64 is strictly prohibited; all monetary fields use decimal.Decimal.
type ExchangeAdapter interface {
	Name() string
	OrderBook(ctx context.Context, symbol string, depth int) (*OrderBook, error)
	PlaceLimitOrder(ctx context.Context, symbol string, side OrderSide, price, qty decimal.Decimal) (*PlacedOrder, error)
	CancelOrder(ctx context.Context, symbol, orderID string) error
	Balances(ctx context.Context) ([]Balance, error)
}