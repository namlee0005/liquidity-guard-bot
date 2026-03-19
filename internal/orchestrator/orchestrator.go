// Package orchestrator manages the lifecycle of all BotWorkers.
// It owns the Bot Registry, starts/stops goroutines, and routes gRPC signals.
package orchestrator

import (
	"context"
	"fmt"
	"sync"
	"time"

	"liquidity-guard-bot/internal/models"
	"liquidity-guard-bot/internal/worker"
	"liquidity-guard-bot/pkg/exchange"
)

// AdapterFactory constructs an ExchangeAdapter from a BotConfig.
// Injected so the Orchestrator doesn't import concrete exchange packages.
type AdapterFactory func(cfg models.BotConfig) (exchange.ExchangeAdapter, error)

// SeqGen generates unique ClientOID strings.
type SeqGen func() string

// entry holds a running Worker and its cancel function.
type entry struct {
	worker *worker.Worker
	cancel context.CancelFunc
	doneCh <-chan struct{} // closed when the goroutine exits
}

// Orchestrator is the Bot Registry: a sharded map of botID → running Worker.
// Sharding (16 buckets) reduces lock contention when many bots update concurrently.
const shards = 16

type shard struct {
	mu      sync.RWMutex
	workers map[string]*entry
}

// Orchestrator manages the full set of active BotWorkers.
type Orchestrator struct {
	shards      [shards]shard
	factory     AdapterFactory
	seqGen      SeqGen
	telemetryCh chan<- worker.Telemetry
}

// New creates a ready-to-use Orchestrator.
func New(factory AdapterFactory, seqGen SeqGen, telemetryCh chan<- worker.Telemetry) *Orchestrator {
	o := &Orchestrator{
		factory:     factory,
		seqGen:      seqGen,
		telemetryCh: telemetryCh,
	}
	for i := range o.shards {
		o.shards[i].workers = make(map[string]*entry)
	}
	return o
}

func (o *Orchestrator) bucket(botID string) *shard {
	// FNV-1a hash for even distribution without importing a crypto package.
	h := uint32(2166136261)
	for i := 0; i < len(botID); i++ {
		h ^= uint32(botID[i])
		h *= 16777619
	}
	return &o.shards[h%shards]
}

// StartBot creates and registers a new Worker goroutine for the given config.
// Returns ErrAlreadyExists if a worker for botID is already running.
func (o *Orchestrator) StartBot(parentCtx context.Context, cfg models.BotConfig) error {
	b := o.bucket(cfg.BotID)
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, exists := b.workers[cfg.BotID]; exists {
		return fmt.Errorf("orchestrator: bot %s already running", cfg.BotID)
	}

	adapter, err := o.factory(cfg)
	if err != nil {
		return fmt.Errorf("orchestrator: adapter for %s: %w", cfg.BotID, err)
	}

	wCfg := worker.Config{
		BotID:        cfg.BotID,
		TradingPair:  cfg.TradingPair,
		OrderLayers:  cfg.OrderLayers,
		LayerSize:    cfg.LayerSizeBase,
		TickInterval: time.Second,
		Bounds: worker.SpreadBoundsFromModel(cfg.Spread),
	}

	w, err := worker.New(wCfg, adapter, o.telemetryCh, o.seqGen)
	if err != nil {
		return fmt.Errorf("orchestrator: worker init for %s: %w", cfg.BotID, err)
	}

	ctx, cancel := context.WithCancel(parentCtx)
	doneCh := make(chan struct{})

	go func() {
		defer close(doneCh)
		w.Run(ctx)
	}()

	b.workers[cfg.BotID] = &entry{worker: w, cancel: cancel, doneCh: doneCh}
	return nil
}

// PauseBot sends a Pause signal to the named worker.
func (o *Orchestrator) PauseBot(botID string) error {
	return o.send(botID, worker.SignalPause)
}

// ResumeBot sends a Resume signal to the named worker.
func (o *Orchestrator) ResumeBot(botID string) error {
	return o.send(botID, worker.SignalResume)
}

// StopBot signals the worker to stop, waits for the goroutine to exit, then
// removes it from the registry.
func (o *Orchestrator) StopBot(botID string) error {
	b := o.bucket(botID)
	b.mu.Lock()
	e, exists := b.workers[botID]
	if !exists {
		b.mu.Unlock()
		return fmt.Errorf("orchestrator: bot %s not found", botID)
	}
	// Remove from registry before releasing lock so new StartBot can reuse the ID.
	delete(b.workers, botID)
	b.mu.Unlock()

	e.worker.Send(worker.SignalStop)
	e.cancel()
	<-e.doneCh
	return nil
}

// UpdateConfig delivers a live config update to the named worker.
func (o *Orchestrator) UpdateConfig(botID string, cfg models.BotConfig) error {
	b := o.bucket(botID)
	b.mu.RLock()
	e, exists := b.workers[botID]
	b.mu.RUnlock()
	if !exists {
		return fmt.Errorf("orchestrator: bot %s not found", botID)
	}
	e.worker.UpdateConfig(worker.Config{
		BotID:        cfg.BotID,
		TradingPair:  cfg.TradingPair,
		OrderLayers:  cfg.OrderLayers,
		LayerSize:    cfg.LayerSizeBase,
		TickInterval: time.Second,
		Bounds:       worker.SpreadBoundsFromModel(cfg.Spread),
	})
	return nil
}

// BotState returns the current state of a bot worker.
func (o *Orchestrator) BotState(botID string) (models.BotState, error) {
	b := o.bucket(botID)
	b.mu.RLock()
	e, exists := b.workers[botID]
	b.mu.RUnlock()
	if !exists {
		return "", fmt.Errorf("orchestrator: bot %s not found", botID)
	}
	return e.worker.State(), nil
}

// ActiveBotIDs returns a snapshot of all currently running bot IDs.
func (o *Orchestrator) ActiveBotIDs() []string {
	var ids []string
	for i := range o.shards {
		o.shards[i].mu.RLock()
		for id := range o.shards[i].workers {
			ids = append(ids, id)
		}
		o.shards[i].mu.RUnlock()
	}
	return ids
}

// StopAll stops every running worker. Used on graceful shutdown.
func (o *Orchestrator) StopAll() {
	ids := o.ActiveBotIDs()
	var wg sync.WaitGroup
	for _, id := range ids {
		id := id
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = o.StopBot(id)
		}()
	}
	wg.Wait()
}

func (o *Orchestrator) send(botID string, sig worker.Signal) error {
	b := o.bucket(botID)
	b.mu.RLock()
	e, exists := b.workers[botID]
	b.mu.RUnlock()
	if !exists {
		return fmt.Errorf("orchestrator: bot %s not found", botID)
	}
	e.worker.Send(sig)
	return nil
}
