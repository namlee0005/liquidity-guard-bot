// Package worker implements the per-bot goroutine lifecycle and main trading loop.
package worker

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shopspring/decimal"

	"github.com/namlee0005/liquidity-guard-bot/internal/engine"
	"github.com/namlee0005/liquidity-guard-bot/internal/models"
	"github.com/namlee0005/liquidity-guard-bot/pkg/exchange"
)

// Signal is an out-of-band command sent from the Orchestrator to a running Worker.
type Signal int

const (
	SignalPause  Signal = iota // stop placing orders, keep goroutine alive
	SignalResume               // resume from PAUSED state
	SignalStop                 // cancel orders and exit the goroutine
)

// Config holds the worker's operating parameters. Immutable after construction;
// live updates arrive via a new Config on the signalCh.
type Config struct {
	BotID       string
	TradingPair string
	Bounds      engine.SpreadBounds
	OrderLayers int
	LayerSize   decimal.Decimal
	TickInterval time.Duration // how often to reconcile orders; 0 = 1s default
}

// InventoryTracker computes NAV and 24-hour drawdown from filled trade records.
// It is owned exclusively by the Worker goroutine — no lock needed.
type InventoryTracker struct {
	BaseAsset    string
	QuoteAsset   string
	BaseBalance  decimal.Decimal
	QuoteBalance decimal.Decimal
	NAVBaseline  decimal.Decimal // NAV at the start of the 24h window
	WindowStart  time.Time
}

// NAV returns the current net asset value in quote currency given the last mid-price.
func (it *InventoryTracker) NAV(midPrice decimal.Decimal) decimal.Decimal {
	return it.BaseBalance.Mul(midPrice).Add(it.QuoteBalance)
}

// DrawdownPct returns the fractional drawdown from the 24h baseline NAV.
// Resets the baseline if the window has elapsed.
func (it *InventoryTracker) DrawdownPct(midPrice decimal.Decimal) decimal.Decimal {
	if it.NAVBaseline.IsZero() {
		return decimal.Zero
	}
	nav := it.NAV(midPrice)
	return it.NAVBaseline.Sub(nav).Div(it.NAVBaseline)
}

// ResetWindowIfExpired resets the 24h drawdown baseline when the window expires.
func (it *InventoryTracker) ResetWindowIfExpired(midPrice decimal.Decimal, now time.Time) {
	if now.Sub(it.WindowStart) >= 24*time.Hour {
		it.NAVBaseline = it.NAV(midPrice)
		it.WindowStart = now
	}
}

// Worker is a single bot goroutine managing orders for one (exchange, trading pair).
type Worker struct {
	cfg      Config
	adapter  exchange.ExchangeAdapter
	manager  *engine.OrderManager
	calc     *engine.SpreadCalculator
	inventory InventoryTracker

	signalCh    chan Signal     // receives Pause/Resume/Stop from Orchestrator
	configCh    chan Config     // receives live config updates
	telemetryCh chan<- Telemetry // emits snapshots to the gRPC streamer

	state    atomic.Int32 // models.BotState encoded as int32
	stopOnce sync.Once
	cancel   context.CancelFunc
}

// Telemetry is a lightweight snapshot emitted to the gRPC telemetry stream.
type Telemetry struct {
	BotID     string
	State     models.BotState
	OrderBook exchange.OrderBook
	Balances  []exchange.Balance
	NAV       decimal.Decimal
	Drawdown  decimal.Decimal
	Timestamp time.Time
}

const (
	stateRunning int32 = iota
	stateSlow
	statePaused
	stateStopped
)

// New constructs a Worker. seqGen must produce unique ClientOID strings (e.g. UUIDs).
func New(
	cfg Config,
	adapter exchange.ExchangeAdapter,
	telemetryCh chan<- Telemetry,
	seqGen func() string,
) (*Worker, error) {
	calc, err := engine.NewSpreadCalculator(cfg.Bounds, cfg.OrderLayers, cfg.LayerSize)
	if err != nil {
		return nil, fmt.Errorf("Worker[%s] bad config: %w", cfg.BotID, err)
	}

	mgr := engine.NewOrderManager(adapter, cfg.BotID, cfg.TradingPair, seqGen)

	w := &Worker{
		cfg:         cfg,
		adapter:     adapter,
		manager:     mgr,
		calc:        calc,
		signalCh:    make(chan Signal, 4),
		configCh:    make(chan Config, 2),
		telemetryCh: telemetryCh,
	}
	w.state.Store(stateRunning)
	return w, nil
}

