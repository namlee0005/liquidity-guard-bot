// Package mexc implements ExchangeAdapter for MEXC.
// MEXC uses a non-standard WebSocket frame format; see their V3 Spot WS docs.
// Rate limits: 20 req/s REST, 5 WS subscriptions per connection.
package mexc

import (
	"context"

	"github.com/namlee0005/liquidity-guard-bot/pkg/exchange"
)

const exchangeName = "MEXC"

// Adapter implements exchange.ExchangeAdapter for MEXC.
type Adapter struct {
	apiKey    string
	apiSecret string
	// wsConn  *websocket.Conn  — added in Phase 4
}

func New(apiKey, apiSecret string) *Adapter {
	return &Adapter{apiKey: apiKey, apiSecret: apiSecret}
}

func (a *Adapter) Exchange() string { return exchangeName }

func (a *Adapter) WatchOrderBook(ctx context.Context, tradingPair string) (<-chan exchange.OrderBook, error) {
	panic("mexc.Adapter.WatchOrderBook: not implemented — Phase 4")
}

func (a *Adapter) PlaceOrder(ctx context.Context, req exchange.PlaceOrderRequest) (exchange.PlaceOrderResult, error) {
	panic("mexc.Adapter.PlaceOrder: not implemented — Phase 4")
}

func (a *Adapter) CancelOrder(ctx context.Context, tradingPair, exchangeOrderID string) error {
	panic("mexc.Adapter.CancelOrder: not implemented — Phase 4")
}

func (a *Adapter) GetBalances(ctx context.Context) ([]exchange.Balance, error) {
	panic("mexc.Adapter.GetBalances: not implemented — Phase 4")
}
