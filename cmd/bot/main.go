package main

import (
	"context"
	"log"
	"os"
	"time"

	mongoClient "github.com/namlee0005/liquidity-guard-bot/pkg/db/mongo"
	maserrors "github.com/namlee0005/liquidity-guard-bot/pkg/errors"
)

func main() {
	uri := envOrDefault("MONGO_URI", "mongodb://localhost:27017")
	dbName := envOrDefault("MONGO_DB", "liquidity_guard")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	log.Printf("connecting to MongoDB at %s (db=%s)", uri, dbName)

	client, err := mongoClient.NewClient(ctx, uri, dbName)
	if err != nil {
		if maserrors.Is(err, maserrors.ErrCodeDB) {
			log.Fatalf("DB init failed: %v", err)
		}
		log.Fatalf("unexpected error: %v", err)
	}
	defer func() {
		if err := client.Disconnect(context.Background()); err != nil {
			log.Printf("disconnect error: %v", err)
		}
	}()

	if err := client.Ping(ctx); err != nil {
		log.Fatalf("ping failed: %v", err)
	}

	log.Println("MongoDB connection OK — Liquidity Guard Bot scaffold ready")
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
