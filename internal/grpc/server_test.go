package grpcserver_test

// Tests use google.golang.org/grpc/test/bufconn — in-process gRPC, no TCP port.
// All tests: FAST (<1s).
// Run: go test -race ./internal/grpc/...

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	. "liquidity-guard-bot/internal/grpc"
	"liquidity-guard-bot/internal/repository"
	pb "liquidity-guard-bot/proto/control"
)

const bufSize = 1 << 20

// ─────────────────────────────────────────────
// Fake in-memory repositories (real behaviour, no mocks)
// ─────────────────────────────────────────────

type fakeConfigs struct{ m map[string]*repository.BotConfig }

func newFakeConfigs() *fakeConfigs { return &fakeConfigs{m: make(map[string]*repository.BotConfig)} }
func (f *fakeConfigs) Insert(_ context.Context, c *repository.BotConfig) error {
	f.m[c.BotID] = c; return nil
}
func (f *fakeConfigs) FindByBotID(_ context.Context, id string) (*repository.BotConfig, error) {
	c, ok := f.m[id]
	if !ok { return nil, repository.ErrNotFound }
	return c, nil
}
func (f *fakeConfigs) Update(_ context.Context, c *repository.BotConfig) error {
	if _, ok := f.m[c.BotID]; !ok { return repository.ErrNotFound }
	f.m[c.BotID] = c; return nil
}
func (f *fakeConfigs) Delete(_ context.Context, id string) error {
	if _, ok := f.m[id]; !ok { return repository.ErrNotFound }
	delete(f.m, id); return nil
}

type fakeSessions struct{ m map[string]*repository.ActiveSession }

func newFakeSessions() *fakeSessions {
	return &fakeSessions{m: make(map[string]*repository.ActiveSession)}
}
func (f *fakeSessions) Upsert(_ context.Context, s *repository.ActiveSession) error {
	f.m[s.BotID] = s; return nil
}
func (f *fakeSessions) FindByBotID(_ context.Context, id string) (*repository.ActiveSession, error) {
	s, ok := f.m[id]
	if !ok { return nil, repository.ErrNotFound }
	return s, nil
}
func (f *fakeSessions) SetState(_ context.Context, id string, st repository.BotState) error {
	s, ok := f.m[id]
	if !ok { return repository.ErrNotFound }
	s.State = st; return nil
}
func (f *fakeSessions) Delete(_ context.Context, id string) error {
	delete(f.m, id); return nil
}

type fakeAudits struct{ logs []repository.AuditLog }

func (f *fakeAudits) Insert(_ context.Context, l *repository.AuditLog) error {
	f.logs = append(f.logs, *l); return nil
}
func (f *fakeAudits) FindByBotID(_ context.Context, id string, _ int) ([]repository.AuditLog, error) {
	var out []repository.AuditLog
	for _, l := range f.logs {
		if l.BotID == id { out = append(out, l) }
	}
	return out, nil
}

// ─────────────────────────────────────────────
// Test harness
// ─────────────────────────────────────────────

