// Package gate implements ExchangeAdapter for Gate.io.
// Gate.io Spot WebSocket v4: wss://api.gateio.ws/ws/v4/
// Rate limits: 900 req/min REST; WebSocket: max 100 subscriptions per connection.
package gate

import (
	"context"

	"liquidity-guard-bot/pkg/exchange"
)

const exchangeName = "GATE"

// Adapter implements exchange.ExchangeAdapter for Gate.io.
type Adapter struct {
	apiKey    string
	apiSecret string
}

func New(apiKey, apiSecret string) *Adapter {
	return &Adapter{apiKey: apiKey, apiSecret: apiSecret}
}

func (a *Adapter) Exchange() string { return exchangeName }

func (a *Adapter) WatchOrderBook(ctx context.Context, tradingPair string) (<-chan exchange.OrderBook, error) {
	panic("gate.Adapter.WatchOrderBook: not implemented — Phase 4")
}

func (a *Adapter) PlaceOrder(ctx context.Context, req exchange.PlaceOrderRequest) (exchange.PlaceOrderResult, error) {
	panic("gate.Adapter.PlaceOrder: not implemented — Phase 4")
}

func (a *Adapter) CancelOrder(ctx context.Context, tradingPair, exchangeOrderID string) error {
	panic("gate.Adapter.CancelOrder: not implemented — Phase 4")
}

func (a *Adapter) GetBalances(ctx context.Context) ([]exchange.Balance, error) {
	panic("gate.Adapter.GetBalances: not implemented — Phase 4")
}
