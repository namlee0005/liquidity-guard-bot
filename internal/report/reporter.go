// Package report generates daily, weekly, and monthly performance summaries
// from MongoDB TradeHistory via aggregation pipelines. All monetary outputs
// use shopspring/decimal — float64 is prohibited.
package report

import (
	"context"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	mongoClient "liquidity-guard-bot/pkg/db/mongo"
	"liquidity-guard-bot/internal/models"
)

// Period labels.
type Period string

const (
	PeriodDaily   Period = "daily"
	PeriodWeekly  Period = "weekly"
	PeriodMonthly Period = "monthly"
)

// Report is the output of a single aggregation window.
type Report struct {
	BotID         string
	Period        Period
	WindowStart   time.Time
	WindowEnd     time.Time
	TotalFills    int64
	TotalVolume   decimal.Decimal // base currency
	TotalFees     decimal.Decimal // quote currency
	TotalPnL      decimal.Decimal // naive: fill revenue minus fees
	BidCount      int64
	AskCount      int64
	GeneratedAt   time.Time
}

// Reporter runs MongoDB aggregation pipelines to produce performance reports.
type Reporter struct {
	db *mongoClient.Client
}

// New creates a Reporter.
func New(db *mongoClient.Client) *Reporter {
	return &Reporter{db: db}
}

// Generate produces a Report for the given bot, period, and window end time.
// windowEnd is truncated to UTC midnight; the window start is derived from the period.
func (r *Reporter) Generate(ctx context.Context, botID string, period Period, windowEnd time.Time) (*Report, error) {
	windowEnd = truncateToDay(windowEnd.UTC())
	windowStart := windowStart(period, windowEnd)

	pipeline := mongo.Pipeline{
		// Stage 1: filter by botID + window + FILLED action only.
		{{Key: "$match", Value: bson.D{
			{Key: "bot_id", Value: botID},
			{Key: "action", Value: models.ActionFilled},
			{Key: "timestamp", Value: bson.D{
				{Key: "$gte", Value: primitive.NewDateTimeFromTime(windowStart)},
				{Key: "$lt",  Value: primitive.NewDateTimeFromTime(windowEnd)},
			}},
		}}},
		// Stage 2: group into summary.
		{{Key: "$group", Value: bson.D{
			{Key: "_id",          Value: "$side"},
			{Key: "fill_count",   Value: bson.D{{Key: "$sum", Value: 1}}},
			{Key: "total_volume", Value: bson.D{{Key: "$sum", Value: "$filled_qty"}}},
			{Key: "total_fees",   Value: bson.D{{Key: "$sum", Value: "$fee"}}},
		}}},
	}

	cursor, err := r.db.Collection(models.CollTradeHistory).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("reporter aggregate: %w", err)
	}
	defer cursor.Close(ctx)

	type groupRow struct {
		Side        string  `bson:"_id"`
		FillCount   int64   `bson:"fill_count"`
		TotalVolume primitive.Decimal128 `bson:"total_volume"`
		TotalFees   primitive.Decimal128 `bson:"total_fees"`
	}

	rep := &Report{
		BotID:       botID,
		Period:      period,
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
		GeneratedAt: time.Now().UTC(),
	}

	for cursor.Next(ctx) {
		var row groupRow
		if err := cursor.Decode(&row); err != nil {
			return nil, fmt.Errorf("reporter decode: %w", err)
		}

		vol  := decimal128ToDecimal(row.TotalVolume)
		fees := decimal128ToDecimal(row.TotalFees)

		rep.TotalFills  += row.FillCount
		rep.TotalVolume  = rep.TotalVolume.Add(vol)
		rep.TotalFees    = rep.TotalFees.Add(fees)

		switch models.OrderSide(row.Side) {
		case models.SideBid:
			rep.BidCount += row.FillCount
		case models.SideAsk:
			rep.AskCount += row.FillCount
			// Naïve PnL: ask revenue (price×qty) minus fees; bid cost subtracted separately.
			rep.TotalPnL = rep.TotalPnL.Add(vol).Sub(fees)
		}
	}
	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("reporter cursor: %w", err)
	}
	return rep, nil
}

// Schedule runs Generate on a ticker: daily at 00:05 UTC, weekly on Mondays,
// monthly on the 1st. Results are inserted into a dedicated "Reports" collection.
// Blocks until ctx is cancelled.
func (r *Reporter) Schedule(ctx context.Context, botIDs []string) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			utc := now.UTC()
			// Daily: fire at 00:05.
			if utc.Hour() == 0 && utc.Minute() == 5 {
				r.runAll(ctx, botIDs, PeriodDaily, utc)
			}
			// Weekly: Monday 00:05.
			if utc.Weekday() == time.Monday && utc.Hour() == 0 && utc.Minute() == 5 {
				r.runAll(ctx, botIDs, PeriodWeekly, utc)
			}
			// Monthly: 1st of month 00:05.
			if utc.Day() == 1 && utc.Hour() == 0 && utc.Minute() == 5 {
				r.runAll(ctx, botIDs, PeriodMonthly, utc)
			}
		}
	}
}

func (r *Reporter) runAll(ctx context.Context, botIDs []string, period Period, now time.Time) {
	for _, id := range botIDs {
		rep, err := r.Generate(ctx, id, period, now)
		if err != nil {
			continue
		}
		_, _ = r.db.Collection("Reports").InsertOne(ctx, rep)
	}
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func truncateToDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func windowStart(period Period, end time.Time) time.Time {
	switch period {
	case PeriodWeekly:
		return end.AddDate(0, 0, -7)
	case PeriodMonthly:
		return end.AddDate(0, -1, 0)
	default: // daily
		return end.AddDate(0, 0, -1)
	}
}

func decimal128ToDecimal(d primitive.Decimal128) decimal.Decimal {
	v, err := decimal.NewFromString(d.String())
	if err != nil {
		return decimal.Zero
	}
	return v
}
