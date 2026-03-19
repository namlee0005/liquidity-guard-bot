package exchange_test

// All tests: FAST (<1s) — httptest server, zero real credentials, zero network.
// Run: go test -race ./pkg/exchange/...

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"liquidity-guard-bot/pkg/exchange"
	"liquidity-guard-bot/pkg/exchange/bybit"
	"liquidity-guard-bot/pkg/exchange/gate"
	"liquidity-guard-bot/pkg/exchange/kraken"
	"liquidity-guard-bot/pkg/exchange/mexc"
)

// ---------------------------------------------------------------------------
// Compile-time interface compliance
// ---------------------------------------------------------------------------

var _ exchange.ExchangeAdapter = (*mexc.Adapter)(nil)
var _ exchange.ExchangeAdapter = (*bybit.Adapter)(nil)
var _ exchange.ExchangeAdapter = (*gate.Adapter)(nil)
var _ exchange.ExchangeAdapter = (*kraken.Adapter)(nil)

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

func newCtx() context.Context {
	ctx, _ := context.WithTimeout(context.Background(), 3*time.Second)
	return ctx
}

func mustDecimal(s string) decimal.Decimal {
	d, _ := decimal.NewFromString(s)
	return d
}

// ---------------------------------------------------------------------------
// Fixture JSON responses
// ---------------------------------------------------------------------------

const mexcDepthJSON = `{"bids":[["29000.50","1.200"],["28999.00","0.500"]],"asks":[["29001.00","0.800"],["29002.50","2.100"]]}`
const bybitDepthJSON = `{"retCode":0,"retMsg":"OK","result":{"b":[["29000.50","1.200"],["28999.00","0.500"]],"a":[["29001.00","0.800"],["29002.50","2.100"]]}}`
const gateDepthJSON = `{"bids":[["29000.50","1.200"],["28999.00","0.500"]],"asks":[["29001.00","0.800"],["29002.50","2.100"]]}`

func krakenDepthJSON(pair string) string {
	return `{"error":[],"result":{"` + pair + `":{"bids":[["29000.50","1.200",1700000000],["28999.00","0.500",1700000001]],"asks":[["29001.00","0.800",1700000002],["29002.50","2.100",1700000003]]}}}`
}

// adapterCase parameterises the cross-adapter contract suite.
type adapterCase struct {
	name     string
	factory  func(baseURL string) exchange.ExchangeAdapter
	bookJSON func() string
	symbol   string
}

func allAdapters() []adapterCase {
	return []adapterCase{
		{
			name:     "mexc",
			factory:  func(u string) exchange.ExchangeAdapter { return mexc.New("k", "s", u) },
			bookJSON: func() string { return mexcDepthJSON },
			symbol:   "BTCUSDT",
		},
		{
			name:     "bybit",
			factory:  func(u string) exchange.ExchangeAdapter { return bybit.New("k", "s", u) },
			bookJSON: func() string { return bybitDepthJSON },
			symbol:   "BTCUSDT",
		},
		{
			name:     "gate",
			factory:  func(u string) exchange.ExchangeAdapter { return gate.New("k", "s", u) },
			bookJSON: func() string { return gateDepthJSON },
			symbol:   "BTC_USDT",
		},
		{
			name:     "kraken",
			factory:  func(u string) exchange.ExchangeAdapter { return kraken.New("k", "aGVsbG8=", u) },
			bookJSON: func() string { return krakenDepthJSON("XBTUSDT") },
			symbol:   "XBTUSDT",
		},
	}
}

// ---------------------------------------------------------------------------
// MEXC — unit tests
// ---------------------------------------------------------------------------

func TestMEXCAdapter_Name(t *testing.T) {
	// FAST (<1s)
	a := mexc.New("k", "s", "http://localhost")
	if a.Name() != "mexc" {
		t.Errorf("expected 'mexc', got %q", a.Name())
	}
}