// Send delivers an out-of-band signal to the worker (non-blocking; drops if full).
func (w *Worker) Send(sig Signal) {
	select {
	case w.signalCh <- sig:
	default:
	}
}

// UpdateConfig delivers a live config replacement (non-blocking).
func (w *Worker) UpdateConfig(cfg Config) {
	select {
	case w.configCh <- cfg:
	default:
	}
}

// State returns the current BotState.
func (w *Worker) State() models.BotState {
	switch w.state.Load() {
	case stateSlow:
		return models.BotStateSlow
	case statePaused:
		return models.BotStatePaused
	case stateStopped:
		return models.BotStateStopped
	default:
		return models.BotStateRunning
	}
}

// Run is the main goroutine entry point. Blocks until the worker stops.
// The caller must invoke this in a separate goroutine.
func (w *Worker) Run(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	w.cancel = cancel
	defer cancel()
	defer w.state.Store(stateStopped)

	tick := w.cfg.TickInterval
	if tick == 0 {
		tick = time.Second
	}

	// Open order book stream.
	bookCh, err := w.adapter.WatchOrderBook(ctx, w.cfg.TradingPair)
	if err != nil {
		// Non-retryable startup failure — log and exit.
		_ = fmt.Errorf("Worker[%s] WatchOrderBook: %w", w.cfg.BotID, err)
		return
	}

	ticker := time.NewTicker(tick)
	defer ticker.Stop()

	var lastBook exchange.OrderBook

	for {
		select {
		case <-ctx.Done():
			_ = w.manager.CancelAll(context.Background())
			return

		case sig := <-w.signalCh:
			w.handleSignal(ctx, sig)
			if w.state.Load() == stateStopped {
				return
			}

		case newCfg := <-w.configCh:
			w.applyConfig(newCfg)

		case book, ok := <-bookCh:
			if !ok {
				// Stream closed — attempt restart handled by Orchestrator watchdog.
				return
			}
			lastBook = book

		case <-ticker.C:
			if w.state.Load() == statePaused {
				continue
			}
			if lastBook.TradingPair == "" {
				continue // no book data yet
			}
			w.cycle(ctx, lastBook)
		}
	}
}

// cycle is one reconciliation tick: compute quotes → reconcile orders → emit telemetry.
func (w *Worker) cycle(ctx context.Context, book exchange.OrderBook) {
	quotes, err := w.calc.Compute(book)
	if err != nil {
		return // stale/crossed book; skip this tick
	}

	// SLOW state: only place innermost layer to reduce market footprint.
	if w.state.Load() == stateSlow {
		quotes = quotes[:1]
	}

	if err := w.manager.Reconcile(ctx, quotes); err != nil {
		return
	}

	// Update inventory.
	if len(book.Bids) > 0 && len(book.Asks) > 0 {
		mid := book.Bids[0].Price.Add(book.Asks[0].Price).Div(decimal.NewFromInt(2))
		now := time.Now().UTC()
		w.inventory.ResetWindowIfExpired(mid, now)

		nav := w.inventory.NAV(mid)
		drawdown := w.inventory.DrawdownPct(mid)

		// Emit telemetry (non-blocking).
		balances, _ := w.adapter.GetBalances(ctx)
		select {
		case w.telemetryCh <- Telemetry{
			BotID:     w.cfg.BotID,
			State:     w.State(),
			OrderBook: book,
			Balances:  balances,
			NAV:       nav,
			Drawdown:  drawdown,
			Timestamp: now,
		}:
		default:
		}
	}
}

func (w *Worker) handleSignal(ctx context.Context, sig Signal) {
	switch sig {
	case SignalPause:
		_ = w.manager.CancelAll(ctx)
		w.state.Store(statePaused)
	case SignalResume:
		if w.state.Load() == statePaused {
			w.state.Store(stateRunning)
		}
	case SignalStop:
		_ = w.manager.CancelAll(context.Background())
		w.state.Store(stateStopped)
		w.cancel()
	}
}

func (w *Worker) applyConfig(cfg Config) {
	calc, err := engine.NewSpreadCalculator(cfg.Bounds, cfg.OrderLayers, cfg.LayerSize)
	if err != nil {
		return // reject invalid config silently; caller should validate first
	}
	w.cfg = cfg
	w.calc = calc
}
