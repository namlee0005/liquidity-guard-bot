package worker_test

// FAST (<1s) — no I/O, pure FSM

import (
	"sync"
	"testing"
)

// ---------------------------------------------------------------------------
// Initial state
// ---------------------------------------------------------------------------

func TestNewWorker_InitialStateIsNormal(t *testing.T) {
	// FAST (<1s)
	w := NewWorker("bot-001")
	if w.State() != StateNormal {
		t.Errorf("expected NORMAL, got %s", w.State())
	}
}

func TestNewWorker_BotIDPreserved(t *testing.T) {
	// FAST (<1s)
	w := NewWorker("bot-xyz")
	if w.BotID() != "bot-xyz" {
		t.Errorf("expected bot-xyz, got %s", w.BotID())
	}
}

// ---------------------------------------------------------------------------
// Valid transitions
// ---------------------------------------------------------------------------

func TestWorker_Transition_NormalToSlow(t *testing.T) {
	// FAST (<1s)
	w := NewWorker("bot-001")
	if err := w.Transition(StateSlow); err != nil {
		t.Errorf("NORMAL→SLOW must be allowed: %v", err)
	}
}

func TestWorker_Transition_NormalToPause(t *testing.T) {
	// FAST (<1s)
	w := NewWorker("bot-001")
	if err := w.Transition(StatePause); err != nil {
		t.Errorf("NORMAL→PAUSE must be allowed: %v", err)
	}
}

func TestWorker_Transition_SlowToNormal(t *testing.T) {
	// FAST (<1s)
	w := NewWorker("bot-001")
	_ = w.Transition(StateSlow)
	if err := w.Transition(StateNormal); err != nil {
		t.Errorf("SLOW→NORMAL must be allowed: %v", err)
	}
}

func TestWorker_Transition_SlowToPause(t *testing.T) {
	// FAST (<1s)
	w := NewWorker("bot-001")
	_ = w.Transition(StateSlow)
	if err := w.Transition(StatePause); err != nil {
		t.Errorf("SLOW→PAUSE must be allowed: %v", err)
	}
}

func TestWorker_Transition_PauseToNormal(t *testing.T) {
	// FAST (<1s)
	w := NewWorker("bot-001")
	_ = w.Transition(StatePause)
	if err := w.Transition(StateNormal); err != nil {
		t.Errorf("PAUSE→NORMAL must be allowed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Invalid transitions
// ---------------------------------------------------------------------------

func TestWorker_Transition_NormalToNormalIsInvalid(t *testing.T) {
	// FAST (<1s)
	w := NewWorker("bot-001")
	err := w.Transition(StateNormal)
	if err != ErrInvalidTransition {
		t.Errorf("NORMAL→NORMAL must be rejected, got: %v", err)
	}
}

func TestWorker_Transition_PauseToSlowIsInvalid(t *testing.T) {
	// FAST (<1s)
	// PAUSE must only resume to NORMAL; jumping to SLOW skips safety checks.
	w := NewWorker("bot-001")
	_ = w.Transition(StatePause)
	err := w.Transition(StateSlow)
	if err != ErrInvalidTransition {
		t.Errorf("PAUSE→SLOW must be rejected, got: %v", err)
	}
}

func TestWorker_Transition_InvalidLeavesStatUnchanged(t *testing.T) {
	// FAST (<1s)
	w := NewWorker("bot-001")
	_ = w.Transition(StatePause)
	_ = w.Transition(StateSlow) // invalid — must be no-op
	if w.State() != StatePause {
		t.Errorf("state must remain PAUSE after invalid transition, got %s", w.State())
	}
}

// ---------------------------------------------------------------------------
// CanPlaceOrders gate
// ---------------------------------------------------------------------------

func TestWorker_CanPlaceOrders_TrueInNormal(t *testing.T) {
	// FAST (<1s)
	w := NewWorker("bot-001")
	if !w.CanPlaceOrders() {
		t.Error("NORMAL state must allow order placement")
	}
}

func TestWorker_CanPlaceOrders_TrueInSlow(t *testing.T) {
	// FAST (<1s)
	w := NewWorker("bot-001")
	_ = w.Transition(StateSlow)
	if !w.CanPlaceOrders() {
		t.Error("SLOW state must still allow order placement (throttled, not halted)")
	}
}

func TestWorker_CanPlaceOrders_FalseInPause(t *testing.T) {
	// FAST (<1s)
	// This is the critical risk gate: PAUSED bots must never place orders.
	w := NewWorker("bot-001")
	_ = w.Transition(StatePause)
	if w.CanPlaceOrders() {
		t.Error("PAUSE state must block order placement — risk control failure")
	}
}

// ---------------------------------------------------------------------------
// Concurrent access — race detector will catch violations
// ---------------------------------------------------------------------------

func TestWorker_ConcurrentStateReads_NoRace(t *testing.T) {
	// FAST (<1s) — run with: go test -race ./internal/worker/...
	w := NewWorker("bot-race")
	var wg sync.WaitGroup

	// 50 readers + 1 writer in parallel
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = w.State()
			_ = w.CanPlaceOrders()
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = w.Transition(StateSlow)
	}()

	wg.Wait()
	// No assertion needed — the race detector is the assertion.
}

func TestWorker_ConcurrentTransitions_StateIsConsistent(t *testing.T) {
	// FAST (<1s)
	// After concurrent transitions the worker must be in a valid state.
	w := NewWorker("bot-concurrent")
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				_ = w.Transition(StateSlow)
			} else {
				_ = w.Transition(StateNormal)
			}
		}(i)
	}

	wg.Wait()

	s := w.State()
	if s != StateNormal && s != StateSlow && s != StatePause {
		t.Errorf("worker ended in invalid state: %d", s)
	}
}