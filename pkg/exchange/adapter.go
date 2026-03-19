// Package exchange defines the canonical interface all exchange adapters must implement.
// float64 is prohibited — all monetary and quantity values use shopspring/decimal.
package exchange

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

// ─── Domain types ─────────────────────────────────────────────────────────────

// OrderSide is BID or ASK.
type OrderSide string

const (
	SideBid OrderSide = "BID"
	SideAsk OrderSide = "ASK"
)

// OrderType is the execution strategy for a placed order.
type OrderType string

const (
	OrderTypeLimit  OrderType = "LIMIT"
	OrderTypeMarket OrderType = "MARKET"
)

// OrderBookLevel is a single price/quantity entry in the book.
type OrderBookLevel struct {
	Price    decimal.Decimal
	Quantity decimal.Decimal
}

// OrderBook is a point-in-time snapshot of the top-N bid and ask levels.
type OrderBook struct {
	TradingPair string
	Bids        []OrderBookLevel // descending price
	Asks        []OrderBookLevel // ascending price
	CapturedAt  time.Time        // UTC exchange timestamp; never zero
}

// PlaceOrderRequest is the input to PlaceOrder.
type PlaceOrderRequest struct {
	ClientOID   string          // caller-assigned idempotency key; persisted to TradeHistory
	TradingPair string          // e.g. "BTC/USDT"
	Side        OrderSide
	Type        OrderType
	Price       decimal.Decimal // ignored for MARKET orders
	Quantity    decimal.Decimal
}

// PlaceOrderResult is returned by a successful PlaceOrder call.
type PlaceOrderResult struct {
	ExchangeOrderID string
	ClientOID       string
	Status          string          // exchange-native status string
	FilledQty       decimal.Decimal
	AvgFillPrice    decimal.Decimal
	Fee             decimal.Decimal
	FeeAsset        string
	Timestamp       time.Time // UTC
}

// Balance holds free and locked amounts for a single asset.
type Balance struct {
	Asset  string
	Free   decimal.Decimal
	Locked decimal.Decimal
}

// ─── Interface ────────────────────────────────────────────────────────────────

// ExchangeAdapter is the unified 4-method contract every exchange must satisfy.
// Implementations must be safe for concurrent use across goroutines.
//
// Error handling contract:
//   - Transient network/rate-limit errors: return a wrapped *ExchangeError with Retryable=true.
//   - Hard failures (invalid key, bad pair, insufficient funds): Retryable=false.
//   - The caller (BotWorker) inspects Retryable to decide backoff vs. shutdown.
type ExchangeAdapter interface {
	// WatchOrderBook opens a WebSocket subscription for the given trading pair
	// and streams OrderBook snapshots onto the returned channel until ctx is
	// cancelled. The channel is closed when the stream ends. Implementations
	// must reconnect transparently on transient failures.
	WatchOrderBook(ctx context.Context, tradingPair string) (<-chan OrderBook, error)

	// PlaceOrder submits a new order. Returns immediately with the exchange
	// acknowledgement; fill updates arrive via WatchOrderBook or a separate
	// fills feed (exchange-specific). Uses ClientOID for idempotent retries.
	PlaceOrder(ctx context.Context, req PlaceOrderRequest) (PlaceOrderResult, error)

	// CancelOrder cancels an open order by exchange-assigned order ID.
	// Returns nil if the order was already filled or cancelled (idempotent).
	CancelOrder(ctx context.Context, tradingPair, exchangeOrderID string) error

	// GetBalances fetches the current account balances for all held assets.
	// Called on startup and after every fill event to recompute NAV.
	GetBalances(ctx context.Context) ([]Balance, error)

	// Exchange returns the canonical exchange identifier for logging/metrics.
	Exchange() string
}

// ─── Error type ───────────────────────────────────────────────────────────────

// ExchangeError wraps raw exchange API errors with metadata for the bot worker.
type ExchangeError struct {
	Exchange string
	Op       string // e.g. "PlaceOrder", "WatchOrderBook"
	Code     int    // HTTP/exchange error code; 0 if unknown
	Message  string
	Retryable bool
	Cause    error
}

func (e *ExchangeError) Error() string {
	retryTag := "permanent"
	if e.Retryable {
		retryTag = "retryable"
	}
	if e.Cause != nil {
		return "[" + e.Exchange + "/" + e.Op + "] " + e.Message + " (" + retryTag + "): " + e.Cause.Error()
	}
	return "[" + e.Exchange + "/" + e.Op + "] " + e.Message + " (" + retryTag + ")"
}

func (e *ExchangeError) Unwrap() error { return e.Cause }

// NewRetryable constructs a transient ExchangeError.
func NewRetryable(exchange, op, msg string, cause error) *ExchangeError {
	return &ExchangeError{Exchange: exchange, Op: op, Message: msg, Retryable: true, Cause: cause}
}

// NewPermanent constructs a hard-failure ExchangeError.
func NewPermanent(exchange, op, msg string, cause error) *ExchangeError {
	return &ExchangeError{Exchange: exchange, Op: op, Message: msg, Retryable: false, Cause: cause}
}
