package repository_test

// All tests use go.mongodb.org/mongo-driver/mongo/integration/mtest.
// No running MongoDB required. All tests: FAST (<1s).
// Run: go test -race ./internal/repository/...

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"

	. "liquidity-guard-bot/internal/repository"
)

func rctx() context.Context {
	c, _ := context.WithTimeout(context.Background(), 2*time.Second)
	return c
}

// ─────────────────────────────────────────────
// BotConfigRepo
// ─────────────────────────────────────────────

func TestMongoBotConfigRepo_Insert_Succeeds(t *testing.T) {
	// FAST (<1s)
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("ok", func(mt *mtest.T) {
		mt.AddMockResponses(mtest.CreateSuccessResponse())
		repo := NewMongoBotConfigRepo(mt.DB)
		err := repo.Insert(rctx(), &BotConfig{BotID: "b1", Exchange: "mexc", Symbol: "BTCUSDT"})
		if err != nil {
			t.Errorf("expected nil, got: %v", err)
		}
	})
}

func TestMongoBotConfigRepo_Insert_SetsCreatedAt(t *testing.T) {
	// FAST (<1s)
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("created_at", func(mt *mtest.T) {
		mt.AddMockResponses(mtest.CreateSuccessResponse())
		cfg := &BotConfig{BotID: "b2", Exchange: "bybit", Symbol: "ETHUSDT"}
		before := time.Now()
		_ = NewMongoBotConfigRepo(mt.DB).Insert(rctx(), cfg)
		if cfg.CreatedAt.Before(before) {
			t.Errorf("created_at %v must not predate insertion %v", cfg.CreatedAt, before)
		}
	})
}

func TestMongoBotConfigRepo_FindByBotID_ReturnsBotID(t *testing.T) {
	// FAST (<1s)
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("find", func(mt *mtest.T) {
		mt.AddMockResponses(mtest.CreateCursorResponse(1, "test.bot_configs", mtest.FirstBatch,
			bson.D{{Key: "bot_id", Value: "b3"}, {Key: "exchange", Value: "gate"}, {Key: "symbol", Value: "SOLUSDT"}},
		))
		got, err := NewMongoBotConfigRepo(mt.DB).FindByBotID(rctx(), "b3")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.BotID != "b3" {
			t.Errorf("expected bot_id='b3', got %q", got.BotID)
		}
	})
}

func TestMongoBotConfigRepo_FindByBotID_MissingReturnsErrNotFound(t *testing.T) {
	// FAST (<1s)
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("not found", func(mt *mtest.T) {
		mt.AddMockResponses(mtest.CreateCursorResponse(0, "test.bot_configs", mtest.FirstBatch))
		_, err := NewMongoBotConfigRepo(mt.DB).FindByBotID(rctx(), "ghost")
		if err != ErrNotFound {
			t.Errorf("expected ErrNotFound, got: %v", err)
		}
	})
}

func TestMongoBotConfigRepo_Update_ZeroMatchedReturnsErrNotFound(t *testing.T) {
	// FAST (<1s)
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("update miss", func(mt *mtest.T) {
		mt.AddMockResponses(bson.D{{Key: "ok", Value: 1}, {Key: "n", Value: 0}, {Key: "nModified", Value: 0}})
		err := NewMongoBotConfigRepo(mt.DB).Update(rctx(), &BotConfig{BotID: "ghost"})
		if err != ErrNotFound {
			t.Errorf("expected ErrNotFound, got: %v", err)
		}
	})
}

func TestMongoBotConfigRepo_Delete_ZeroDeletedReturnsErrNotFound(t *testing.T) {
	// FAST (<1s)
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("delete miss", func(mt *mtest.T) {
		mt.AddMockResponses(bson.D{{Key: "ok", Value: 1}, {Key: "n", Value: 0}})
		err := NewMongoBotConfigRepo(mt.DB).Delete(rctx(), "ghost")
		if err != ErrNotFound {
			t.Errorf("expected ErrNotFound, got: %v", err)
		}
	})
}