func TestMEXCAdapter_OrderBook_BidCountMatchesFixture(t *testing.T) {
	// FAST (<1s)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(mexcDepthJSON))
	}))
	defer srv.Close()

	ob, err := mexc.New("k", "s", srv.URL).OrderBook(newCtx(), "BTCUSDT", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ob.Bids) != 2 {
		t.Errorf("expected 2 bids, got %d", len(ob.Bids))
	}
}

func TestMEXCAdapter_OrderBook_AskCountMatchesFixture(t *testing.T) {
	// FAST (<1s)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(mexcDepthJSON))
	}))
	defer srv.Close()

	ob, _ := mexc.New("k", "s", srv.URL).OrderBook(newCtx(), "BTCUSDT", 5)
	if len(ob.Asks) != 2 {
		t.Errorf("expected 2 asks, got %d", len(ob.Asks))
	}
}

func TestMEXCAdapter_OrderBook_BidPriceIsDecimalNotFloat(t *testing.T) {
	// FAST (<1s) — verifies no float64 precision loss in price parsing
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(mexcDepthJSON))
	}))
	defer srv.Close()

	ob, _ := mexc.New("k", "s", srv.URL).OrderBook(newCtx(), "BTCUSDT", 5)
	expected := mustDecimal("29000.50")
	if !ob.Bids[0].Price.Equal(expected) {
		t.Errorf("precision loss: want %s got %s", expected, ob.Bids[0].Price)
	}
}

func TestMEXCAdapter_OrderBook_BestBidLessThanBestAsk(t *testing.T) {
	// FAST (<1s)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(mexcDepthJSON))
	}))
	defer srv.Close()

	ob, _ := mexc.New("k", "s", srv.URL).OrderBook(newCtx(), "BTCUSDT", 5)
	if !ob.Bids[0].Price.LessThan(ob.Asks[0].Price) {
		t.Errorf("inverted book: bid %s >= ask %s", ob.Bids[0].Price, ob.Asks[0].Price)
	}
}

func TestMEXCAdapter_OrderBook_HTTPErrorReturnsError(t *testing.T) {
	// FAST (<1s)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	_, err := mexc.New("k", "s", srv.URL).OrderBook(newCtx(), "BTCUSDT", 5)
	if err == nil {
		t.Error("expected error on HTTP 429, got nil")
	}
}

func TestMEXCAdapter_OrderBook_EmptyBooksAreValid(t *testing.T) {
	// FAST (<1s)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"bids":[],"asks":[]}`))
	}))
	defer srv.Close()

	ob, err := mexc.New("k", "s", srv.URL).OrderBook(newCtx(), "BTCUSDT", 5)
	if err != nil {
		t.Fatalf("empty order book must not error: %v", err)
	}
	if len(ob.Bids) != 0 {
		t.Errorf("expected 0 bids, got %d", len(ob.Bids))
	}
}

func TestMEXCAdapter_PlaceLimitOrder_ReturnsOrderID(t *testing.T) {
	// FAST (<1s)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"orderId":"MEXC-123456"}`))
	}))
	defer srv.Close()

	order, err := mexc.New("k", "s", srv.URL).PlaceLimitOrder(
		newCtx(), "BTCUSDT", exchange.SideBuy, mustDecimal("29000.50"), mustDecimal("0.001"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if order.ExchangeOrderID != "MEXC-123456" {
		t.Errorf("expected MEXC-123456, got %q", order.ExchangeOrderID)
	}
}

func TestMEXCAdapter_PlaceLimitOrder_RequestContainsAPIKeyHeader(t *testing.T) {
	// FAST (<1s)
	var capturedKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedKey = r.Header.Get("X-MEXC-APIKEY")
		w.Write([]byte(`{"orderId":"x"}`))
	}))
	defer srv.Close()

	mexc.New("my-api-key", "s", srv.URL).PlaceLimitOrder(
		newCtx(), "BTCUSDT", exchange.SideBuy, mustDecimal("100"), mustDecimal("1"))
	if capturedKey != "my-api-key" {
		t.Errorf("expected X-MEXC-APIKEY='my-api-key', got %q", capturedKey)
	}
}

