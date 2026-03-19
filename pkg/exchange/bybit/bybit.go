// Package bybit implements ExchangeAdapter for Bybit.
// Bybit V5 Unified API; WebSocket endpoint: wss://stream.bybit.com/v5/public/spot
// Rate limits: 10 req/s per endpoint category; WebSocket: 1 conn per IP for public.
package bybit

import (
	"context"

	"liquidity-guard-bot/pkg/exchange"
)

const exchangeName = "BYBIT"

// Adapter implements exchange.ExchangeAdapter for Bybit.
type Adapter struct {
	apiKey    string
	apiSecret string
}

func New(apiKey, apiSecret string) *Adapter {
	return &Adapter{apiKey: apiKey, apiSecret: apiSecret}
}

func (a *Adapter) Exchange() string { return exchangeName }

func (a *Adapter) WatchOrderBook(ctx context.Context, tradingPair string) (<-chan exchange.OrderBook, error) {
	panic("bybit.Adapter.WatchOrderBook: not implemented — Phase 4")
}

func (a *Adapter) PlaceOrder(ctx context.Context, req exchange.PlaceOrderRequest) (exchange.PlaceOrderResult, error) {
	panic("bybit.Adapter.PlaceOrder: not implemented — Phase 4")
}

func (a *Adapter) CancelOrder(ctx context.Context, tradingPair, exchangeOrderID string) error {
	panic("bybit.Adapter.CancelOrder: not implemented — Phase 4")
}

func (a *Adapter) GetBalances(ctx context.Context) ([]exchange.Balance, error) {
	panic("bybit.Adapter.GetBalances: not implemented — Phase 4")
}
