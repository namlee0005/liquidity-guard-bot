// Package models defines the canonical MongoDB document structs for the
// Liquidity Guard Bot. All monetary fields use shopspring/decimal stored as
// BSON Decimal128 via the custom codec registered in pkg/db/mongo/client.go.
// float64 is strictly prohibited.
package models

import (
	"time"

	"github.com/shopspring/decimal"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Collection names — use these constants everywhere; no magic strings.
const (
	CollBotConfigs     = "BotConfigs"
	CollActiveSessions = "ActiveSessions"
	CollTradeHistory   = "TradeHistory"
	CollAuditLogs      = "AuditLogs"
)

// ─── Enums ────────────────────────────────────────────────────────────────────

type Exchange string

const (
	ExchangeMEXC   Exchange = "MEXC"
	ExchangeBybit  Exchange = "BYBIT"
	ExchangeGate   Exchange = "GATE"
	ExchangeKraken Exchange = "KRAKEN"
)

type BotState string

const (
	BotStateRunning BotState = "RUNNING"
	BotStateSlow    BotState = "SLOW"   // risk watchdog throttle
	BotStatePaused  BotState = "PAUSED"
	BotStateStopped BotState = "STOPPED"
)

type OrderSide string

const (
	SideBid OrderSide = "BID"
	SideAsk OrderSide = "ASK"
)

type TradeAction string

const (
	ActionPlaced    TradeAction = "PLACED"
	ActionFilled    TradeAction = "FILLED"
	ActionCancelled TradeAction = "CANCELLED"
	ActionRejected  TradeAction = "REJECTED"
)

type AuditAction string

const (
	AuditCreate       AuditAction = "CREATE_BOT"
	AuditPause        AuditAction = "PAUSE_BOT"
	AuditResume       AuditAction = "RESUME_BOT"
	AuditDelete       AuditAction = "DELETE_BOT"
	AuditUpdateConfig AuditAction = "UPDATE_CONFIG"
)

// ─── Embedded sub-documents ───────────────────────────────────────────────────

// SpreadConfig defines the allowed spread band for a trading pair.
type SpreadConfig struct {
	MinSpreadPct decimal.Decimal `bson:"min_spread_pct"` // e.g. 0.003 = 0.3%
	MaxSpreadPct decimal.Decimal `bson:"max_spread_pct"` // e.g. 0.01  = 1.0%
}

// RiskConfig defines NAV drawdown limits enforced by the Risk Watchdog.
type RiskConfig struct {
	MaxDrawdownPct24h decimal.Decimal `bson:"max_drawdown_pct_24h"` // e.g. 0.05 = 5%
	MaxNAVPct         decimal.Decimal `bson:"max_nav_pct"`          // e.g. 0.10 = 10%
}

// ─── BotConfig (collection: BotConfigs) ──────────────────────────────────────

// BotConfig stores static bot parameters and exchange credentials.
// Credentials are encrypted before insertion; the application layer owns
// encrypt/decrypt. This struct holds the ciphertext fields only.
type BotConfig struct {
	ID             primitive.ObjectID `bson:"_id,omitempty"`
	BotID          string             `bson:"bot_id"`          // idempotency key, unique index
	Exchange       Exchange           `bson:"exchange"`
	TradingPair    string             `bson:"trading_pair"`    // e.g. "BTC/USDT"
	APIKeyEnc      string             `bson:"api_key_enc"`     // AES-GCM ciphertext, base64
	APISecretEnc   string             `bson:"api_secret_enc"`  // AES-GCM ciphertext, base64
	Spread         SpreadConfig       `bson:"spread"`
	Risk           RiskConfig         `bson:"risk"`
	OrderLayers    int                `bson:"order_layers"`    // number of bid/ask layers
	LayerSizeBase  decimal.Decimal    `bson:"layer_size_base"` // size per layer in base currency
	CreatedAt      time.Time          `bson:"created_at"`
	UpdatedAt      time.Time          `bson:"updated_at"`
}

// ─── BotSession (collection: ActiveSessions) ─────────────────────────────────

// BotSession is the live state document for a running bot.
// The Risk Watchdog writes RiskState; the bot goroutine writes everything else.
// The bot polls this document every 5s for PAUSE/CONFIG_UPDATE signals.
type BotSession struct {
	ID              primitive.ObjectID `bson:"_id,omitempty"`
	BotID           string             `bson:"bot_id"`          // FK → BotConfig.BotID
	State           BotState           `bson:"state"`
	RiskState       BotState           `bson:"risk_state"`      // written by Risk Watchdog
	CurrentNAV      decimal.Decimal    `bson:"current_nav"`     // base currency
	DrawdownPct24h  decimal.Decimal    `bson:"drawdown_pct_24h"`
	OpenOrderCount  int                `bson:"open_order_count"`
	LastHeartbeat   time.Time          `bson:"last_heartbeat"`  // updated every cycle
	ConfigVersion   int64              `bson:"config_version"`  // incremented on UpdateConfig
	StartedAt       time.Time          `bson:"started_at"`
	UpdatedAt       time.Time          `bson:"updated_at"`
}

// ─── TradeRecord (collection: TradeHistory) ───────────────────────────────────

// TradeRecord captures every order lifecycle event.
// Writes use WriteConcern: Majority (enforced at the client level).
// Multi-step rebalancing writes use MongoDB sessions/transactions.
type TradeRecord struct {
	ID          primitive.ObjectID `bson:"_id,omitempty"`
	BotID       string             `bson:"bot_id"`
	Exchange    Exchange           `bson:"exchange"`
	TradingPair string             `bson:"trading_pair"`
	OrderID     string             `bson:"order_id"`    // exchange-assigned order ID
	ClientOID   string             `bson:"client_oid"`  // our internal idempotency key
	Side        OrderSide          `bson:"side"`
	Action      TradeAction        `bson:"action"`
	Price       decimal.Decimal    `bson:"price"`
	Quantity    decimal.Decimal    `bson:"quantity"`
	FilledQty   decimal.Decimal    `bson:"filled_qty"`
	Fee         decimal.Decimal    `bson:"fee"`
	FeeAsset    string             `bson:"fee_asset"`
	Layer       int                `bson:"layer"`       // which depth layer (1-N)
	Timestamp   time.Time          `bson:"timestamp"`   // exchange event time (UTC)
	RecordedAt  time.Time          `bson:"recorded_at"` // our insertion time (UTC)
}

// ─── AuditLog (collection: AuditLogs) ────────────────────────────────────────

// AuditLog records every external management action received via the gRPC API.
// Immutable after insertion — no update or delete operations on this collection.
// Writes use WriteConcern: Majority (enforced at the client level).
type AuditLog struct {
	ID         primitive.ObjectID     `bson:"_id,omitempty"`
	BotID      string                 `bson:"bot_id"`
	Action     AuditAction            `bson:"action"`
	CallerID   string                 `bson:"caller_id"`   // API key identifier of requester
	Payload    map[string]interface{} `bson:"payload"`     // sanitized request parameters
	Result     string                 `bson:"result"`      // "OK" or error message
	Timestamp  time.Time              `bson:"timestamp"`   // UTC, always set by application
}
