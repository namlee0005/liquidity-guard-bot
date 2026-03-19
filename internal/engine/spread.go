// Package engine contains the core trading logic: spread calculation and order management.
package engine

import (
	"fmt"

	"github.com/shopspring/decimal"

	"github.com/namlee0005/liquidity-guard-bot/pkg/exchange"
)

// SpreadBounds defines the allowed spread range for a trading pair.
// MinPct and MaxPct are fractional (0.003 = 0.3%, 0.01 = 1.0%).
type SpreadBounds struct {
	MinPct decimal.Decimal
	MaxPct decimal.Decimal
}

// LayerQuote is a single bid/ask price+size for one depth layer.
type LayerQuote struct {
	Layer    int
	BidPrice decimal.Decimal
	AskPrice decimal.Decimal
	Size     decimal.Decimal // base currency quantity per side
}

// SpreadCalculator computes layered bid/ask quotes from an order book snapshot.
// It enforces that the resulting spread stays within the configured bounds.
type SpreadCalculator struct {
	bounds      SpreadBounds
	orderLayers int
	layerSize   decimal.Decimal // base currency per layer
}

// NewSpreadCalculator creates a SpreadCalculator. Returns an error if bounds are invalid.
func NewSpreadCalculator(bounds SpreadBounds, orderLayers int, layerSize decimal.Decimal) (*SpreadCalculator, error) {
	if bounds.MinPct.LessThanOrEqual(decimal.Zero) {
		return nil, fmt.Errorf("SpreadCalculator: MinPct must be > 0, got %s", bounds.MinPct)
	}
	if bounds.MaxPct.LessThan(bounds.MinPct) {
		return nil, fmt.Errorf("SpreadCalculator: MaxPct (%s) < MinPct (%s)", bounds.MaxPct, bounds.MinPct)
	}
	if orderLayers < 1 {
		return nil, fmt.Errorf("SpreadCalculator: orderLayers must be >= 1, got %d", orderLayers)
	}
	if layerSize.LessThanOrEqual(decimal.Zero) {
		return nil, fmt.Errorf("SpreadCalculator: layerSize must be > 0, got %s", layerSize)
	}
	return &SpreadCalculator{
		bounds:      bounds,
		orderLayers: orderLayers,
		layerSize:   layerSize,
	}, nil
}

// two is a decimal constant used for midpoint calculation.
var two = decimal.NewFromInt(2)

// Compute derives layered quotes from the given order book.
// It derives the mid-price from the best bid/ask, clamps the spread to bounds,
// then fans out N layers with geometrically increasing offsets.
//
// Returns an error if the book has no usable levels (e.g. empty/stale snapshot).
func (sc *SpreadCalculator) Compute(book exchange.OrderBook) ([]LayerQuote, error) {
	if len(book.Bids) == 0 || len(book.Asks) == 0 {
		return nil, fmt.Errorf("SpreadCalculator.Compute: empty order book for %s", book.TradingPair)
	}

	bestBid := book.Bids[0].Price
	bestAsk := book.Asks[0].Price
	if bestAsk.LessThanOrEqual(bestBid) {
		return nil, fmt.Errorf("SpreadCalculator.Compute: crossed book (bid %s >= ask %s)", bestBid, bestAsk)
	}

	mid := bestBid.Add(bestAsk).Div(two)

	// Raw spread from the book; clamp to [MinPct, MaxPct].
	rawSpreadPct := bestAsk.Sub(bestBid).Div(mid)
	spreadPct := rawSpreadPct
	if spreadPct.LessThan(sc.bounds.MinPct) {
		spreadPct = sc.bounds.MinPct
	} else if spreadPct.GreaterThan(sc.bounds.MaxPct) {
		spreadPct = sc.bounds.MaxPct
	}

	halfSpread := mid.Mul(spreadPct).Div(two)

	quotes := make([]LayerQuote, sc.orderLayers)
	for i := 0; i < sc.orderLayers; i++ {
		// Each outer layer widens by one additional half-spread increment.
		layerFactor := decimal.NewFromInt(int64(i + 1))
		offset := halfSpread.Mul(layerFactor)
		quotes[i] = LayerQuote{
			Layer:    i + 1,
			BidPrice: mid.Sub(offset),
			AskPrice: mid.Add(offset),
			Size:     sc.layerSize,
		}
	}
	return quotes, nil
}

// CurrentSpreadPct computes the observed spread percentage from a live book.
// Used by the DepthMonitor to check spread adherence in metrics.
func CurrentSpreadPct(book exchange.OrderBook) (decimal.Decimal, error) {
	if len(book.Bids) == 0 || len(book.Asks) == 0 {
		return decimal.Zero, fmt.Errorf("empty book")
	}
	mid := book.Bids[0].Price.Add(book.Asks[0].Price).Div(two)
	if mid.IsZero() {
		return decimal.Zero, fmt.Errorf("zero midpoint")
	}
	return book.Asks[0].Price.Sub(book.Bids[0].Price).Div(mid), nil
}