func TestMEXCAdapter_PlaceLimitOrder_RequestContainsSignatureParam(t *testing.T) {
	// FAST (<1s)
	var hasSignature bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hasSignature = r.URL.Query().Get("signature") != ""
		w.Write([]byte(`{"orderId":"x"}`))
	}))
	defer srv.Close()

	mexc.New("k", "secret", srv.URL).PlaceLimitOrder(
		newCtx(), "BTCUSDT", exchange.SideBuy, mustDecimal("100"), mustDecimal("1"))
	if !hasSignature {
		t.Error("signed request must include 'signature' query parameter")
	}
}

func TestMEXCAdapter_CancelOrder_SuccessReturnsNilError(t *testing.T) {
	// FAST (<1s)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	err := mexc.New("k", "s", srv.URL).CancelOrder(newCtx(), "BTCUSDT", "ORDER-99")
	if err != nil {
		t.Errorf("expected nil on cancel success, got: %v", err)
	}
}

func TestMEXCAdapter_Balances_FiltersZeroBalances(t *testing.T) {
	// FAST (<1s)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"balances":[{"asset":"BTC","free":"0.5","locked":"0.1"},{"asset":"ETH","free":"0","locked":"0"}]}`))
	}))
	defer srv.Close()

	bals, err := mexc.New("k", "s", srv.URL).Balances(newCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bals) != 1 {
		t.Errorf("expected 1 non-zero balance, got %d", len(bals))
	}
}

// ---------------------------------------------------------------------------
// Bybit — unit tests
// ---------------------------------------------------------------------------

func TestBybitAdapter_Name(t *testing.T) {
	// FAST (<1s)
	a := bybit.New("k", "s", "http://localhost")
	if a.Name() != "bybit" {
		t.Errorf("expected 'bybit', got %q", a.Name())
	}
}

func TestBybitAdapter_OrderBook_BidCountMatchesFixture(t *testing.T) {
	// FAST (<1s)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(bybitDepthJSON))
	}))
	defer srv.Close()

	ob, err := bybit.New("k", "s", srv.URL).OrderBook(newCtx(), "BTCUSDT", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ob.Bids) != 2 {
		t.Errorf("expected 2 bids, got %d", len(ob.Bids))
	}
}

func TestBybitAdapter_OrderBook_NonZeroRetCodeReturnsError(t *testing.T) {
	// FAST (<1s)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"retCode":10001,"retMsg":"Invalid symbol","result":{}}`))
	}))
	defer srv.Close()

	_, err := bybit.New("k", "s", srv.URL).OrderBook(newCtx(), "INVALID", 5)
	if err == nil {
		t.Error("expected error on non-zero retCode, got nil")
	}
}

func TestBybitAdapter_OrderBook_AskPriceDecimalPrecision(t *testing.T) {
	// FAST (<1s)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(bybitDepthJSON))
	}))
	defer srv.Close()

	ob, _ := bybit.New("k", "s", srv.URL).OrderBook(newCtx(), "BTCUSDT", 5)
	expected := mustDecimal("29001.00")
	if !ob.Asks[0].Price.Equal(expected) {
		t.Errorf("want %s got %s", expected, ob.Asks[0].Price)
	}
}

func TestBybitAdapter_PlaceLimitOrder_RequestContainsAPIKeyHeader(t *testing.T) {
	// FAST (<1s)
	var capturedKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedKey = r.Header.Get("X-BAPI-API-KEY")
		w.Write([]byte(`{"retCode":0,"result":{"orderId":"BYB-001"}}`))
	}))
	defer srv.Close()

	bybit.New("bybit-key", "s", srv.URL).PlaceLimitOrder(
		newCtx(), "BTCUSDT", exchange.SideBuy, mustDecimal("100"), mustDecimal("1"))
	if capturedKey != "bybit-key" {
		t.Errorf("expected X-BAPI-API-KEY='bybit-key', got %q", capturedKey)
	}
}

