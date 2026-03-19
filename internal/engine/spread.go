package engine

import (
	"errors"

	"github.com/shopspring/decimal"
)

// SpreadBounds defines the min/max spread as basis points expressed as decimals.
// e.g. 0.003 = 0.3%, 0.01 = 1.0%
type SpreadBounds struct {
	Min decimal.Decimal
	Max decimal.Decimal
}

// DefaultSpreadBounds enforces the spec: 0.3%–1.0%
var DefaultSpreadBounds = SpreadBounds{
	Min: decimal.NewFromFloat(0.003),
	Max: decimal.NewFromFloat(0.010),
}

// SpreadCalc computes layered bid/ask prices from a mid price and a target spread.
type SpreadCalc struct {
	Bounds SpreadBounds
}

// NewSpreadCalc returns a SpreadCalc with the provided bounds.
func NewSpreadCalc(bounds SpreadBounds) *SpreadCalc {
	return &SpreadCalc{Bounds: bounds}
}

// Prices returns bid and ask prices for a given mid price and target spread fraction.
// targetSpread must satisfy Bounds.Min <= targetSpread <= Bounds.Max.
// Returns ErrInvalidMidPrice if midPrice <= 0.
// Returns ErrSpreadOutOfBounds if targetSpread is outside configured bounds.
func (s *SpreadCalc) Prices(midPrice, targetSpread decimal.Decimal) (bid, ask decimal.Decimal, err error) {
	if midPrice.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero, decimal.Zero, ErrInvalidMidPrice
	}
	if targetSpread.LessThan(s.Bounds.Min) || targetSpread.GreaterThan(s.Bounds.Max) {
		return decimal.Zero, decimal.Zero, ErrSpreadOutOfBounds
	}

	half := targetSpread.Div(decimal.NewFromInt(2))
	bid = midPrice.Mul(decimal.NewFromInt(1).Sub(half))
	ask = midPrice.Mul(decimal.NewFromInt(1).Add(half))
	return bid, ask, nil
}

// EffectiveSpread computes the actual spread fraction between a bid and ask price.
// Returns ErrInvalidMidPrice if ask <= 0.
func EffectiveSpread(bid, ask decimal.Decimal) (decimal.Decimal, error) {
	if ask.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero, ErrInvalidMidPrice
	}
	return ask.Sub(bid).Div(ask.Add(bid).Div(decimal.NewFromInt(2))), nil
}

var (
	ErrInvalidMidPrice  = errors.New("mid price must be greater than zero")
	ErrSpreadOutOfBounds = errors.New("target spread is outside configured bounds")
)