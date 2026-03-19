package bybit

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/shopspring/decimal"
	"liquidity-guard-bot/pkg/exchange"
)

// Adapter implements exchange.ExchangeAdapter for Bybit v5.
type Adapter struct {
	apiKey    string
	apiSecret string
	baseURL   string
	client    *http.Client
}

// New returns an Adapter pointed at baseURL.
func New(apiKey, apiSecret, baseURL string) *Adapter {
	return &Adapter{
		apiKey:    apiKey,
		apiSecret: apiSecret,
		baseURL:   baseURL,
		client:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (a *Adapter) Name() string { return "bybit" }

func (a *Adapter) sign(params url.Values) (sig, ts string) {
	ts = strconv.FormatInt(time.Now().UnixMilli(), 10)
	const recvWindow = "5000"
	payload := ts + a.apiKey + recvWindow + params.Encode()
	mac := hmac.New(sha256.New, []byte(a.apiSecret))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil)), ts
}

func (a *Adapter) OrderBook(ctx context.Context, symbol string, depth int) (*exchange.OrderBook, error) {
	u := fmt.Sprintf("%s/v5/market/orderbook?category=spot&symbol=%s&limit=%d", a.baseURL, symbol, depth)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bybit order book: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bybit order book status %d: %s", resp.StatusCode, body)
	}

	var raw struct {
		RetCode int    `json:"retCode"`
		RetMsg  string `json:"retMsg"`
		Result  struct {
			B [][]string `json:"b"`
			A [][]string `json:"a"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("bybit order book parse: %w", err)
	}
	if raw.RetCode != 0 {
		return nil, fmt.Errorf("bybit order book api error %d: %s", raw.RetCode, raw.RetMsg)
	}

	ob := &exchange.OrderBook{Symbol: symbol, Timestamp: time.Now()}
	for _, row := range raw.Result.B {
		if len(row) < 2 {
			continue
		}
		p, _ := decimal.NewFromString(row[0])
		q, _ := decimal.NewFromString(row[1])
		ob.Bids = append(ob.Bids, exchange.OrderBookLevel{Price: p, Quantity: q})
	}
	for _, row := range raw.Result.A {
		if len(row) < 2 {
			continue
		}
		p, _ := decimal.NewFromString(row[0])
		q, _ := decimal.NewFromString(row[1])
		ob.Asks = append(ob.Asks, exchange.OrderBookLevel{Price: p, Quantity: q})
	}
	return ob, nil
}

func (a *Adapter) PlaceLimitOrder(ctx context.Context, symbol string, side exchange.OrderSide, price, qty decimal.Decimal) (*exchange.PlacedOrder, error) {
	params := url.Values{}
	params.Set("category", "spot")
	params.Set("symbol", symbol)
	params.Set("side", string(side))
	params.Set("orderType", "Limit")
	params.Set("price", price.String())
	params.Set("qty", qty.String())
	sig, ts := a.sign(params)

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/v5/order/create?%s", a.baseURL, params.Encode()), nil)
	req.Header.Set("X-BAPI-API-KEY", a.apiKey)
	req.Header.Set("X-BAPI-SIGN", sig)
	req.Header.Set("X-BAPI-TIMESTAMP", ts)
	req.Header.Set("X-BAPI-RECV-WINDOW", "5000")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bybit place order: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)

	var raw struct {
		RetCode int    `json:"retCode"`
		RetMsg  string `json:"retMsg"`
		Result  struct {
			OrderID string `json:"orderId"`
		} `json:"result"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("bybit place order parse: %w", err)
	}
	if raw.RetCode != 0 {
		return nil, fmt.Errorf("bybit place order error %d: %s", raw.RetCode, raw.RetMsg)
	}
	return &exchange.PlacedOrder{
		ExchangeOrderID: raw.Result.OrderID,
		Symbol:          symbol,
		Side:            side,
		Price:           price,
		Quantity:        qty,
		Timestamp:       time.Now(),
	}, nil
}

func (a *Adapter) CancelOrder(ctx context.Context, symbol, orderID string) error {
	params := url.Values{}
	params.Set("category", "spot")
	params.Set("symbol", symbol)
	params.Set("orderId", orderID)
	sig, ts := a.sign(params)

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/v5/order/cancel?%s", a.baseURL, params.Encode()), nil)
	req.Header.Set("X-BAPI-API-KEY", a.apiKey)
	req.Header.Set("X-BAPI-SIGN", sig)
	req.Header.Set("X-BAPI-TIMESTAMP", ts)
	req.Header.Set("X-BAPI-RECV-WINDOW", "5000")

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("bybit cancel order: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var raw struct {
		RetCode int    `json:"retCode"`
		RetMsg  string `json:"retMsg"`
	}
	json.Unmarshal(b, &raw)
	if raw.RetCode != 0 {
		return fmt.Errorf("bybit cancel error %d: %s", raw.RetCode, raw.RetMsg)
	}
	return nil
}

func (a *Adapter) Balances(ctx context.Context) ([]exchange.Balance, error) {
	params := url.Values{}
	params.Set("accountType", "SPOT")
	sig, ts := a.sign(params)

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/v5/account/wallet-balance?%s", a.baseURL, params.Encode()), nil)
	req.Header.Set("X-BAPI-API-KEY", a.apiKey)
	req.Header.Set("X-BAPI-SIGN", sig)
	req.Header.Set("X-BAPI-TIMESTAMP", ts)
	req.Header.Set("X-BAPI-RECV-WINDOW", "5000")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bybit balances: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)

	var raw struct {
		RetCode int    `json:"retCode"`
		RetMsg  string `json:"retMsg"`
		Result  struct {
			List []struct {
				Coin []struct {
					Coin            string `json:"coin"`
					WalletBalance   string `json:"walletBalance"`
					AvailableToWith string `json:"availableToWithdraw"`
				} `json:"coin"`
			} `json:"list"`
		} `json:"result"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("bybit balances parse: %w", err)
	}
	if raw.RetCode != 0 {
		return nil, fmt.Errorf("bybit balances error %d: %s", raw.RetCode, raw.RetMsg)
	}

	var out []exchange.Balance
	for _, acct := range raw.Result.List {
		for _, c := range acct.Coin {
			avail, _ := decimal.NewFromString(c.AvailableToWith)
			total, _ := decimal.NewFromString(c.WalletBalance)
			locked := total.Sub(avail)
			if avail.IsZero() && locked.IsZero() {
				continue
			}
			out = append(out, exchange.Balance{Asset: c.Coin, Available: avail, Locked: locked})
		}
	}
	return out, nil
}