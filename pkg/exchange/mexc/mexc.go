package mexc

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

// Adapter implements exchange.ExchangeAdapter for MEXC.
type Adapter struct {
	apiKey    string
	apiSecret string
	baseURL   string
	client    *http.Client
}

// New returns an Adapter pointed at baseURL (override in tests via httptest).
func New(apiKey, apiSecret, baseURL string) *Adapter {
	return &Adapter{
		apiKey:    apiKey,
		apiSecret: apiSecret,
		baseURL:   baseURL,
		client:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (a *Adapter) Name() string { return "mexc" }

func (a *Adapter) sign(params url.Values) string {
	params.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))
	mac := hmac.New(sha256.New, []byte(a.apiSecret))
	mac.Write([]byte(params.Encode()))
	return hex.EncodeToString(mac.Sum(nil))
}

func (a *Adapter) OrderBook(ctx context.Context, symbol string, depth int) (*exchange.OrderBook, error) {
	u := fmt.Sprintf("%s/api/v3/depth?symbol=%s&limit=%d", a.baseURL, symbol, depth)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	req.Header.Set("X-MEXC-APIKEY", a.apiKey)

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("mexc order book: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mexc order book status %d: %s", resp.StatusCode, body)
	}

	var raw struct {
		Bids [][]string `json:"bids"`
		Asks [][]string `json:"asks"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("mexc order book parse: %w", err)
	}

	ob := &exchange.OrderBook{Symbol: symbol, Timestamp: time.Now()}
	for _, row := range raw.Bids {
		if len(row) < 2 {
			continue
		}
		p, _ := decimal.NewFromString(row[0])
		q, _ := decimal.NewFromString(row[1])
		ob.Bids = append(ob.Bids, exchange.OrderBookLevel{Price: p, Quantity: q})
	}
	for _, row := range raw.Asks {
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
	params.Set("symbol", symbol)
	params.Set("side", string(side))
	params.Set("type", "LIMIT")
	params.Set("price", price.String())
	params.Set("quantity", qty.String())
	params.Set("signature", a.sign(params))

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/api/v3/order?%s", a.baseURL, params.Encode()), nil)
	req.Header.Set("X-MEXC-APIKEY", a.apiKey)

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("mexc place order: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mexc place order status %d: %s", resp.StatusCode, b)
	}

	var raw struct {
		OrderID string `json:"orderId"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("mexc place order parse: %w", err)
	}
	return &exchange.PlacedOrder{
		ExchangeOrderID: raw.OrderID,
		Symbol:          symbol,
		Side:            side,
		Price:           price,
		Quantity:        qty,
		Timestamp:       time.Now(),
	}, nil
}

func (a *Adapter) CancelOrder(ctx context.Context, symbol, orderID string) error {
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("orderId", orderID)
	params.Set("signature", a.sign(params))

	req, _ := http.NewRequestWithContext(ctx, http.MethodDelete,
		fmt.Sprintf("%s/api/v3/order?%s", a.baseURL, params.Encode()), nil)
	req.Header.Set("X-MEXC-APIKEY", a.apiKey)

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("mexc cancel order: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("mexc cancel order status %d: %s", resp.StatusCode, b)
	}
	return nil
}

func (a *Adapter) Balances(ctx context.Context) ([]exchange.Balance, error) {
	params := url.Values{}
	params.Set("signature", a.sign(params))

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/api/v3/account?%s", a.baseURL, params.Encode()), nil)
	req.Header.Set("X-MEXC-APIKEY", a.apiKey)

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("mexc balances: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mexc balances status %d: %s", resp.StatusCode, b)
	}

	var raw struct {
		Balances []struct {
			Asset  string `json:"asset"`
			Free   string `json:"free"`
			Locked string `json:"locked"`
		} `json:"balances"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("mexc balances parse: %w", err)
	}

	var out []exchange.Balance
	for _, row := range raw.Balances {
		avail, _ := decimal.NewFromString(row.Free)
		locked, _ := decimal.NewFromString(row.Locked)
		if avail.IsZero() && locked.IsZero() {
			continue
		}
		out = append(out, exchange.Balance{Asset: row.Asset, Available: avail, Locked: locked})
	}
	return out, nil
}