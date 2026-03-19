# Liquidity Guard Bot — Market Maker Project Spec

## Goal
A specialized Market Maker bot designed to maintain liquidity and prevent delisting for low-volume crypto assets.

## Core Requirements
- **Dynamic Spread:** Maintain spread between 0.3% - 1% (fully configurable).
- **Market Depth:** 
    - Minimum depth of $X within ±2% price.
    - OR maintain >= 20-50 buy orders and >= 20-50 sell orders.
- **Uptime:** >= 95% guaranteed.
- **Reporting:** Daily, Weekly, and Monthly performance & status reports.
- **Risk Management (Inventory Drawdown):**
    - Max drawdown limit: 5-10% NAV within 24h.
    - Action: Automatically slow down or pause operations if threshold is breached.
- **Inventory Rebalancing:**
    - Trigger: Inventory skew > 20%.
    - Recovery Target: Rebalance to neutral within 24-72 hours.

## Initial Tech Stack Candidates
- Python 3.12+ (for rapid dev & quant libraries).
- CCXT (Exchange integration).
- SQLite or PostgreSQL (for trade logs and inventory state).
- Prometheus/Grafana (for uptime and real-time monitoring).
