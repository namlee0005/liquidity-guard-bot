// Package kraken implements ExchangeAdapter for Kraken.
// Kraken WebSocket v2: wss://ws.kraken.com/v2
// Quirk: Kraken uses asset pair naming (XBTUSD, not BTC/USDT); normalization required.
// Rate limits: REST 15–20 tokens refilled at 0.33/s; WebSocket: no hard sub limit.
package kraken

import (
	"context"

	"github.com/namlee0005/liquidity-guard-bot/pkg/exchange"
)

const exchangeName = "KRAKEN"

// Adapter implements exchange.ExchangeAdapter for Kraken.
type Adapter struct {
	apiKey    string
	apiSecret string
}

func New(apiKey, apiSecret string) *Adapter {
	return &Adapter{apiKey: apiKey, apiSecret: apiSecret}
}

func (a *Adapter) Exchange() string { return exchangeName }

func (a *Adapter) WatchOrderBook(ctx context.Context, tradingPair string) (<-chan exchange.OrderBook, error) {
	panic("kraken.Adapter.WatchOrderBook: not implemented — Phase 4")
}

func (a *Adapter) PlaceOrder(ctx context.Context, req exchange.PlaceOrderRequest) (exchange.PlaceOrderResult, error) {
	panic("kraken.Adapter.PlaceOrder: not implemented — Phase 4")
}

func (a *Adapter) CancelOrder(ctx context.Context, tradingPair, exchangeOrderID string) error {
	panic("kraken.Adapter.CancelOrder: not implemented — Phase 4")
}

func (a *Adapter) GetBalances(ctx context.Context) ([]exchange.Balance, error) {
	panic("kraken.Adapter.GetBalances: not implemented — Phase 4")
}