func TestBybitAdapter_PlaceLimitOrder_RequestContainsSignatureHeader(t *testing.T) {
	// FAST (<1s)
	var hasSign bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hasSign = r.Header.Get("X-BAPI-SIGN") != ""
		w.Write([]byte(`{"retCode":0,"result":{"orderId":"x"}}`))
	}))
	defer srv.Close()

	bybit.New("k", "secret", srv.URL).PlaceLimitOrder(
		newCtx(), "BTCUSDT", exchange.SideBuy, mustDecimal("100"), mustDecimal("1"))
	if !hasSign {
		t.Error("signed request must include X-BAPI-SIGN header")
	}
}

func TestBybitAdapter_PlaceLimitOrder_ErrorRetCodeReturnsError(t *testing.T) {
	// FAST (<1s)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"retCode":10004,"retMsg":"Insufficient balance","result":{}}`))
	}))
	defer srv.Close()

	_, err := bybit.New("k", "s", srv.URL).PlaceLimitOrder(
		newCtx(), "BTCUSDT", exchange.SideBuy, mustDecimal("100"), mustDecimal("1"))
	if err == nil {
		t.Error("expected error on retCode=10004, got nil")
	}
}

func TestBybitAdapter_Balances_FiltersZeroBalances(t *testing.T) {
	// FAST (<1s)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"retCode":0,"result":{"list":[{"coin":[
			{"coin":"BTC","walletBalance":"1.5","availableToWithdraw":"1.2"},
			{"coin":"DOGE","walletBalance":"0","availableToWithdraw":"0"}
		]}]}}`))
	}))
	defer srv.Close()

	bals, err := bybit.New("k", "s", srv.URL).Balances(newCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bals) != 1 {
		t.Errorf("expected 1 non-zero balance, got %d", len(bals))
	}
}

// ---------------------------------------------------------------------------
// Gate.io — unit tests
// ---------------------------------------------------------------------------

func TestGateAdapter_Name(t *testing.T) {
	// FAST (<1s)
	a := gate.New("k", "s", "http://localhost")
	if a.Name() != "gate" {
		t.Errorf("expected 'gate', got %q", a.Name())
	}
}

func TestGateAdapter_OrderBook_BidCountMatchesFixture(t *testing.T) {
	// FAST (<1s)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(gateDepthJSON))
	}))
	defer srv.Close()

	ob, err := gate.New("k", "s", srv.URL).OrderBook(newCtx(), "BTC_USDT", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ob.Bids) != 2 {
		t.Errorf("expected 2 bids, got %d", len(ob.Bids))
	}
}

func TestGateAdapter_OrderBook_BidPriceDecimalPrecision(t *testing.T) {
	// FAST (<1s)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(gateDepthJSON))
	}))
	defer srv.Close()

	ob, _ := gate.New("k", "s", srv.URL).OrderBook(newCtx(), "BTC_USDT", 5)
	expected := mustDecimal("29000.50")
	if !ob.Bids[0].Price.Equal(expected) {
		t.Errorf("want %s got %s", expected, ob.Bids[0].Price)
	}
}

func TestGateAdapter_PlaceLimitOrder_RequestContainsSignHeaders(t *testing.T) {
	// FAST (<1s)
	var hasSign bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hasSign = r.Header.Get("SIGN") != "" && r.Header.Get("KEY") != "" && r.Header.Get("Timestamp") != ""
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":"GATE-777"}`))
	}))
	defer srv.Close()

	gate.New("mykey", "mysecret", srv.URL).PlaceLimitOrder(
		newCtx(), "BTC_USDT", exchange.SideSell, mustDecimal("100"), mustDecimal("1"))
	if !hasSign {
		t.Error("Gate.io signed request must include KEY, SIGN, and Timestamp headers")
	}
}

