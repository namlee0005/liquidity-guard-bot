# Implementation Plan — Liquidity Guard Bot

## Phase 1 — Project Scaffold & Config
**Milestone:** Repo structure, config, and Docker environment operational.

- [ ] 1.1 Create directory layout: `src/`, `tests/`, `config/`, `docker/`, `scripts/`, `reports/`
- [ ] 1.2 Implement `src/config.py` — Pydantic `Settings` with all env vars (`spread_min_pct`, `spread_max_pct`, `max_drawdown_pct`, `min_order_count`, etc.); all monetary fields as `Decimal`
- [ ] 1.3 Add `@model_validator` to enforce `spread_min < spread_target < spread_max`
- [ ] 1.4 Create `pyproject.toml` with full dependency list (ccxt[pro], asyncpg, redis, prometheus-client, aiohttp, apscheduler, pydantic-settings)
- [ ] 1.5 Create `.env.example` with all required vars — no secrets in code
- [ ] 1.6 Create `docker-compose.yml`: app + PostgreSQL + Redis + Prometheus + Grafana

---

## Phase 2 — Data Models
**Milestone:** All Pydantic models defined; no I/O code yet.

- [ ] 2.1 `src/models/order.py` — `OrderSide`, `OrderStatus`, `OrderQuote`, `PlacedOrder`, `OrderGrid` (all prices as `Decimal`)
- [ ] 2.2 `src/models/inventory.py` — `InventoryState` (with `skew_pct`, `nav_usd`, `skew_direction` computed property), `RebalanceTarget`
- [ ] 2.3 `src/models/risk.py` — `DrawdownSnapshot`, `RiskEvent`, `RiskLevel` enum (`NORMAL`, `SLOW`, `PAUSE`)
- [ ] 2.4 `src/models/report.py` — `Report` with period, P&L (`Decimal`), uptime %, depth compliance %

---

## Phase 3 — Exchange Gateway & Price Oracle
**Milestone:** Live bid/ask prices flowing from exchange; testnet orders placeable.

- [ ] 3.1 `src/exchange/gateway.py` — async CCXT wrapper: `initialize`, `get_balances`, `place_order`, `cancel_order`, `cancel_all_orders`, `fetch_open_orders`
- [ ] 3.2 Add exponential backoff retry (max 3 attempts) for rate-limit and network errors
- [ ] 3.3 Wrap all CCXT exceptions in a local `ExchangeError` hierarchy
- [ ] 3.4 `src/exchange/price_oracle.py` — WebSocket feed via CCXT Pro `watch_order_book`; expose `get_mid_price()` and `get_order_book()`
- [ ] 3.5 Implement staleness check: raise `StalePriceError` if no update in >5s; fallback to REST `fetch_order_book`
- [ ] 3.6 Integration smoke test: connect to testnet, assert `Decimal` types throughout

---

## Phase 4 — Market Making Core
**Milestone:** Bot places and maintains a live two-sided order grid on testnet.

- [ ] 4.1 `src/market_maker/spread_calculator.py` — generate bid/ask `OrderGrid` within spread bounds; apply inventory skew adjustment (LONG → lower ask; SHORT → lower bid); widen spread if `SLOW`; return empty grid if `PAUSE`
- [ ] 4.2 `src/market_maker/order_manager.py` — `run_cycle()`: fetch price → calculate grid → diff vs open orders → cancel stale (drift >0.1%) → place new orders; track active orders in-memory dict
- [ ] 4.3 Implement `emergency_cancel_all()` called by `RiskController` on `PAUSE`
- [ ] 4.4 `src/market_maker/depth_monitor.py` — `check_depth()`: USD value within ±2% band ≥ `min_depth_usd` OR count per side ≥ `min_order_count`; return `DepthReport`; expose compliance as Prometheus gauge
- [ ] 4.5 Unit tests: symmetric spread on neutral inventory; ask skewed down on LONG; empty grid on PAUSE

---

## Phase 5 — Risk & Inventory Management
**Milestone:** Drawdown and skew controls halt/throttle engine automatically.

- [ ] 5.1 `src/risk/engine.py` — `compute_drawdown()` (DB read for 24h-ago NAV), `evaluate_risk_level()` (NORMAL / SLOW at 5% / PAUSE at 10%), `check_inventory_skew()`
- [ ] 5.2 `src/risk/controller.py` — state machine `NORMAL → SLOW → PAUSE → RESUME`; PAUSE calls `emergency_cancel_all()`; RESUME only after >30min recovery; state persisted in Redis
- [ ] 5.3 `src/inventory/manager.py` — `get_current_state()`, `compute_rebalance_target()`, `snapshot_to_db()` every 5 min
- [ ] 5.4 Rebalancing via `SpreadCalculator` skew; escalate to market orders only if skew >40% (configurable)
- [ ] 5.5 Unit tests: NORMAL→SLOW→PAUSE transitions; RESUME blocked until 30-min recovery; skew >20% triggers rebalance

