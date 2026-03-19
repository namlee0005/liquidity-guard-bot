// Package orchestrator manages the lifecycle of all BotWorkers.
// It owns the Bot Registry and routes state transitions (Pause/Resume/Stop).
package orchestrator

import (
	"fmt"
	"sync"

	"liquidity-guard-bot/internal/models"
	"liquidity-guard-bot/internal/worker"
	"liquidity-guard-bot/pkg/exchange"
)

// AdapterFactory constructs an ExchangeAdapter from a BotConfig.
// Injected so the Orchestrator doesn't import concrete exchange packages.
type AdapterFactory func(cfg models.BotConfig) (exchange.ExchangeAdapter, error)

// entry holds a running Worker and its associated adapter.
type entry struct {
	w       *worker.Worker
	adapter exchange.ExchangeAdapter
}

// Orchestrator is the Bot Registry: a sharded map of botID → Worker.
// Sharding (16 buckets) reduces lock contention under concurrent gRPC calls.
const shards = 16

type shard struct {
	mu      sync.RWMutex
	workers map[string]*entry
}

// Orchestrator manages the full set of active bot workers.
type Orchestrator struct {
	shards  [shards]shard
	factory AdapterFactory
}

// New creates a ready-to-use Orchestrator.
func New(factory AdapterFactory) *Orchestrator {
	o := &Orchestrator{factory: factory}
	for i := range o.shards {
		o.shards[i].workers = make(map[string]*entry)
	}
	return o
}

func (o *Orchestrator) bucket(botID string) *shard {
	h := uint32(2166136261)
	for i := 0; i < len(botID); i++ {
		h ^= uint32(botID[i])
		h *= 16777619
	}
	return &o.shards[h%shards]
}

// StartBot registers a new worker for the given config.
// Returns an error if a worker for botID is already registered.
func (o *Orchestrator) StartBot(cfg models.BotConfig) error {
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

	b.workers[cfg.BotID] = &entry{
		w:       worker.NewWorker(cfg.BotID),
		adapter: adapter,
	}
	return nil
}

// PauseBot transitions the named worker to StatePause.
func (o *Orchestrator) PauseBot(botID string) error {
	e, err := o.get(botID)
	if err != nil {
		return err
	}
	return e.w.Transition(worker.StatePause)
}

// ResumeBot transitions the named worker from StatePause to StateNormal.
func (o *Orchestrator) ResumeBot(botID string) error {
	e, err := o.get(botID)
	if err != nil {
		return err
	}
	return e.w.Transition(worker.StateNormal)
}

// StopBot removes the worker from the registry.
func (o *Orchestrator) StopBot(botID string) error {
	b := o.bucket(botID)
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, exists := b.workers[botID]; !exists {
		return fmt.Errorf("orchestrator: bot %s not found", botID)
	}
	delete(b.workers, botID)
	return nil
}

// BotState returns the current WorkerState of a bot as a models.BotState string.
func (o *Orchestrator) BotState(botID string) (models.BotState, error) {
	e, err := o.get(botID)
	if err != nil {
		return "", err
	}
	switch e.w.State() {
	case worker.StateSlow:
		return models.BotStateSlow, nil
	case worker.StatePause:
		return models.BotStatePaused, nil
	default:
		return models.BotStateRunning, nil
	}
}

// Adapter returns the ExchangeAdapter registered for the given botID.
// Used by the trading engine to fetch books and place orders.
func (o *Orchestrator) Adapter(botID string) (exchange.ExchangeAdapter, error) {
	e, err := o.get(botID)
	if err != nil {
		return nil, err
	}
	return e.adapter, nil
}

// ActiveBotIDs returns a snapshot of all currently registered bot IDs.
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

// StopAll removes all workers from the registry. Called on graceful shutdown.
func (o *Orchestrator) StopAll() {
	for i := range o.shards {
		o.shards[i].mu.Lock()
		for id := range o.shards[i].workers {
			delete(o.shards[i].workers, id)
		}
		o.shards[i].mu.Unlock()
	}
}

func (o *Orchestrator) get(botID string) (*entry, error) {
	b := o.bucket(botID)
	b.mu.RLock()
	e, exists := b.workers[botID]
	b.mu.RUnlock()
	if !exists {
		return nil, fmt.Errorf("orchestrator: bot %s not found", botID)
	}
	return e, nil
}