// ─────────────────────────────────────────────
// SessionRepo
// ─────────────────────────────────────────────

func TestMongoSessionRepo_Upsert_SetsUpdatedAt(t *testing.T) {
	// FAST (<1s)
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("updated_at", func(mt *mtest.T) {
		mt.AddMockResponses(mtest.CreateSuccessResponse())
		s := &ActiveSession{BotID: "b1", State: BotStateNormal}
		before := time.Now()
		_ = NewMongoSessionRepo(mt.DB).Upsert(rctx(), s)
		if s.UpdatedAt.Before(before) {
			t.Errorf("updated_at %v predates upsert %v", s.UpdatedAt, before)
		}
	})
}

func TestMongoSessionRepo_FindByBotID_ReturnsState(t *testing.T) {
	// FAST (<1s)
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("find slow", func(mt *mtest.T) {
		mt.AddMockResponses(mtest.CreateCursorResponse(1, "test.active_sessions", mtest.FirstBatch,
			bson.D{{Key: "bot_id", Value: "b1"}, {Key: "state", Value: string(BotStateSlow)}},
		))
		got, err := NewMongoSessionRepo(mt.DB).FindByBotID(rctx(), "b1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.State != BotStateSlow {
			t.Errorf("expected SLOW, got %q", got.State)
		}
	})
}

func TestMongoSessionRepo_FindByBotID_MissingReturnsErrNotFound(t *testing.T) {
	// FAST (<1s)
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("session not found", func(mt *mtest.T) {
		mt.AddMockResponses(mtest.CreateCursorResponse(0, "test.active_sessions", mtest.FirstBatch))
		_, err := NewMongoSessionRepo(mt.DB).FindByBotID(rctx(), "ghost")
		if err != ErrNotFound {
			t.Errorf("expected ErrNotFound, got: %v", err)
		}
	})
}

func TestMongoSessionRepo_SetState_ZeroMatchedReturnsErrNotFound(t *testing.T) {
	// FAST (<1s)
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("set state miss", func(mt *mtest.T) {
		mt.AddMockResponses(bson.D{{Key: "ok", Value: 1}, {Key: "n", Value: 0}, {Key: "nModified", Value: 0}})
		err := NewMongoSessionRepo(mt.DB).SetState(rctx(), "ghost", BotStatePause)
		if err != ErrNotFound {
			t.Errorf("expected ErrNotFound, got: %v", err)
		}
	})
}

// ─────────────────────────────────────────────
// TradeRepo
// ─────────────────────────────────────────────

func TestMongoTradeRepo_Insert_SetsTimestamp(t *testing.T) {
	// FAST (<1s)
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("timestamp", func(mt *mtest.T) {
		mt.AddMockResponses(mtest.CreateSuccessResponse())
		rec := &TradeRecord{BotID: "b1", OrderID: "O1", Side: "buy",
			Price: decimal.NewFromFloat(29000), Quantity: decimal.NewFromFloat(0.1), EventType: "place"}
		before := time.Now()
		_ = NewMongoTradeRepo(mt.DB).Insert(rctx(), rec)
		if rec.Timestamp.Before(before) {
			t.Errorf("timestamp %v predates insert %v", rec.Timestamp, before)
		}
	})
}

