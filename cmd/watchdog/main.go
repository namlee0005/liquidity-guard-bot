// cmd/watchdog is a standalone binary that monitors NAV drawdown for all active
// bots and writes risk state (NORMAL / SLOW / PAUSE) directly to their session
// documents in MongoDB. It has NO import dependency on the main bot binary.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/shopspring/decimal"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"

	mongoClient "liquidity-guard-bot/pkg/db/mongo"
	"liquidity-guard-bot/internal/models"
)

const (
	pollInterval   = 10 * time.Second
	slowThreshold  = 0.03  // 3% drawdown → SLOW
	pauseThreshold = 0.05  // 5% drawdown → PAUSE (default; overridden per-bot)
)

func main() {
	uri    := envOrDefault("MONGO_URI", "mongodb://localhost:27017")
	dbName := envOrDefault("MONGO_DB",  "liquidity_guard")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	client, err := mongoClient.NewClient(ctx, uri, dbName)
	if err != nil {
		log.Fatalf("watchdog: MongoDB connect: %v", err)
	}
	defer func() { _ = client.Disconnect(context.Background()) }()

	log.Printf("watchdog: started — poll interval %s", pollInterval)

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("watchdog: shutting down")
			return
		case <-ticker.C:
			runCycle(ctx, client)
		}
	}
}

// runCycle fetches all active sessions and updates their risk state.
func runCycle(ctx context.Context, client *mongoClient.Client) {
	sessions := client.Collection(models.CollActiveSessions)
	configs  := client.Collection(models.CollBotConfigs)

	cursor, err := sessions.Find(ctx, bson.M{"state": bson.M{"$ne": models.BotStateStopped}})
	if err != nil {
		log.Printf("watchdog: find sessions: %v", err)
		return
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var sess models.BotSession
		if err := cursor.Decode(&sess); err != nil {
			log.Printf("watchdog: decode session: %v", err)
			continue
		}

		// Fetch per-bot risk thresholds from BotConfigs.
		var cfg models.BotConfig
		if err := configs.FindOne(ctx, bson.M{"bot_id": sess.BotID}).Decode(&cfg); err != nil {
			log.Printf("watchdog: config for %s: %v", sess.BotID, err)
			continue
		}

		newRiskState := computeRiskState(sess, cfg)
		if newRiskState == sess.RiskState {
			continue // no change — skip write
		}

		_, err := sessions.UpdateOne(ctx,
			bson.M{"bot_id": sess.BotID},
			bson.M{"$set": bson.M{
				"risk_state": newRiskState,
				"updated_at": time.Now().UTC(),
			}},
			options.Update().SetUpsert(false),
		)
		if err != nil {
			log.Printf("watchdog: update risk_state for %s: %v", sess.BotID, err)
			continue
		}
		log.Printf("watchdog: bot %s risk_state %s → %s (drawdown=%s)",
			sess.BotID, sess.RiskState, newRiskState, sess.DrawdownPct24h)
	}
}

// computeRiskState applies the FSM: NORMAL → SLOW → PAUSED based on drawdown.
func computeRiskState(sess models.BotSession, cfg models.BotConfig) models.BotState {
	drawdown := sess.DrawdownPct24h

	pauseLimit := cfg.Risk.MaxDrawdownPct24h
	if pauseLimit.IsZero() {
		pauseLimit = decimal.NewFromFloat(pauseThreshold)
	}
	// SLOW threshold is half the pause limit (or 3% floor).
	slowLimit := pauseLimit.Div(decimal.NewFromInt(2))
	if slowLimit.LessThan(decimal.NewFromFloat(slowThreshold)) {
		slowLimit = decimal.NewFromFloat(slowThreshold)
	}

	switch {
	case drawdown.GreaterThanOrEqual(pauseLimit):
		return models.BotStatePaused
	case drawdown.GreaterThanOrEqual(slowLimit):
		return models.BotStateSlow
	default:
		return models.BotStateRunning
	}
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
