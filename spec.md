# Liquidity Guard Bot — Market Maker Project Spec

## Executive Summary

The Liquidity Guard Bot is a specialized automated market maker designed to maintain liquidity and prevent delisting for low-volume crypto assets. It continuously places and manages layered bid/ask orders to enforce minimum market depth, operates within configurable spread bounds (0.3%–1%), and enforces hard risk controls against inventory drawdown (max 5–10% NAV per 24h). The system targets ≥95% uptime with autonomous SLOW/PAUSE throttle logic and automated daily, weekly, and monthly reporting.

## Recommended Tech Stack (ORIGINAL — DEPRECATED)

| Layer | Technology | Rationale |
|---|---|---|
| **Runtime** | Python 3.12+ | Mature async ecosystem, rich quant libraries, rapid iteration |
| **Exchange Integration** | CCXT Pro | Unified async API across 100+ exchanges; WebSocket `watch_order_book` support |
| **Order & State DB** | PostgreSQL | ACID compliance for trade logs and inventory snapshots; `NUMERIC` types enforced |
| **Caching / Pub-Sub** | Redis | Sub-millisecond order book state, risk controller state persistence across restarts |
| **Market Feed** | CCXT Pro WebSocket | Real-time bid/ask via async streams; REST fallback on staleness (>5s) |
| **Monitoring** | Prometheus + Grafana | Uptime, spread adherence, drawdown, inventory skew dashboards and alerting |
| **Task Scheduling** | APScheduler | Inventory snapshots every 5 min; report generation daily/weekly/monthly |
| **Containerization** | Docker + Docker Compose | Reproducible local and production deployments |
| **Config** | Pydantic Settings + `.env` | Type-safe, validated configuration; no secrets in code |

> **Hard rule:** `Float` is strictly prohibited for all monetary and quantity fields. Use Python `Decimal` and PostgreSQL `NUMERIC(28,10)` throughout.

---

## REVISION 1 — 2026-03-19 (Architecture & Stack Pivot — DEPRECATED)

### New Technical Constraints
- **Language:** **Go (Golang)** — Mandatory for high-concurrency exchange handling via Goroutines.
- **Database:** **MongoDB** — For flexible, document-based configuration and bot lifecycle state management.
- **Multi-Exchange Support:** Initial scope: MEXC, Bybit, Gate.io, Kraken.
- **Control Plane:** Must support an external 3rd-party management interface (Create/Pause/Delete bots, Live Config Update).
- **Telemetry:** Real-time WebSocket streaming of per-pair order books and account balances to the control plane.

---

## REVISION 2 — 2026-03-19 (Hybrid DB — DEPRECATED)

### Summary of Changes from Revision 1
1. **DB split:** MongoDB for configs/sessions; PostgreSQL for TradeHistory/AuditLogs.
2. **API:** Management API is gRPC + Protobuf.
3. **Risk Watchdog:** Separate Docker container.
4. **Redis:** Eliminated.

---

## REVISION 3 — 2026-03-19 (Single DB Consolidation)

### Rationale for Consolidation
To reduce operational complexity and infrastructure footprint, the system is consolidated to a **single database: MongoDB**. All financial records, audit logs, and configurations will reside in MongoDB. Financial consistency is ensured via MongoDB Multi-Document Transactions where necessary.

### Final Tech Stack

| Layer | Technology | Rationale |
|---|---|---|
| **Runtime** | Go 1.22+ | Native goroutine parallelism; type-safe concurrency models |
| **Primary Database** | **MongoDB 7.0+** | Consolidated store for all data types; flexible schema for multi-exchange configs |
| **Exchange Integration** | Native Go adapters | Unified 4-method interface for MEXC, Bybit, Gate.io, Kraken |
| **Control Plane** | gRPC (Protobuf) | Strongly typed contracts; bidirectional telemetry streaming |
| **Risk Watchdog** | Separate Go service | Independent process monitoring NAV/Drawdown; communicates via MongoDB |
| **Telemetry** | gRPC Server-Streaming | Order books and balances pushed over the management channel |
| **Monetary Types** | `shopspring/decimal` + **BSON Decimal128** | `float64` strictly prohibited; atomic precision maintained |

### Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│                    Liquidity Guard Bot (Go binary)                   │
│                                                                     │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │            gRPC Control Plane API (Protobuf)                 │   │
│  │  CreateBot / PauseBot / DeleteBot / UpdateConfig             │   │
│  │  StreamTelemetry (server-stream: order book + balances)      │   │
│  └───────────────────────┬─────────────────────────────────────┘   │
│                          │ signals via MongoDB/Channels             │
│  ┌───────────────────────▼─────────────────────────────────────┐   │
│  │      Bot Registry (sharded map[botID]*BotWorker)            │   │
│  │                                                             │   │
│  │  ┌──────────────┐  ┌──────────────┐  ┌────────────────┐   │   │
│  │  │  MEXC Worker │  │ Bybit Worker │  │ Gate / Kraken  │   │   │
│  │  │ (goroutine)  │  │ (goroutine)  │  │   Workers      │   │   │
│  │  └──────┬───────┘  └──────┬───────┘  └───────┬────────┘   │   │
│  └─────────┼─────────────────┼──────────────────┼────────────┘   │
│            │ market data, balance, order events                  │
│  ┌─────────▼─────────────────▼──────────────────▼────────────┐   │
│  │  Per-Worker Core                                            │   │
│  │  SpreadCalc │ OrderManager │ DepthMonitor │ InventoryTracker│   │
│  └────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────┬───────────────────────┘
                                              │ CRUD
                                              ▼
┌─────────────────────┐        ┌──────────────────────────────────┐
│   Risk Watchdog     │        │           MongoDB                │
│  (separate container│◀──────▶│  - BotConfigs                    │
│   no main dep.)     │        │  - ActiveSessions                │
│  FSM: NOR/SLOW/PAUSE│        │  - TradeHistory (ACID)           │
│                     │        │  - AuditLogs (ACID)              │
└─────────────────────┘        └──────────────────────────────────┘
```

### Key Component Adjustments

- **State Store (MongoDB Only):** 
    - `BotConfigs`: Bot parameters and exchange credentials.
    - `ActiveSessions`: Real-time bot status, risk state, and heartbeats.
    - `TradeHistory`: Every fill, order placement, and cancellation. Use TTL indexes for automatic data rotation if needed.
    - `AuditLogs`: Record of all external management actions.
- **Data Integrity:** All writes to `TradeHistory` and `AuditLogs` use `WriteConcern: Majority`. Complex rebalancing or multi-step inventory updates use MongoDB sessions/transactions to prevent split-brain state.
- **Risk Watchdog:** Runs as an independent Docker service. Polls NAV snapshots from `TradeHistory` and balances from `ActiveSessions`. Writes risk state directly to the bot's session document in MongoDB.
- **Bot Orchestrator:** Polls its assigned session document in MongoDB for state changes (`PAUSE`, `CONFIG_UPDATE`) every 5s as a fallback to internal gRPC signals.
