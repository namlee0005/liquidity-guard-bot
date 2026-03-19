# Liquidity Guard Bot — Implementation Tasks
# Revision 3: Go + MongoDB (Single DB) + Multi-Exchange
# Generated: 2026-03-19

## Phase 1 — Project Scaffold & MongoDB Base
- [ ] **T1.1** Init Go module `github.com/namlee0005/liquidity-guard-bot`
- [ ] **T1.2** Setup MongoDB 7.0 Docker Compose service
- [ ] **T1.3** Implement `pkg/db/mongo`: connection pool, Decimal128 codec registration
- [ ] **T1.4** Define common error types (MASError hierarchy)

## Phase 2 — Data Models & Protobuf
- [ ] **T2.1** Define `bot.proto` for Management API (CRUD + Telemetry Stream)
- [ ] **T2.2** Generate Go gRPC code from proto
- [ ] **T2.3** Create `internal/models`: `BotConfig`, `BotSession`, `TradeRecord`, `AuditLog` using `shopspring/decimal`

## Phase 3 — Exchange Adapters (MEXC, Bybit, Gate, Kraken)
- [ ] **T3.1** Define `ExchangeAdapter` Go interface
- [ ] **T3.2** Implement Bybit adapter (WebSocket order book + REST orders)
- [ ] **T3.3** Implement MEXC adapter
- [ ] **T3.4** Implement Gate.io adapter
- [ ] **T3.5** Implement Kraken adapter

## Phase 4 — Trading Engine Core
- [ ] **T4.1** Implement `SpreadCalculator`: grid generation with inventory skew
- [ ] **T4.2** Implement `OrderManager`: diff logic, atomic cancel/replace
- [ ] **T4.3** Implement `DepthMonitor`: compliance check against depth requirements

## Phase 5 — Bot Orchestrator & Worker
- [ ] **T5.1** Implement `internal/orchestrator`: Bot Registry, goroutine management
- [ ] **T5.2** Implement `internal/worker`: Main loop (Tick), signal handling (Channels)
- [ ] **T5.3** Implement `InventoryTracker`: real-time balance and NAV computation

## Phase 6 — Risk Engine & Watchdog
- [ ] **T6.1** Implement `RiskWatchdog` separate service: Drawdown FSM logic
- [ ] **T6.2** Implement MongoDB status polling/signaling mechanism
- [ ] **T6.3** Wire circuit-breaker logic into Worker loop

## Phase 7 — Management API & Telemetry
- [ ] **T7.1** Implement gRPC Server: Lifecycle RPCs (Create/Pause/Update/Delete)
- [ ] **T7.2** Implement `TelemetryHub`: WebSocket/gRPC stream for order book & balance
- [ ] **T7.3** Implement `AuditLogger` repository (MongoDB with WriteConcern: Majority)

## Phase 8 — Reporting & Monitoring
- [ ] **T8.1** Implement `Reporter` service: Daily/Weekly/Monthly aggregations from MongoDB
- [ ] **T8.2** Setup Prometheus metrics exporter
- [ ] **T8.3** Setup Grafana dashboard boilerplate

## Phase 9 — Hardening & Final Integration
- [ ] **T9.1** Implement graceful shutdown (context cancellation, order cleanup)
- [ ] **T9.2** Setup GitHub Actions CI: lint, test, build
- [ ] **T9.3** Documentation: README.md, Deployment guide

---

## Invariants
- **Single DB:** All data must reside in MongoDB. No PostgreSQL or Redis.
- **Precision:** `float64` strictly prohibited for all monetary values.
- **Additive Doc:** History of spec/tasks preserved.
