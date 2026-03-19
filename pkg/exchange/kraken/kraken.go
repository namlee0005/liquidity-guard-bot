package kraken

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"liquidity-guard-bot/pkg/exchange"
)

// Adapter implements exchange.ExchangeAdapter for Kraken REST v0.
type Adapter struct {
	apiKey    string
	apiSecret string // base64-encoded
	baseURL   string
	client    *http.Client
}

// New returns an Adapter pointed at baseURL. apiSecret must be base64-encoded.
func New(apiKey, apiSecret, baseURL string) *Adapter {
	return &Adapter{
		apiKey:    apiKey,
		apiSecret: apiSecret,
		baseURL:   baseURL,
		client:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (a *Adapter) Name() string { return "kraken" }

func (a *Adapter) sign(path, nonce, encoded string) string {
	sha := sha256.Sum256([]byte(nonce + encoded))
	secret, _ := base64.StdEncoding.DecodeString(a.apiSecret)
	mac := hmac.New(sha512.New, secret)
	mac.Write([]byte(path))
	mac.Write(sha[:])
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func (a *Adapter) OrderBook(ctx context.Context, symbol string, depth int) (*exchange.OrderBook, error) {
	u := fmt.Sprintf("%s/0/public/Depth?pair=%s&count=%d", a.baseURL, symbol, depth)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("kraken order book: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kraken order book status %d: %s", resp.StatusCode, body)
	}

	var raw struct {
		Error  []string                   `json:"error"`
		Result map[string]json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("kraken order book parse: %w", err)
	}
	if len(raw.Error) > 0 {
		return nil, fmt.Errorf("kraken order book api error: %v", raw.Error)
	}

	var inner struct {
		Bids [][]json.RawMessage `json:"bids"`
		Asks [][]json.RawMessage `json:"asks"`
	}
	for _, v := range raw.Result {
		json.Unmarshal(v, &inner)
		break
	}

	parseLevels := func(rows [][]json.RawMessage) []exchange.OrderBookLevel {
		var levels []exchange.OrderBookLevel
		for _, row := range rows {
			if len(row) < 2 {
				continue
			}
			var ps, qs string
			json.Unmarshal(row[0], &ps)
			json.Unmarshal(row[1], &qs)
			p, _ := decimal.NewFromString(ps)
			q, _ := decimal.NewFromString(qs)
			levels = append(levels, exchange.OrderBookLevel{Price: p, Quantity: q})
		}
		return levels
	}

	ob := &exchange.OrderBook{Symbol: symbol, Timestamp: time.Now()}
	ob.Bids = parseLevels(inner.Bids)
	ob.Asks = parseLevels(inner.Asks)
	return ob, nil
}

func (a *Adapter) PlaceLimitOrder(ctx context.Context, symbol string, side exchange.OrderSide, price, qty decimal.Decimal) (*exchange.PlacedOrder, error) {
	nonce := strconv.FormatInt(time.Now().UnixMicro(), 10)
	params := url.Values{}
	params.Set("nonce", nonce)
	params.Set("pair", symbol)
	params.Set("type", string(side))
	params.Set("ordertype", "limit")
	params.Set("price", price.String())
	params.Set("volume", qty.String())
	encoded := params.Encode()
	path := "/0/private/AddOrder"

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		a.baseURL+path, strings.NewReader(encoded))
	req.Header.Set("API-Key", a.apiKey)
	req.Header.Set("API-Sign", a.sign(path, nonce, encoded))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("kraken place order: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)

	var raw struct {
		Error  []string `json:"error"`
		Result struct {
			TxID []string `json:"txid"`
		} `json:"result"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("kraken place order parse: %w", err)
	}
	if len(raw.Error) > 0 {
		return nil, fmt.Errorf("kraken place order error: %v", raw.Error)
	}
	if len(raw.Result.TxID) == 0 {
		return nil, fmt.Errorf("kraken place order: no txid returned")
	}
	return &exchange.PlacedOrder{
		ExchangeOrderID: raw.Result.TxID[0],
		Symbol:          symbol,
		Side:            side,
		Price:           price,
		Quantity:        qty,
		Timestamp:       time.Now(),
	}, nil
}

func (a *Adapter) CancelOrder(ctx context.Context, symbol, orderID string) error {
	nonce := strconv.FormatInt(time.Now().UnixMicro(), 10)
	params := url.Values{}
	params.Set("nonce", nonce)
	params.Set("txid", orderID)
	encoded := params.Encode()
	path := "/0/private/CancelOrder"

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		a.baseURL+path, strings.NewReader(encoded))
	req.Header.Set("API-Key", a.apiKey)
	req.Header.Set("API-Sign", a.sign(path, nonce, encoded))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("kraken cancel order: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var raw struct {
		Error []string `json:"error"`
	}
	json.Unmarshal(b, &raw)
	if len(raw.Error) > 0 {
		return fmt.Errorf("kraken cancel error: %v", raw.Error)
	}
	return nil
}

func (a *Adapter) Balances(ctx context.Context) ([]exchange.Balance, error) {
	nonce := strconv.FormatInt(time.Now().UnixMicro(), 10)
	params := url.Values{}
	params.Set("nonce", nonce)
	encoded := params.Encode()
	path := "/0/private/Balance"

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		a.baseURL+path, strings.NewReader(encoded))
	req.Header.Set("API-Key", a.apiKey)
	req.Header.Set("API-Sign", a.sign(path, nonce, encoded))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("kraken balances: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)

	var raw struct {
		Error  []string          `json:"error"`
		Result map[string]string `json:"result"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("kraken balances parse: %w", err)
	}
	if len(raw.Error) > 0 {
		return nil, fmt.Errorf("kraken balances error: %v", raw.Error)
	}

	var out []exchange.Balance
	for asset, amtStr := range raw.Result {
		amt, _ := decimal.NewFromString(amtStr)
		if amt.IsZero() {
			continue
		}
		out = append(out, exchange.Balance{Asset: asset, Available: amt})
	}
	return out, nil
}

// sha256Hex is used in signHeaders — kept here to avoid cross-package import.
func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}