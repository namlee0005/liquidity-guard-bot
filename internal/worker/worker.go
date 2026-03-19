package worker

import (
	"errors"
	"sync"
)

// WorkerState represents the FSM state of a bot worker.
type WorkerState int

const (
	StateNormal WorkerState = iota
	StateSlow
	StatePause
)

func (s WorkerState) String() string {
	switch s {
	case StateNormal:
		return "NORMAL"
	case StateSlow:
		return "SLOW"
	case StatePause:
		return "PAUSE"
	default:
		return "UNKNOWN"
	}
}

// validTransitions defines allowed FSM edges.
var validTransitions = map[WorkerState][]WorkerState{
	StateNormal: {StateSlow, StatePause},
	StateSlow:   {StateNormal, StatePause},
	StatePause:  {StateNormal},
}

// Worker holds the mutable state for a single bot worker goroutine.
type Worker struct {
	mu    sync.RWMutex
	state WorkerState
	botID string
}

// NewWorker returns a Worker initialised to StateNormal.
func NewWorker(botID string) *Worker {
	return &Worker{botID: botID, state: StateNormal}
}

// State returns the current FSM state (safe for concurrent reads).
func (w *Worker) State() WorkerState {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.state
}

// Transition moves the worker to the next state if the transition is valid.
// Returns ErrInvalidTransition if the edge is not in validTransitions.
func (w *Worker) Transition(next WorkerState) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	allowed := validTransitions[w.state]
	for _, s := range allowed {
		if s == next {
			w.state = next
			return nil
		}
	}
	return ErrInvalidTransition
}

// CanPlaceOrders returns true only when the worker is in StateNormal or StateSlow.
func (w *Worker) CanPlaceOrders() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.state != StatePause
}

// BotID returns the immutable bot identifier.
func (w *Worker) BotID() string { return w.botID }

var ErrInvalidTransition = errors.New("invalid state transition")