---

## Phase 6 — Persistence Layer
**Milestone:** All trades, inventory snapshots, and risk events reliably stored.

- [ ] 6.1 `src/persistence/migrations/001_initial.sql` — tables `inventory_snapshots`, `placed_orders`, `risk_events`; all monetary columns `NUMERIC(28,10)` — never `FLOAT`
- [ ] 6.2 `src/persistence/db.py` — `asyncpg.Pool` init + `run_migrations()` helper
- [ ] 6.3 `src/persistence/repositories/trade_repo.py` — `save_order`, `update_order_status`, `get_orders_in_range`
- [ ] 6.4 `src/persistence/repositories/inventory_repo.py` — `save_snapshot`, `get_nav_at(datetime)`, `get_snapshots_in_range`
- [ ] 6.5 `src/persistence/repositories/risk_event_repo.py` — `save_event`, `get_active_pause`, `resolve_event`
- [ ] 6.6 Integration tests against real PostgreSQL (Docker fixture)

---

## Phase 7 — Reporting
**Milestone:** Automated reports generated and saved on schedule.

- [ ] 7.1 `src/reporting/reporter.py` — `generate_daily_report()`, `generate_weekly_report()`, `generate_monthly_report()`; compute P&L as `Decimal`, uptime %, depth compliance %, risk event summary
- [ ] 7.2 Schedule via APScheduler: daily 00:05 UTC, weekly Monday 00:10 UTC, monthly 1st 00:15 UTC
- [ ] 7.3 Save reports as JSON to `reports/` with sanitized paths (prevent path traversal)
- [ ] 7.4 Optional email delivery when `REPORT_EMAIL` env var is set

---

## Phase 8 — Monitoring & Observability
**Milestone:** Grafana dashboard live; alerts configured.

- [ ] 8.1 `src/monitoring/metrics.py` — Prometheus gauges (NAV, skew, drawdown, risk level, depth compliance, order counts, spread), counters (orders placed/cancelled/filled, risk events, cycle errors), histograms (cycle latency, order placement latency)
- [ ] 8.2 Expose `/metrics` via `aiohttp` on port 9090; add `/health` returning `{"status": "ok", "risk_level": "..."}`
- [ ] 8.3 Grafana dashboard: spread gauge, drawdown gauge, inventory skew time series, order counts, uptime %
- [ ] 8.4 Alerting rules: drawdown breach, uptime <95%, price feed disconnect >10s
- [ ] 8.5 Structured JSON logging with `_log_lock` for all shared log file writes

---

## Phase 9 — Main Orchestrator & Graceful Shutdown
**Milestone:** Full system starts, runs, and shuts down without leaving open orders.

- [ ] 9.1 `src/main.py` — boot sequence: DB pool → Redis → Gateway → Oracle → Business logic → Background tasks → Scheduler
- [ ] 9.2 `main_loop()` runs every `CYCLE_INTERVAL_S` (default 10s): risk check → inventory snapshot → order cycle
- [ ] 9.3 Graceful shutdown on `SIGINT`/`SIGTERM`: `emergency_cancel_all()`, flush DB, close connections
- [ ] 9.4 All startup errors fatal — log and exit with non-zero code

---

## Phase 10 — Testing & Hardening
**Milestone:** System passes 72h continuous testnet run at ≥95% uptime.

- [ ] 10.1 Unit tests: `SpreadCalculator`, `RiskEngine`, `InventoryState`, `DepthMonitor`
- [ ] 10.2 Integration test: full market-making cycle with stubbed oracle + gateway
- [ ] 10.3 Chaos tests: feed disconnect recovery, exchange timeout, Redis failover
- [ ] 10.4 72h continuous testnet run — verify uptime ≥95%, no float leakage, reports generated correctly
- [ ] 10.5 Security review: API key handling via env vars only, path sanitization on all file writes, no secrets logged

---

## Completion Checklist

| Phase | Description | Status |
|-------|-------------|--------|
| 1 | Scaffold + Config | [ ] |
| 2 | Data Models | [ ] |
| 3 | Exchange Gateway + Oracle | [ ] |
| 4 | Market Making Core | [ ] |
| 5 | Risk + Inventory | [ ] |
| 6 | Persistence Layer | [ ] |
| 7 | Reporting | [ ] |
| 8 | Monitoring | [ ] |
| 9 | Main Orchestrator | [ ] |
| 10 | Tests & Hardening | [ ] |
