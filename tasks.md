# Liquidity Guard Bot — Implementation Tasks
# Revision 3: Go + MongoDB (Single DB) + Multi-Exchange
# Generated: 2026-03-19

## Phase 1 — Project Scaffold & MongoDB Base
- [x] **T1.1** Init Go module `github.com/namlee0005/liquidity-guard-bot`
- [x] **T1.2** Setup MongoDB 7.0 Docker Compose service
- [x] **T1.3** Implement `pkg/db/mongo`: connection pool, Decimal128 codec registration
- [x] **T1.4** Define common error types (MASError hierarchy)

## Phase 2 — Data Models & Protobuf
- [x] **T2.1** Define `bot.proto` for Management API (CRUD + Telemetry Stream)
- [x] **T2.2** Generate Go gRPC code from proto
- [x] **T2.3** Create `internal/models`: `BotConfig`, `BotSession`, `TradeRecord`, `AuditLog` using `shopspring/decimal`

## Phase 3 — Exchange Adapters (MEXC, Bybit, Gate, Kraken)
- [x] **T3.1** Define `ExchangeAdapter` Go interface
- [x] **T3.2** Implement Bybit adapter (WebSocket order book + REST orders)
- [x] **T3.3** Implement MEXC adapter
- [x] **T3.4** Implement Gate.io adapter
- [x] **T3.5** Implement Kraken adapter

## Phase 4 — Trading Engine Core
- [x] **T4.1** Implement `SpreadCalculator`: grid generation with inventory skew
- [x] **T4.2** Implement `OrderManager`: diff logic, atomic cancel/replace
- [x] **T4.3** Implement `DepthMonitor`: compliance check against depth requirements

## Phase 5 — Bot Orchestrator & Worker
- [x] **T5.1** Implement `internal/orchestrator`: Bot Registry, goroutine management
- [x] **T5.2** Implement `internal/worker`: Main loop (Tick), signal handling (Channels)
- [x] **T5.3** Implement `InventoryTracker`: real-time balance and NAV computation

## Phase 6 — Risk Engine & Watchdog
- [x] **T6.1** Implement `RiskWatchdog` separate service: Drawdown FSM logic
- [x] **T6.2** Implement MongoDB status polling/signaling mechanism
- [x] **T6.3** Wire circuit-breaker logic into Worker loop

## Phase 7 — Management API & Telemetry
- [x] **T7.1** Implement gRPC Server: Lifecycle RPCs (Create/Pause/Update/Delete)
- [x] **T7.2** Implement `TelemetryHub`: WebSocket/gRPC stream for order book & balance
- [x] **T7.3** Implement `AuditLogger` repository (MongoDB with WriteConcern: Majority)
- [x] **T7.4** Implement Repository Layer: Interfaces and MongoDB implementations for all collections.
- [x] **T7.5** Unit Tests: 14 tests for Repository layer and 22 tests for gRPC server.

## Phase 8 — Reporting & Monitoring
- [x] **T8.1** Implement `Reporter` service: Daily/Weekly/Monthly aggregations from MongoDB
- [x] **T8.2** Setup Prometheus metrics exporter
- [x] **T8.3** Setup Grafana dashboard boilerplate

## Phase 9 — Hardening & Final Integration
- [x] **T9.1** Implement graceful shutdown (context cancellation, order cleanup)
- [x] **T9.2** Setup GitHub Actions CI: lint, test, build
- [x] **T9.3** Documentation: README.md, Deployment guide

---

## Invariants
- **Single DB:** All data must reside in MongoDB. No PostgreSQL or Redis.
- **Precision:** `float64` strictly prohibited for all monetary values.
- **Additive Doc:** History of spec/tasks preserved.