func TestGateAdapter_PlaceLimitOrder_ReturnsOrderID(t *testing.T) {
	// FAST (<1s)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":"GATE-777"}`))
	}))
	defer srv.Close()

	order, err := gate.New("k", "s", srv.URL).PlaceLimitOrder(
		newCtx(), "BTC_USDT", exchange.SideSell, mustDecimal("100"), mustDecimal("1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if order.ExchangeOrderID != "GATE-777" {
		t.Errorf("expected GATE-777, got %q", order.ExchangeOrderID)
	}
}

func TestGateAdapter_Balances_FiltersZeroBalances(t *testing.T) {
	// FAST (<1s)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[
			{"currency":"BTC","available":"0.3","locked":"0.05"},
			{"currency":"ETH","available":"0","locked":"0"}
		]`))
	}))
	defer srv.Close()

	bals, err := gate.New("k", "s", srv.URL).Balances(newCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bals) != 1 {
		t.Errorf("expected 1 non-zero balance, got %d", len(bals))
	}
}

// ---------------------------------------------------------------------------
// Kraken — unit tests
// ---------------------------------------------------------------------------

func TestKrakenAdapter_Name(t *testing.T) {
	// FAST (<1s)
	a := kraken.New("k", "aGVsbG8=", "http://localhost")
	if a.Name() != "kraken" {
		t.Errorf("expected 'kraken', got %q", a.Name())
	}
}

func TestKrakenAdapter_OrderBook_BidCountMatchesFixture(t *testing.T) {
	// FAST (<1s)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(krakenDepthJSON("XBTUSDT")))
	}))
	defer srv.Close()

	ob, err := kraken.New("k", "aGVsbG8=", srv.URL).OrderBook(newCtx(), "XBTUSDT", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ob.Bids) != 2 {
		t.Errorf("expected 2 bids, got %d", len(ob.Bids))
	}
}

func TestKrakenAdapter_OrderBook_APIErrorReturnsError(t *testing.T) {
	// FAST (<1s)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"error":["EQuery:Unknown asset pair"],"result":{}}`))
	}))
	defer srv.Close()

	_, err := kraken.New("k", "aGVsbG8=", srv.URL).OrderBook(newCtx(), "INVALID", 5)
	if err == nil {
		t.Error("expected error on Kraken API error array, got nil")
	}
}

func TestKrakenAdapter_OrderBook_BidPriceDecimalPrecision(t *testing.T) {
	// FAST (<1s)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(krakenDepthJSON("XBTUSDT")))
	}))
	defer srv.Close()

	ob, _ := kraken.New("k", "aGVsbG8=", srv.URL).OrderBook(newCtx(), "XBTUSDT", 5)
	expected := mustDecimal("29000.50")
	if !ob.Bids[0].Price.Equal(expected) {
		t.Errorf("want %s got %s", expected, ob.Bids[0].Price)
	}
}

func TestKrakenAdapter_PlaceLimitOrder_RequestContainsAPIKeyHeader(t *testing.T) {
	// FAST (<1s)
	var capturedKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedKey = r.Header.Get("API-Key")
		w.Write([]byte(`{"error":[],"result":{"txid":["KRK-TXN-001"]}}`))
	}))
	defer srv.Close()

	kraken.New("kraken-key", "aGVsbG8=", srv.URL).PlaceLimitOrder(
		newCtx(), "XBTUSDT", exchange.SideBuy, mustDecimal("100"), mustDecimal("1"))
	if capturedKey != "kraken-key" {
		t.Errorf("expected API-Key='kraken-key', got %q", capturedKey)
	}
}

