package engine_test

// FAST (<1s) — pure math, no I/O

import (
	"testing"

	"github.com/shopspring/decimal"
)

// re-import the package under test via its module path.
// Adjust the module path to match go.mod if different.
// import "github.com/your-org/liquidity-guard/internal/engine"

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

func mustDecimal(s string) decimal.Decimal {
	d, err := decimal.NewFromString(s)
	if err != nil {
		panic(err)
	}
	return d
}

// ---------------------------------------------------------------------------
// SpreadCalc.Prices — happy path
// ---------------------------------------------------------------------------

func TestSpreadCalc_Prices_HalfSpreadSymmetry(t *testing.T) {
	// FAST (<1s)
	// Ask − mid == mid − bid  (symmetric around mid price)
	calc := NewSpreadCalc(DefaultSpreadBounds)
	mid := mustDecimal("100")
	spread := mustDecimal("0.005") // 0.5%

	bid, ask, err := calc.Prices(mid, spread)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	bidDelta := mid.Sub(bid)
	askDelta := ask.Sub(mid)
	if !bidDelta.Equal(askDelta) {
		t.Errorf("spread is not symmetric: bidDelta=%s askDelta=%s", bidDelta, askDelta)
	}
}

func TestSpreadCalc_Prices_BidLessThanAsk(t *testing.T) {
	// FAST (<1s)
	calc := NewSpreadCalc(DefaultSpreadBounds)
	bid, ask, err := calc.Prices(mustDecimal("50000"), mustDecimal("0.003"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bid.LessThan(ask) {
		t.Errorf("bid %s must be < ask %s", bid, ask)
	}
}

func TestSpreadCalc_Prices_MinBoundAccepted(t *testing.T) {
	// FAST (<1s)
	calc := NewSpreadCalc(DefaultSpreadBounds)
	_, _, err := calc.Prices(mustDecimal("1"), DefaultSpreadBounds.Min)
	if err != nil {
		t.Errorf("min bound spread should be accepted, got: %v", err)
	}
}

func TestSpreadCalc_Prices_MaxBoundAccepted(t *testing.T) {
	// FAST (<1s)
	calc := NewSpreadCalc(DefaultSpreadBounds)
	_, _, err := calc.Prices(mustDecimal("1"), DefaultSpreadBounds.Max)
	if err != nil {
		t.Errorf("max bound spread should be accepted, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// SpreadCalc.Prices — error paths
// ---------------------------------------------------------------------------

func TestSpreadCalc_Prices_ZeroMidPriceReturnsError(t *testing.T) {
	// FAST (<1s)
	calc := NewSpreadCalc(DefaultSpreadBounds)
	_, _, err := calc.Prices(decimal.Zero, mustDecimal("0.005"))
	if err != ErrInvalidMidPrice {
		t.Errorf("expected ErrInvalidMidPrice, got: %v", err)
	}
}

func TestSpreadCalc_Prices_NegativeMidPriceReturnsError(t *testing.T) {
	// FAST (<1s)
	calc := NewSpreadCalc(DefaultSpreadBounds)
	_, _, err := calc.Prices(mustDecimal("-1"), mustDecimal("0.005"))
	if err != ErrInvalidMidPrice {
		t.Errorf("expected ErrInvalidMidPrice, got: %v", err)
	}
}

func TestSpreadCalc_Prices_SpreadBelowMinReturnsError(t *testing.T) {
	// FAST (<1s)
	calc := NewSpreadCalc(DefaultSpreadBounds)
	_, _, err := calc.Prices(mustDecimal("100"), mustDecimal("0.001"))
	if err != ErrSpreadOutOfBounds {
		t.Errorf("expected ErrSpreadOutOfBounds, got: %v", err)
	}
}

func TestSpreadCalc_Prices_SpreadAboveMaxReturnsError(t *testing.T) {
	// FAST (<1s)
	calc := NewSpreadCalc(DefaultSpreadBounds)
	_, _, err := calc.Prices(mustDecimal("100"), mustDecimal("0.02"))
	if err != ErrSpreadOutOfBounds {
		t.Errorf("expected ErrSpreadOutOfBounds, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// SpreadCalc.Prices — precision (no float64 drift)
// ---------------------------------------------------------------------------

func TestSpreadCalc_Prices_DecimalPrecisionNoDrift(t *testing.T) {
	// FAST (<1s)
	// Verify that the result is exact to 10 decimal places (shopspring guarantee).
	calc := NewSpreadCalc(DefaultSpreadBounds)
	bid, ask, _ := calc.Prices(mustDecimal("0.00000001"), mustDecimal("0.005"))

	// ask = 0.00000001 * 1.0025 = 0.0000000100250000000...
	expected := mustDecimal("0.0000000100250000000")
	if !ask.Equal(expected) {
		t.Errorf("precision failure: want %s got %s", expected, ask)
	}
	_ = bid
}

// ---------------------------------------------------------------------------
// EffectiveSpread
// ---------------------------------------------------------------------------

func TestEffectiveSpread_RoundTrip(t *testing.T) {
	// FAST (<1s)
	// Generate bid/ask from a known spread, then verify EffectiveSpread recovers it.
	calc := NewSpreadCalc(DefaultSpreadBounds)
	target := mustDecimal("0.006")
	bid, ask, _ := calc.Prices(mustDecimal("200"), target)

	got, err := EffectiveSpread(bid, ask)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Allow ±1e-10 tolerance for decimal rounding
	diff := got.Sub(target).Abs()
	if diff.GreaterThan(mustDecimal("0.0000000001")) {
		t.Errorf("round-trip mismatch: target=%s got=%s diff=%s", target, got, diff)
	}
}

func TestEffectiveSpread_ZeroAskReturnsError(t *testing.T) {
	// FAST (<1s)
	_, err := EffectiveSpread(decimal.Zero, decimal.Zero)
	if err != ErrInvalidMidPrice {
		t.Errorf("expected ErrInvalidMidPrice, got: %v", err)
	}
}