func TestMongoTradeRepo_FindByBotID_ReturnsRecords(t *testing.T) {
	// FAST (<1s)
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("find records", func(mt *mtest.T) {
		mt.AddMockResponses(
			mtest.CreateCursorResponse(1, "test.trade_history", mtest.FirstBatch,
				bson.D{{Key: "bot_id", Value: "b1"}, {Key: "event_type", Value: "fill"}, {Key: "side", Value: "buy"}},
			),
			mtest.CreateCursorResponse(0, "test.trade_history", mtest.NextBatch),
		)
		records, err := NewMongoTradeRepo(mt.DB).FindByBotID(rctx(), "b1", 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(records) != 1 {
			t.Errorf("expected 1 record, got %d", len(records))
		}
	})
}

func TestMongoTradeRepo_DrawdownSince_NoFillsIsZero(t *testing.T) {
	// FAST (<1s)
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("zero drawdown", func(mt *mtest.T) {
		mt.AddMockResponses(mtest.CreateCursorResponse(0, "test.trade_history", mtest.FirstBatch))
		dd, err := NewMongoTradeRepo(mt.DB).DrawdownSince(rctx(), "b1", time.Now().Add(-24*time.Hour))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !dd.IsZero() {
			t.Errorf("expected zero drawdown with no fills, got %s", dd)
		}
	})
}

func TestMongoTradeRepo_DrawdownSince_ProfitableCycleIsZero(t *testing.T) {
	// FAST (<1s)
	// buy 1 @ 100, sell 1 @ 101 = net +1 (profit). Drawdown must be zero.
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("profitable zero drawdown", func(mt *mtest.T) {
		mt.AddMockResponses(
			mtest.CreateCursorResponse(1, "test.trade_history", mtest.FirstBatch,
				bson.D{{Key: "bot_id", Value: "b1"}, {Key: "event_type", Value: "fill"},
					{Key: "side", Value: "buy"}, {Key: "price", Value: "100"}, {Key: "quantity", Value: "1"}},
				bson.D{{Key: "bot_id", Value: "b1"}, {Key: "event_type", Value: "fill"},
					{Key: "side", Value: "sell"}, {Key: "price", Value: "101"}, {Key: "quantity", Value: "1"}},
			),
			mtest.CreateCursorResponse(0, "test.trade_history", mtest.NextBatch),
		)
		dd, _ := NewMongoTradeRepo(mt.DB).DrawdownSince(rctx(), "b1", time.Now().Add(-24*time.Hour))
		if !dd.IsZero() {
			t.Errorf("profitable cycle must report zero drawdown, got %s", dd)
		}
	})
}

// ─────────────────────────────────────────────
// AuditRepo
// ─────────────────────────────────────────────

func TestMongoAuditRepo_Insert_SetsIDAndTimestamp(t *testing.T) {
	// FAST (<1s)
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("id+ts", func(mt *mtest.T) {
		mt.AddMockResponses(mtest.CreateSuccessResponse())
		log := &AuditLog{BotID: "b1", Action: "create", Actor: "admin"}
		before := time.Now()
		_ = NewMongoAuditRepo(mt.DB).Insert(rctx(), log)
		if log.ID.IsZero() {
			t.Error("ID must be populated after Insert")
		}
		if log.Timestamp.Before(before) {
			t.Errorf("timestamp %v predates insert %v", log.Timestamp, before)
		}
	})
}

func TestMongoAuditRepo_FindByBotID_ReturnsLogs(t *testing.T) {
	// FAST (<1s)
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("find logs", func(mt *mtest.T) {
		mt.AddMockResponses(
			mtest.CreateCursorResponse(1, "test.audit_logs", mtest.FirstBatch,
				bson.D{{Key: "bot_id", Value: "b1"}, {Key: "action", Value: "pause"}},
			),
			mtest.CreateCursorResponse(0, "test.audit_logs", mtest.NextBatch),
		)
		logs, err := NewMongoAuditRepo(mt.DB).FindByBotID(rctx(), "b1", 5)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(logs) != 1 {
			t.Errorf("expected 1 audit log, got %d", len(logs))
		}
	})
}

func TestMongoAuditRepo_FindByBotID_EmptyIsNotError(t *testing.T) {
	// FAST (<1s)
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	mt.Run("empty ok", func(mt *mtest.T) {
		mt.AddMockResponses(mtest.CreateCursorResponse(0, "test.audit_logs", mtest.FirstBatch))
		logs, err := NewMongoAuditRepo(mt.DB).FindByBotID(rctx(), "b1", 5)
		if err != nil {
			t.Fatalf("empty result must not error: %v", err)
		}
		if logs == nil {
			t.Error("empty result must be empty slice, not nil")
		}
	})
}