func TestKrakenAdapter_PlaceLimitOrder_RequestContainsSignHeader(t *testing.T) {
	// FAST (<1s)
	var hasSign bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hasSign = r.Header.Get("API-Sign") != ""
		w.Write([]byte(`{"error":[],"result":{"txid":["x"]}}`))
	}))
	defer srv.Close()

	kraken.New("k", "aGVsbG8=", srv.URL).PlaceLimitOrder(
		newCtx(), "XBTUSDT", exchange.SideBuy, mustDecimal("100"), mustDecimal("1"))
	if !hasSign {
		t.Error("Kraken signed request must include API-Sign header")
	}
}

func TestKrakenAdapter_PlaceLimitOrder_ReturnsFirstTxID(t *testing.T) {
	// FAST (<1s)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"error":[],"result":{"txid":["KRK-TXN-001","KRK-TXN-002"]}}`))
	}))
	defer srv.Close()

	order, err := kraken.New("k", "aGVsbG8=", srv.URL).PlaceLimitOrder(
		newCtx(), "XBTUSDT", exchange.SideBuy, mustDecimal("100"), mustDecimal("1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if order.ExchangeOrderID != "KRK-TXN-001" {
		t.Errorf("expected KRK-TXN-001, got %q", order.ExchangeOrderID)
	}
}

func TestKrakenAdapter_PlaceLimitOrder_EmptyTxIDReturnsError(t *testing.T) {
	// FAST (<1s)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"error":[],"result":{"txid":[]}}`))
	}))
	defer srv.Close()

	_, err := kraken.New("k", "aGVsbG8=", srv.URL).PlaceLimitOrder(
		newCtx(), "XBTUSDT", exchange.SideBuy, mustDecimal("100"), mustDecimal("1"))
	if err == nil {
		t.Error("expected error when txid array is empty, got nil")
	}
}

func TestKrakenAdapter_Balances_FiltersZeroBalances(t *testing.T) {
	// FAST (<1s)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := json.Marshal(map[string]interface{}{
			"error":  []string{},
			"result": map[string]string{"XXBT": "0.5", "ZUSD": "0"},
		})
		w.Write(b)
	}))
	defer srv.Close()

	bals, err := kraken.New("k", "aGVsbG8=", srv.URL).Balances(newCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bals) != 1 {
		t.Errorf("expected 1 non-zero balance, got %d", len(bals))
	}
}

// ---------------------------------------------------------------------------
// Cross-adapter contract suite — same behaviour expected from every adapter
// ---------------------------------------------------------------------------

func TestAllAdapters_OrderBook_SymbolFieldMatchesRequest(t *testing.T) {
	// FAST (<1s)
	for _, tc := range allAdapters() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(tc.bookJSON()))
			}))
			defer srv.Close()

			ob, err := tc.factory(srv.URL).OrderBook(newCtx(), tc.symbol, 5)
			if err != nil {
				t.Fatalf("[%s] unexpected error: %v", tc.name, err)
			}
			if ob.Symbol != tc.symbol {
				t.Errorf("[%s] symbol mismatch: want %q got %q", tc.name, tc.symbol, ob.Symbol)
			}
		})
	}
}

func TestAllAdapters_OrderBook_TimestampIsRecent(t *testing.T) {
	// FAST (<1s)
	before := time.Now()
	for _, tc := range allAdapters() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(tc.bookJSON()))
			}))
			defer srv.Close()

			ob, _ := tc.factory(srv.URL).OrderBook(newCtx(), tc.symbol, 5)
			if ob.Timestamp.Before(before) {
				t.Errorf("[%s] timestamp %v predates test start %v", tc.name, ob.Timestamp, before)
			}
		})
	}
}

func TestAllAdapters_NetworkTimeout_ReturnsError(t *testing.T) {
	// FAST (<1s) — context cancelled before server responds
	for _, tc := range allAdapters() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(200 * time.Millisecond)
			}))
			defer srv.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer cancel()
			_, err := tc.factory(srv.URL).OrderBook(ctx, tc.symbol, 5)
			if err == nil {
				t.Errorf("[%s] expected timeout error, got nil", tc.name)
			}
		})
	}
}