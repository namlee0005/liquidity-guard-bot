package repository

import (
	"context"
	"errors"
	"time"

	"github.com/shopspring/decimal"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ErrNotFound is returned when a requested document does not exist.
var ErrNotFound = errors.New("document not found")

// BotState is the FSM state stored in ActiveSessions.
type BotState string

const (
	BotStateNormal BotState = "NORMAL"
	BotStateSlow   BotState = "SLOW"
	BotStatePause  BotState = "PAUSE"
)

// BotConfig holds per-bot parameters in the BotConfigs collection.
type BotConfig struct {
	ID             primitive.ObjectID `bson:"_id,omitempty"`
	BotID          string             `bson:"bot_id"`
	Exchange       string             `bson:"exchange"`
	Symbol         string             `bson:"symbol"`
	MinSpread      decimal.Decimal    `bson:"min_spread"`
	MaxSpread      decimal.Decimal    `bson:"max_spread"`
	MaxDrawdownPct decimal.Decimal    `bson:"max_drawdown_pct"`
	CreatedAt      time.Time          `bson:"created_at"`
}

// ActiveSession tracks live bot state in the ActiveSessions collection.
type ActiveSession struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"`
	BotID     string             `bson:"bot_id"`
	State     BotState           `bson:"state"`
	Heartbeat time.Time          `bson:"heartbeat"`
	UpdatedAt time.Time          `bson:"updated_at"`
}

// TradeRecord is a single fill, placement, or cancellation event.
type TradeRecord struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"`
	BotID     string             `bson:"bot_id"`
	OrderID   string             `bson:"order_id"`
	Symbol    string             `bson:"symbol"`
	Side      string             `bson:"side"`       // "buy" | "sell"
	Price     decimal.Decimal    `bson:"price"`
	Quantity  decimal.Decimal    `bson:"quantity"`
	EventType string             `bson:"event_type"` // "place" | "fill" | "cancel"
	Timestamp time.Time          `bson:"timestamp"`
}

// AuditLog records an external management action.
type AuditLog struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"`
	BotID     string             `bson:"bot_id"`
	Action    string             `bson:"action"` // "create" | "pause" | "delete" | "update_config"
	Actor     string             `bson:"actor"`
	Payload   string             `bson:"payload"` // JSON-encoded before/after state
	Timestamp time.Time          `bson:"timestamp"`
}

// BotConfigRepo manages the BotConfigs collection.
type BotConfigRepo interface {
	Insert(ctx context.Context, cfg *BotConfig) error
	FindByBotID(ctx context.Context, botID string) (*BotConfig, error)
	Update(ctx context.Context, cfg *BotConfig) error
	Delete(ctx context.Context, botID string) error
}

// SessionRepo manages the ActiveSessions collection.
type SessionRepo interface {
	Upsert(ctx context.Context, session *ActiveSession) error
	FindByBotID(ctx context.Context, botID string) (*ActiveSession, error)
	SetState(ctx context.Context, botID string, state BotState) error
	Delete(ctx context.Context, botID string) error
}

// TradeRepo manages the TradeHistory collection.
type TradeRepo interface {
	Insert(ctx context.Context, record *TradeRecord) error
	FindByBotID(ctx context.Context, botID string, limit int) ([]TradeRecord, error)
	DrawdownSince(ctx context.Context, botID string, since time.Time) (decimal.Decimal, error)
}

// AuditRepo manages the AuditLogs collection.
type AuditRepo interface {
	Insert(ctx context.Context, log *AuditLog) error
	FindByBotID(ctx context.Context, botID string, limit int) ([]AuditLog, error)
}