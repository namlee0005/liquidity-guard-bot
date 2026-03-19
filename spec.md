# Liquidity Guard Bot — Market Maker Project Spec

## Executive Summary

The Liquidity Guard Bot is a specialized automated market maker designed to maintain liquidity and prevent delisting for low-volume crypto assets. It continuously places and manages layered bid/ask orders to enforce minimum market depth, operates within configurable spread bounds (0.3%–1%), and enforces hard risk controls against inventory drawdown (max 5–10% NAV per 24h). The system targets ≥95% uptime with autonomous SLOW/PAUSE throttle logic and automated daily, weekly, and monthly reporting.

## Recommended Tech Stack

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

## Architecture Overview

```
┌──────────────────────────────────────────────────────────────┐
│                      Liquidity Guard Bot                      │
│                                                              │
│  ┌─────────────────┐    ┌──────────────────┐  ┌──────────┐  │
│  │   Price Oracle   │───▶│   Order Engine   │─▶│  CCXT    │  │
│  │ (WS + REST fall) │    │  (Spread Calc +  │  │ Exchange │  │
│  └────────┬────────┘    │   Order Manager) │  └──────────┘  │
│           │             └────────┬─────────┘                │
│           ▼                      │                           │
│  ┌─────────────────┐    ┌────────▼─────────┐  ┌──────────┐  │
│  │  Depth Monitor  │    │ Inventory Manager│─▶│ PostgreSQL│  │
│  │ (compliance chk)│    │ (skew, NAV, snap)│  │(trade log)│  │
│  └─────────────────┘    └────────┬─────────┘  └──────────┘  │
│                                  │                           │
│  ┌─────────────────┐    ┌────────▼─────────┐  ┌──────────┐  │
│  │  Risk Controller │◀───│   Risk Engine    │  │  Redis   │  │
│  │  (state machine) │    │ (drawdown check) │─▶│(state +  │  │
│  │ NORMAL/SLOW/PAUSE│    └──────────────────┘  │ cache)   │  │
│  └────────┬────────┘                           └──────────┘  │
│           │                                                   │
│  ┌────────▼────────┐    ┌──────────────────┐                 │
│  │    Reporter     │    │Prometheus Exporter│                 │
│  │(daily/wk/monthly│    │  + Grafana dash  │                 │
│  └─────────────────┘    └──────────────────┘                 │
└──────────────────────────────────────────────────────────────┘
```

### Key Components

- **Price Oracle:** Async WebSocket consumer (CCXT Pro). Raises `StalePriceError` if no update in >5s; falls back to REST. Feeds mid-price and order book snapshot into Redis.
- **Order Engine:** `SpreadCalculator` generates a layered bid/ask grid (20–50 orders per side) within configured spread (0.3%–1%). Skews quotes toward rebalancing when inventory is unbalanced (LONG → lower ask; SHORT → lower bid). `OrderManager` diffs against live orders, cancels stale (price drift >0.1%), and places new orders each cycle (default 10s).
- **Depth Monitor:** Verifies compliance — either total order value within ±2% price band ≥ `min_depth_usd`, OR order count per side ≥ `min_order_count`. Exposes compliance as a Prometheus gauge.
- **Inventory Manager:** Tracks fills and balances; computes NAV and skew %; snapshots to PostgreSQL every 5 min. Feeds skew data to Risk Engine and SpreadCalculator.
- **Risk Engine + Controller:** Monitors 24h NAV drawdown. State machine: `NORMAL → SLOW (≥5% drawdown) → PAUSE (≥10% drawdown) → RESUME (after 30-min recovery)`. PAUSE triggers `emergency_cancel_all()`. State persisted in Redis for crash recovery.
- **Reporter:** APScheduler jobs query PostgreSQL for P&L, uptime %, depth compliance %, and risk events. Outputs sanitized JSON reports to `reports/`; optional email delivery.
- **Prometheus Exporter:** `/metrics` on port 9090 (configurable); `/health` endpoint. Grafana dashboard covers spread, drawdown, skew, order counts, and uptime.
