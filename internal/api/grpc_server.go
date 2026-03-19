// Package api implements the gRPC BotManagement service.
// It translates proto RPCs into Orchestrator calls and writes AuditLogs to MongoDB.
package api

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	mongoClient "liquidity-guard-bot/pkg/db/mongo"
	"liquidity-guard-bot/internal/models"
	"liquidity-guard-bot/internal/orchestrator"
	"liquidity-guard-bot/internal/telemetry"
	botpb "liquidity-guard-bot/proto/gen"

	"github.com/shopspring/decimal"
	"go.mongodb.org/mongo-driver/bson"
)

// Server implements botpb.BotManagementServer.
type Server struct {
	botpb.UnimplementedBotManagementServer

	orch    *orchestrator.Orchestrator
	hub     *telemetry.Hub
	db      *mongoClient.Client
}

// NewServer wires the gRPC server to the Orchestrator, telemetry Hub, and MongoDB.
func NewServer(orch *orchestrator.Orchestrator, hub *telemetry.Hub, db *mongoClient.Client) *Server {
	return &Server{orch: orch, hub: hub, db: db}
}

// ─── Lifecycle RPCs ───────────────────────────────────────────────────────────

func (s *Server) CreateBot(ctx context.Context, req *botpb.CreateBotRequest) (*botpb.CreateBotResponse, error) {
	if req.BotId == "" || req.TradingPair == "" {
		return nil, status.Error(codes.InvalidArgument, "bot_id and trading_pair are required")
	}

	cfg := models.BotConfig{
		BotID:         req.BotId,
		Exchange:      protoExchangeToModel(req.Exchange),
		TradingPair:   req.TradingPair,
		APIKeyEnc:     req.ApiKey,    // caller encrypts before sending over mTLS
		APISecretEnc:  req.ApiSecret,
		OrderLayers:   int(req.OrderLayers),
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	if req.Spread != nil {
		cfg.Spread = models.SpreadConfig{
			MinSpreadPct: mustDecimal(req.Spread.MinSpreadPct),
			MaxSpreadPct: mustDecimal(req.Spread.MaxSpreadPct),
		}
	}
	if req.Risk != nil {
		cfg.Risk = models.RiskConfig{
			MaxDrawdownPct24h: mustDecimal(req.Risk.MaxDrawdownPct_24H),
			MaxNAVPct:         mustDecimal(req.Risk.MaxNavPct),
		}
	}
	if req.LayerSizeBase != nil {
		cfg.LayerSizeBase = mustDecimal(req.LayerSizeBase)
	}

	// Persist config first — idempotent upsert on bot_id.
	_, err := s.db.Collection(models.CollBotConfigs).UpdateOne(ctx,
		bson.M{"bot_id": cfg.BotID},
		bson.M{"$setOnInsert": cfg},
		options.Update().SetUpsert(true),
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "persist config: %v", err)
	}

	if err := s.orch.StartBot(ctx, cfg); err != nil {
		return nil, status.Errorf(codes.AlreadyExists, "start bot: %v", err)
	}

	s.writeAudit(ctx, cfg.BotID, models.AuditCreate, "system", map[string]interface{}{
		"exchange": cfg.Exchange, "pair": cfg.TradingPair,
	}, "OK")

	return &botpb.CreateBotResponse{
		BotId:     cfg.BotID,
		State:     botpb.BotState_BOT_STATE_RUNNING,
		CreatedAt: timestamppb.New(cfg.CreatedAt),
	}, nil
}

func (s *Server) PauseBot(ctx context.Context, req *botpb.PauseBotRequest) (*botpb.PauseBotResponse, error) {
	if err := s.orch.PauseBot(req.BotId); err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}
	s.writeAudit(ctx, req.BotId, models.AuditPause, "system", nil, "OK")
	return &botpb.PauseBotResponse{BotId: req.BotId, State: botpb.BotState_BOT_STATE_PAUSED}, nil
}

func (s *Server) ResumeBot(ctx context.Context, req *botpb.ResumeBotRequest) (*botpb.ResumeBotResponse, error) {
	if err := s.orch.ResumeBot(req.BotId); err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}
	s.writeAudit(ctx, req.BotId, models.AuditResume, "system", nil, "OK")
	return &botpb.ResumeBotResponse{BotId: req.BotId, State: botpb.BotState_BOT_STATE_RUNNING}, nil
}