type harness struct {
	configs  *fakeConfigs
	sessions *fakeSessions
	audits   *fakeAudits
	client   pb.ControlPlaneClient
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{
		configs:  newFakeConfigs(),
		sessions: newFakeSessions(),
		audits:   &fakeAudits{},
	}
	lis := bufconn.Listen(bufSize)
	srv := grpc.NewServer()
	pb.RegisterControlPlaneServer(srv, NewServer(h.configs, h.sessions, h.audits))
	go func() { _ = srv.Serve(lis) }()

	conn, err := grpc.DialContext(context.Background(), "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	h.client = pb.NewControlPlaneClient(conn)
	t.Cleanup(func() { conn.Close(); srv.Stop() })
	return h
}

func gctx() context.Context {
	c, _ := context.WithTimeout(context.Background(), 3*time.Second)
	return c
}

func (h *harness) create(t *testing.T, botID, exchange, symbol string) {
	t.Helper()
	_, err := h.client.CreateBot(gctx(), &pb.CreateBotRequest{
		BotId: botID, Exchange: exchange, Symbol: symbol, Actor: "test",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
}

// ─────────────────────────────────────────────
// CreateBot
// ─────────────────────────────────────────────

func TestCreateBot_ReturnsBotID(t *testing.T) {
	// FAST (<1s)
	h := newHarness(t)
	resp, err := h.client.CreateBot(gctx(), &pb.CreateBotRequest{BotId: "b1", Exchange: "mexc", Symbol: "BTCUSDT", Actor: "a"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.BotId != "b1" {
		t.Errorf("expected BotId='b1', got %q", resp.BotId)
	}
}

func TestCreateBot_ReturnsSuccessTrue(t *testing.T) {
	// FAST (<1s)
	h := newHarness(t)
	resp, _ := h.client.CreateBot(gctx(), &pb.CreateBotRequest{BotId: "b2", Exchange: "bybit", Symbol: "ETHUSDT", Actor: "a"})
	if !resp.Success {
		t.Error("CreateBot must return Success=true")
	}
}

func TestCreateBot_PersistsConfigInRepo(t *testing.T) {
	// FAST (<1s)
	h := newHarness(t)
	h.create(t, "b3", "gate", "SOLUSDT")
	cfg, err := h.configs.FindByBotID(gctx(), "b3")
	if err != nil {
		t.Fatalf("config not persisted: %v", err)
	}
	if cfg.Exchange != "gate" {
		t.Errorf("expected exchange='gate', got %q", cfg.Exchange)
	}
}

func TestCreateBot_InitialisesSessionToNormal(t *testing.T) {
	// FAST (<1s)
	h := newHarness(t)
	h.create(t, "b4", "kraken", "XBTUSDT")
	s, err := h.sessions.FindByBotID(gctx(), "b4")
	if err != nil {
		t.Fatalf("session not created: %v", err)
	}
	if s.State != repository.BotStateNormal {
		t.Errorf("new bot must start NORMAL, got %q", s.State)
	}
}

func TestCreateBot_WritesCreateAuditLog(t *testing.T) {
	// FAST (<1s)
	h := newHarness(t)
	h.create(t, "b5", "mexc", "ADAUSDT")
	logs, _ := h.audits.FindByBotID(gctx(), "b5", 10)
	if len(logs) == 0 || logs[0].Action != "create" {
		t.Errorf("expected audit action='create', got: %v", logs)
	}
}

func TestCreateBot_EmptyBotIDIsInvalidArgument(t *testing.T) {
	// FAST (<1s)
	h := newHarness(t)
	_, err := h.client.CreateBot(gctx(), &pb.CreateBotRequest{Exchange: "mexc", Symbol: "BTCUSDT"})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got: %v", err)
	}
}

func TestCreateBot_EmptyExchangeIsInvalidArgument(t *testing.T) {
	// FAST (<1s)
	h := newHarness(t)
	_, err := h.client.CreateBot(gctx(), &pb.CreateBotRequest{BotId: "bx", Symbol: "BTCUSDT"})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got: %v", err)
	}
}

func TestCreateBot_EmptySymbolIsInvalidArgument(t *testing.T) {
	// FAST (<1s)
	h := newHarness(t)
	_, err := h.client.CreateBot(gctx(), &pb.CreateBotRequest{BotId: "bx", Exchange: "mexc"})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got: %v", err)
	}
}

// ─────────────────────────────────────────────
// PauseBot
// ─────────────────────────────────────────────

func TestPauseBot_ReturnsSuccessTrue(t *testing.T) {
	// FAST (<1s)
	h := newHarness(t)
	h.create(t, "b6", "bybit", "BTCUSDT")
	resp, err := h.client.PauseBot(gctx(), &pb.PauseBotRequest{BotId: "b6", Actor: "watchdog"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Success {
		t.Error("PauseBot must return Success=true")
	}
}

func TestPauseBot_SetsSessionStateToPause(t *testing.T) {
	// FAST (<1s)
	h := newHarness(t)
	h.create(t, "b7", "bybit", "BTCUSDT")
	h.client.PauseBot(gctx(), &pb.PauseBotRequest{BotId: "b7", Actor: "risk"})
	s, _ := h.sessions.FindByBotID(gctx(), "b7")
	if s.State != repository.BotStatePause {
		t.Errorf("expected PAUSE, got %q", s.State)
	}
}

func TestPauseBot_WritesPauseAuditLog(t *testing.T) {
	// FAST (<1s)
	h := newHarness(t)
	h.create(t, "b8", "gate", "SOLBTC")
	h.client.PauseBot(gctx(), &pb.PauseBotRequest{BotId: "b8", Actor: "system"})
	logs, _ := h.audits.FindByBotID(gctx(), "b8", 10)
	count := 0
	for _, l := range logs { if l.Action == "pause" { count++ } }
	if count != 1 {
		t.Errorf("expected 1 pause audit log, got %d", count)
	}
}

func TestPauseBot_EmptyBotIDIsInvalidArgument(t *testing.T) {
	// FAST (<1s)
	h := newHarness(t)
	_, err := h.client.PauseBot(gctx(), &pb.PauseBotRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got: %v", err)
	}
}

func TestPauseBot_UnknownBotIDIsNotFound(t *testing.T) {
	// FAST (<1s)
	h := newHarness(t)
	_, err := h.client.PauseBot(gctx(), &pb.PauseBotRequest{BotId: "ghost"})
	if status.Code(err) != codes.NotFound {
		t.Errorf("expected NotFound, got: %v", err)
	}
}

// ─────────────────────────────────────────────
// DeleteBot
// ─────────────────────────────────────────────

func TestDeleteBot_ReturnsSuccessTrue(t *testing.T) {
	// FAST (<1s)
	h := newHarness(t)
	h.create(t, "b9", "mexc", "BTCUSDT")
	resp, err := h.client.DeleteBot(gctx(), &pb.DeleteBotRequest{BotId: "b9", Actor: "admin"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Success {
		t.Error("DeleteBot must return Success=true")
	}
}

func TestDeleteBot_RemovesConfigFromRepo(t *testing.T) {
	// FAST (<1s)
	h := newHarness(t)
	h.create(t, "b10", "mexc", "BTCUSDT")
	h.client.DeleteBot(gctx(), &pb.DeleteBotRequest{BotId: "b10", Actor: "admin"})
	_, err := h.configs.FindByBotID(gctx(), "b10")
	if !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("config must be gone after delete, got err: %v", err)
	}
}

func TestDeleteBot_WritesDeleteAuditLog(t *testing.T) {
	// FAST (<1s)
	h := newHarness(t)
	h.create(t, "b11", "kraken", "XBTUSDT")
	h.client.DeleteBot(gctx(), &pb.DeleteBotRequest{BotId: "b11", Actor: "admin"})
	logs, _ := h.audits.FindByBotID(gctx(), "b11", 10)
	count := 0
	for _, l := range logs { if l.Action == "delete" { count++ } }
	if count != 1 {
		t.Errorf("expected 1 delete audit log, got %d", count)
	}
}

func TestDeleteBot_UnknownBotIDIsNotFound(t *testing.T) {
	// FAST (<1s)
	h := newHarness(t)
	_, err := h.client.DeleteBot(gctx(), &pb.DeleteBotRequest{BotId: "ghost"})
	if status.Code(err) != codes.NotFound {
		t.Errorf("expected NotFound, got: %v", err)
	}
}

func TestDeleteBot_EmptyBotIDIsInvalidArgument(t *testing.T) {
	// FAST (<1s)
	h := newHarness(t)
	_, err := h.client.DeleteBot(gctx(), &pb.DeleteBotRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got: %v", err)
	}
}

// ─────────────────────────────────────────────
// UpdateConfig
// ─────────────────────────────────────────────

func TestUpdateConfig_ReturnsSuccessTrue(t *testing.T) {
	// FAST (<1s)
	h := newHarness(t)
	h.create(t, "b12", "mexc", "BTCUSDT")
	resp, err := h.client.UpdateConfig(gctx(), &pb.UpdateConfigRequest{BotId: "b12", Symbol: "ETHUSDT", Actor: "a"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Success {
		t.Error("UpdateConfig must return Success=true")
	}
}

func TestUpdateConfig_UpdatesSymbolInRepo(t *testing.T) {
	// FAST (<1s)
	h := newHarness(t)
	h.create(t, "b13", "mexc", "BTCUSDT")
	h.client.UpdateConfig(gctx(), &pb.UpdateConfigRequest{BotId: "b13", Symbol: "ADAUSDT", Actor: "a"})
	cfg, _ := h.configs.FindByBotID(gctx(), "b13")
	if cfg.Symbol != "ADAUSDT" {
		t.Errorf("expected symbol='ADAUSDT', got %q", cfg.Symbol)
	}
}

func TestUpdateConfig_WritesUpdateConfigAuditLog(t *testing.T) {
	// FAST (<1s)
	h := newHarness(t)
	h.create(t, "b14", "gate", "BTCUSDT")
	h.client.UpdateConfig(gctx(), &pb.UpdateConfigRequest{BotId: "b14", Symbol: "DOGEUSDT", Actor: "a"})
	logs, _ := h.audits.FindByBotID(gctx(), "b14", 10)
	count := 0
	for _, l := range logs { if l.Action == "update_config" { count++ } }
	if count != 1 {
		t.Errorf("expected 1 update_config audit log, got %d", count)
	}
}

func TestUpdateConfig_UnknownBotIDIsNotFound(t *testing.T) {
	// FAST (<1s)
	h := newHarness(t)
	_, err := h.client.UpdateConfig(gctx(), &pb.UpdateConfigRequest{BotId: "ghost", Symbol: "BTCUSDT"})
	if status.Code(err) != codes.NotFound {
		t.Errorf("expected NotFound, got: %v", err)
	}
}

func TestUpdateConfig_EmptyBotIDIsInvalidArgument(t *testing.T) {
	// FAST (<1s)
	h := newHarness(t)
	_, err := h.client.UpdateConfig(gctx(), &pb.UpdateConfigRequest{Symbol: "BTCUSDT"})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got: %v", err)
	}
}

// ─────────────────────────────────────────────
// Lifecycle simulation
// ─────────────────────────────────────────────

func TestLifecycle_FullCycleProducesFourAuditEntries(t *testing.T) {
	// FAST (<1s)
	h := newHarness(t)
	const id = "lifecycle-bot"
	h.client.CreateBot(gctx(), &pb.CreateBotRequest{BotId: id, Exchange: "mexc", Symbol: "BTCUSDT", Actor: "a"})
	h.client.PauseBot(gctx(), &pb.PauseBotRequest{BotId: id, Actor: "watchdog"})
	h.client.UpdateConfig(gctx(), &pb.UpdateConfigRequest{BotId: id, Symbol: "ETHUSDT", Actor: "a"})
	h.client.DeleteBot(gctx(), &pb.DeleteBotRequest{BotId: id, Actor: "a"})

	logs, _ := h.audits.FindByBotID(gctx(), id, 20)
	if len(logs) != 4 {
		t.Errorf("expected 4 audit entries for full lifecycle, got %d", len(logs))
	}
}

func TestLifecycle_DeletedBotConfigIsGone(t *testing.T) {
	// FAST (<1s)
	h := newHarness(t)
	const id = "gone-bot"
	h.create(t, id, "bybit", "BTCUSDT")
	h.client.DeleteBot(gctx(), &pb.DeleteBotRequest{BotId: id, Actor: "a"})
	_, err := h.configs.FindByBotID(gctx(), id)
	if !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("deleted config must not be retrievable, got: %v", err)
	}
}