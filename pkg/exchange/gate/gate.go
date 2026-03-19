package gate

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/shopspring/decimal"
	"liquidity-guard-bot/pkg/exchange"
)

// Adapter implements exchange.ExchangeAdapter for Gate.io v4.
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

func (a *Adapter) Name() string { return "gate" }

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func (a *Adapter) signHeaders(method, path, query, body string) map[string]string {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	payload := fmt.Sprintf("%s\n%s\n%s\n%s\n%s", method, path, query, sha256Hex(body), ts)
	mac := hmac.New(sha256.New, []byte(a.apiSecret))
	mac.Write([]byte(payload))
	return map[string]string{
		"KEY":       a.apiKey,
		"Timestamp": ts,
		"SIGN":      hex.EncodeToString(mac.Sum(nil)),
	}
}

func (a *Adapter) OrderBook(ctx context.Context, symbol string, depth int) (*exchange.OrderBook, error) {
	path := "/api/v4/spot/order_book"
	query := fmt.Sprintf("currency_pair=%s&limit=%d", symbol, depth)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s%s?%s", a.baseURL, path, query), nil)

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gate order book: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gate order book status %d: %s", resp.StatusCode, body)
	}

	var raw struct {
		Bids [][]string `json:"bids"`
		Asks [][]string `json:"asks"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("gate order book parse: %w", err)
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
	path := "/api/v4/spot/orders"
	bodyStr := fmt.Sprintf(`{"currency_pair":"%s","side":"%s","type":"limit","price":"%s","amount":"%s"}`,
		symbol, side, price.String(), qty.String())
	headers := a.signHeaders(http.MethodPost, path, "", bodyStr)

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gate place order: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gate place order status %d: %s", resp.StatusCode, b)
	}

	var raw struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("gate place order parse: %w", err)
	}
	return &exchange.PlacedOrder{
		ExchangeOrderID: raw.ID,
		Symbol:          symbol,
		Side:            side,
		Price:           price,
		Quantity:        qty,
		Timestamp:       time.Now(),
	}, nil
}

func (a *Adapter) CancelOrder(ctx context.Context, symbol, orderID string) error {
	path := fmt.Sprintf("/api/v4/spot/orders/%s", orderID)
	query := fmt.Sprintf("currency_pair=%s", symbol)
	headers := a.signHeaders(http.MethodDelete, path, query, "")

	req, _ := http.NewRequestWithContext(ctx, http.MethodDelete,
		fmt.Sprintf("%s%s?%s", a.baseURL, path, query), nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("gate cancel order: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("gate cancel order status %d: %s", resp.StatusCode, b)
	}
	return nil
}

func (a *Adapter) Balances(ctx context.Context) ([]exchange.Balance, error) {
	path := "/api/v4/spot/accounts"
	headers := a.signHeaders(http.MethodGet, path, "", "")

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gate balances: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gate balances status %d: %s", resp.StatusCode, b)
	}

	var raw []struct {
		Currency  string `json:"currency"`
		Available string `json:"available"`
		Locked    string `json:"locked"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("gate balances parse: %w", err)
	}

	var out []exchange.Balance
	for _, r := range raw {
		avail, _ := decimal.NewFromString(r.Available)
		locked, _ := decimal.NewFromString(r.Locked)
		if avail.IsZero() && locked.IsZero() {
			continue
		}
		out = append(out, exchange.Balance{Asset: r.Currency, Available: avail, Locked: locked})
	}
	return out, nil
}