func (s *Server) DeleteBot(ctx context.Context, req *botpb.DeleteBotRequest) (*botpb.DeleteBotResponse, error) {
	if err := s.orch.StopBot(req.BotId); err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}
	// Mark session as stopped in MongoDB.
	_, _ = s.db.Collection(models.CollActiveSessions).UpdateOne(ctx,
		bson.M{"bot_id": req.BotId},
		bson.M{"$set": bson.M{"state": models.BotStateStopped, "updated_at": time.Now().UTC()}},
	)
	s.writeAudit(ctx, req.BotId, models.AuditDelete, "system",
		map[string]interface{}{"cancel_open_orders": req.CancelOpenOrders}, "OK")
	return &botpb.DeleteBotResponse{BotId: req.BotId, OrdersCancelled: req.CancelOpenOrders}, nil
}

func (s *Server) UpdateConfig(ctx context.Context, req *botpb.UpdateConfigRequest) (*botpb.UpdateConfigResponse, error) {
	// Fetch existing config to merge.
	var cfg models.BotConfig
	if err := s.db.Collection(models.CollBotConfigs).
		FindOne(ctx, bson.M{"bot_id": req.BotId}).Decode(&cfg); err != nil {
		return nil, status.Errorf(codes.NotFound, "config not found: %v", err)
	}

	if req.Spread != nil {
		cfg.Spread = models.SpreadConfig{
			MinSpreadPct: mustDecimal(req.Spread.MinSpreadPct),
			MaxSpreadPct: mustDecimal(req.Spread.MaxSpreadPct),
		}
	}
	if req.Risk != nil {
		cfg.Risk = models.RiskConfig{
			MaxDrawdownPct24h: mustDecimal(req.Risk.MaxDrawdownPct_24H),
			MaxNAVPct:         mustDecimal(req.Risk.MaxNavPct),
		}
	}
	now := time.Now().UTC()
	cfg.UpdatedAt = now

	_, err := s.db.Collection(models.CollBotConfigs).UpdateOne(ctx,
		bson.M{"bot_id": cfg.BotID},
		bson.M{"$set": cfg},
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "persist update: %v", err)
	}

	if err := s.orch.UpdateConfig(req.BotId, cfg); err != nil {
		return nil, status.Errorf(codes.NotFound, "update config: %v", err)
	}

	s.writeAudit(ctx, req.BotId, models.AuditUpdateConfig, "system", nil, "OK")
	return &botpb.UpdateConfigResponse{
		BotId:     req.BotId,
		State:     botpb.BotState_BOT_STATE_RUNNING,
		UpdatedAt: timestamppb.New(now),
	}, nil
}

// ─── Telemetry stream ─────────────────────────────────────────────────────────

func (s *Server) StreamTelemetry(req *botpb.StreamTelemetryRequest, stream botpb.BotManagement_StreamTelemetryServer) error {
	sub, unsub := s.hub.Subscribe(req.BotIds)
	defer unsub()

	for {
		select {
		case <-stream.Context().Done():
			return nil
		case event, ok := <-sub:
			if !ok {
				return nil
			}
			proto, err := telemetryToProto(event)
			if err != nil {
				continue
			}
			if err := stream.Send(proto); err != nil {
				return err
			}
		}
	}
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func (s *Server) writeAudit(ctx context.Context, botID string, action models.AuditAction, callerID string, payload map[string]interface{}, result string) {
	entry := models.AuditLog{
		ID:        primitive.NewObjectID(),
		BotID:     botID,
		Action:    action,
		CallerID:  callerID,
		Payload:   payload,
		Result:    result,
		Timestamp: time.Now().UTC(),
	}
	_, _ = s.db.Collection(models.CollAuditLogs).InsertOne(ctx, entry)
}

func mustDecimal(d *botpb.DecimalValue) decimal.Decimal {
	if d == nil {
		return decimal.Zero
	}
	v, err := decimal.NewFromString(d.Value)
	if err != nil {
		return decimal.Zero
	}
	return v
}

func protoExchangeToModel(e botpb.Exchange) models.Exchange {
	switch e {
	case botpb.Exchange_EXCHANGE_MEXC:
		return models.ExchangeMEXC
	case botpb.Exchange_EXCHANGE_BYBIT:
		return models.ExchangeBybit
	case botpb.Exchange_EXCHANGE_GATE:
		return models.ExchangeGate
	case botpb.Exchange_EXCHANGE_KRAKEN:
		return models.ExchangeKraken
	default:
		return models.Exchange("UNKNOWN")
	}
}

func telemetryToProto(t interface{}) (*botpb.TelemetryEvent, error) {
	// Conversion from worker.Telemetry → botpb.TelemetryEvent.
	// Full implementation requires proto/gen package to be compiled from bot.proto.
	// Scaffold: returns nil until proto codegen runs in CI.
	return nil, fmt.Errorf("proto gen not yet compiled")
}
