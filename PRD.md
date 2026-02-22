## Vibe Copy Trading Core – Product Requirements Document (PRD)

### 1. Overview

- **Product name (working):** Vibe Copy Trading Core
- **Scope:** Backend services only – **signal listener**, **copy engine**, **analytics**, **monitoring**, and **tracing**.
- **Out of scope:**
  - User authentication / authorization.
  - Web or mobile frontends.
  - Billing, KYC/AML, legal/compliance workflows.
  - Influencer and user onboarding UX.
- **Purpose:** Provide a highly scalable and observable core that ingests trading signals from influencers on Hyperliquid, fans them out via a message bus, computes copy trades for subscribers, and records analytics.
- **Primary targets:**
  - **5,000 influencer addresses** as signal sources.
  - **200,000 users** represented as subscribers in the core configuration.
  - Designed for **horizontal scalability** via a **message bus**.
  - Rich **monitoring, logging, and distributed tracing** across all core services.

### 2. Stakeholders

- **Tech Lead / Architect:** TBD
- **Backend Engineering:** TBD
- **DevOps / SRE:** TBD
- **Data / Analytics:** TBD

### 3. Core Use Cases (Core Engine Only)

1. **Signal ingestion from Hyperliquid**
   - The system listens to trades/positions of configured influencer addresses on Hyperliquid in near real time.

2. **Signal normalization and publication**
   - Raw Hyperliquid events are normalized into a canonical "signal" format and published to a message bus.

3. **Subscription-based fan-out**
   - For each normalized signal, the system determines which subscribers should react and emits execution requests accordingly.

4. **Copy execution**
   - For each (subscriber, signal) pair, the core computes the target copy trade (size, leverage, risk) and submits an order to Hyperliquid (or queues execution, depending on integration mode).

5. **Analytics & metrics computation**
   - The core records signals and executions to compute influencer performance, subscriber performance, and system throughput/latencies.

6. **Monitoring & tracing**
   - Operators can monitor health, throughput, latency, and error rates.
   - Engineers can trace any given signal through ingestion → fan-out → execution.

> Note: Creation and management of influencers, users, and subscriptions (e.g., via an admin UI or external system) is **out of scope** for this PRD. This document assumes those configurations are already present in storage and readable by the core.

### 4. Functional Requirements

#### 4.1 Configuration & Identity Model (Read-Only for Core)

- The core reads configuration from a **configuration store** (DB, config service, or files – TBD):
  - **Influencers**: list of Hyperliquid addresses to track.
  - **Subscribers/Users**: identifiers and configuration relevant to risk and allocation.
  - **Subscriptions**: mappings between subscribers and influencers plus per-subscription settings.
- Requirements:
  - Read-only from the perspective of the core services (no CRUD APIs required here).
  - Configuration must be cacheable and support efficient lookup by influencer and subscriber IDs.
  - Hot-reload or periodic refresh of configuration without restarting services.

#### 4.2 Subscription Model (Engine Perspective)

- Each **subscription** includes:
  - `subscriber_id`
  - `influencer_id`
  - Allocation configuration:
    - Mode (fixed size, % of portfolio, or notional cap).
    - Per-trade max risk (max size, max loss, max leverage).
  - Filters:
    - Allowed markets (e.g., only certain pairs).
    - Optional time-of-day or day-of-week rules (TBD).
  - Status (active, paused).
- The core must support at least **200,000 active subscriptions** overall, distributed across up to **5,000 influencers**.

#### 4.3 Signal Ingestion (Hyperliquid)

- Connect to **Hyperliquid** APIs (websocket and/or REST):
  - Receive trades, orders, and/or position updates for configured influencer addresses.
- Ingestion behavior:
  - Maintain resilient connections with automatic reconnect and backoff.
  - Ensure idempotent handling (deduplicate events using IDs/timestamps).
  - Provide backfill mechanisms for gaps (e.g., start-up or outages).
- Performance:
  - Target **P95 < 1s** from Hyperliquid event reception to publication on the internal message bus under normal load.

#### 4.4 Signal Normalization & Enrichment

- Normalize all events into a canonical **Signal** object, including fields like:
  - `signal_id`, `influencer_id`, `hyperliquid_address`, `timestamp`, `market`, `side`, `size`, `price`, `leverage`, `order_type`, `position_state`.
- Enrich with:
  - Market metadata (symbol normalization, tick size, lot size, leverage caps).
  - Derived quantities (e.g., notional value in USD, risk bucket).
- Storage:
  - Persist both raw events and normalized signals to durable storage for at least **90 days** in total.
  - Allow tiered storage: recent data (for example the last 7–30 days) may live in hot OLTP/operational stores, with older data moved to warm/cold storage (for example object storage or a warehouse) while remaining queryable for analytics.

#### 4.5 Message Bus & Horizontal Fan-Out

- Provide a **message bus** abstraction (Kafka, NATS, RabbitMQ, or managed equivalent – TBD):
  - Topics/subjects for:
    - `influencer_signals` (normalized signals).
    - `execution_requests` (per-subscriber execution jobs).
    - `execution_results` (outcomes of submitted orders).
- Partitioning & scaling:
  - Partition `influencer_signals` by `influencer_id` or market to spread load.
  - Consumers must be horizontally scalable (add instances to increase throughput).
  - At-least-once delivery semantics.

#### 4.6 Subscription Matcher Service

- Subscribes to `influencer_signals`.
- For each incoming signal:
  - Look up active subscriptions for the corresponding `influencer_id`.
  - Apply filter rules (markets, risk constraints pre-checks, etc.).
  - Emit an `execution_request` message for each matched (subscriber, signal) pair.
- Performance targets:
  - Sustain approximately **100 signals/second** across all influencers under normal load, with design headroom for bursts of **300–500 signals/second** via internal queues and backpressure.
  - Support internal fan-out generating up to **1,000–5,000 execution requests/second** during peaks. This is an internal pipeline throughput target; the actual rate of orders placed to Hyperliquid may be lower and is explicitly constrained by exchange and per-account rate limits.

#### 4.7 Copy Execution Engine

- **Execution Planner**:
  - Consume `execution_requests` from the bus.
  - For each (subscriber, signal):
    - Compute order parameters (side, size, leverage, price behavior).
    - Apply risk and allocation rules from the subscription and subscriber config.
    - Ensure idempotency (avoid duplicate orders for the same signal/subscriber).
  - Publish concrete execution jobs to a worker queue or `execution_requests` sub-topic if needed.

- **Execution Workers**:
  - Submit orders to Hyperliquid (or generate execution instructions, depending on integration model).
  - Handle order placement, amendments, and cancellations based on strategy or failure modes.
  - Implement robust retry with exponential backoff and circuit-breaking.
  - Persist a complete audit trail: request payloads, responses, timestamps, and error details.
  - Respect explicit rate-limit and concurrency configurations so that external order submission never exceeds agreed Hyperliquid or per-account limits, even if the internal pipeline can generate more execution requests.

#### 4.8 Analytics & Reporting (Core Data)

- Capture data required for downstream analytics (dashboards may be out of scope but data must be available):
  - **Influencer-level metrics**:
    - PnL, drawdown, win rate, trade frequency.
    - Total copied volume and number of active subscribers.
  - **Subscriber-level metrics**:
    - PnL per influencer and overall.
    - Execution quality (fill rate, slippage estimates).
  - **System metrics**:
    - Signals per second, execution requests per second, completed orders per second.
- Provide queryable storage (e.g., OLAP/warehouse-friendly schema) or export streams for external analytics tools.

### 5. Non-Functional Requirements

#### 5.1 Scalability Targets

- **Influencers:**
  - Support **5,000 active influencer addresses** as signal sources.
- **Subscribers/Users:**
  - Represent **up to 200,000 subscribers** in the configuration store.
  - Support at least **50,000 concurrently active** subscribers initially, with design headroom for 200,000 active.
- **Throughput:**
  - Sustain approximately **100 signals/second** under normal conditions.
  - Provide design headroom for bursts of **300–500 signals/second** via internal queues and backpressure, with graceful degradation if bursts exceed engineered capacity.
  - Support resulting internal fan-out of **1,000–5,000 execution requests/second**; the actual rate of external order submissions is governed by exchange and per-account rate limits and may be lower.
- **Horizontal scaling:**
  - Ingestion, matcher, planner, and worker services must be stateless or shardable behind the message bus.

#### 5.2 Availability & Reliability

- Target **99.5% uptime** for core services and internal APIs.
- Once a signal is accepted by the ingestion service, it must be **durably stored** (raw and/or normalized) before further processing.
- Use at-least-once delivery semantics between services; downstream components must handle duplicates.
- Graceful degradation:
  - If execution is partially degraded, ingestion and signal recording should continue where possible.
  - Allow read-only or "record-only" operational modes during incidents where order placement is disabled or heavily rate-limited, while still meeting durability guarantees.

#### 5.3 Latency

- End-to-end internal latency from received influencer signal to the first order submission attempt sent from the Execution Worker to Hyperliquid:
  - **Target P95 < 2 seconds**, **P99 < 5 seconds** under normal load.
- Expose latency metrics for:
  - Ingestion → bus publish.
  - Bus → matcher.
  - Matcher → execution planner.
  - Execution planner → Hyperliquid API.
  - Hyperliquid API response latency (tracked and monitored, but not part of the internal pipeline SLO and subject to external exchange conditions).

#### 5.4 Security (Core Services)

- Secure storage of any API keys and secrets (vault/KMS).
- Encrypted communication (TLS) between services and external APIs.
- Minimal internal RBAC for operators/automation (e.g., read-only vs write for config access) – UI for this is out of scope.
- Basic protections for external-facing endpoints (if any) such as rate limiting.

### 6. Architecture & Components (Core Only)

#### 6.1 Logical Components

- **Signal Ingestion Service (Hyperliquid)**
  - Maintains connections to Hyperliquid.
  - Receives and persists raw events.
  - Produces normalized signals to the message bus.

- **Message Bus Layer**
  - Provides topics/subjects for `influencer_signals`, `execution_requests`, and `execution_results`.
  - Handles partitioning and consumer groups for scale-out.

- **Subscription Matcher Service**
  - Consumes `influencer_signals`.
  - Resolves subscriptions from the configuration store.
  - Emits `execution_requests` for each matched subscriber.

- **Execution Planner Service**
  - Consumes `execution_requests`.
  - Applies allocation and risk logic.
  - Produces concrete orders or execution jobs.

- **Execution Worker Service(s)**
  - Submit orders to Hyperliquid.
  - Track statuses and emit `execution_results`.
  - Persist execution records and audit trails.

- **Analytics Processor**
  - Consumes signals and execution results.
  - Aggregates metrics and stores them in analytics-friendly storage.

- **Observability Stack (shared)**
  - Metrics collection (Prometheus/OpenTelemetry or equivalent).
  - Centralized logging.
  - Distributed tracing backend.

> Note: API gateways, web frontends, and admin consoles are intentionally excluded. Any required internal or external APIs are assumed to be thin wrappers around these core services and are not specified in detail here.

### 7. Monitoring, Logging & Tracing

#### 7.1 Metrics

- Implement standardized metrics across all services, including:
  - Request/operation counts and error rates per endpoint or handler.
  - Latency histograms per stage (ingestion, matcher, planner, worker).
  - Message bus topic depths and consumer lag.
  - Active connections to Hyperliquid.
  - Number of active influencers, subscribers, and subscriptions loaded in memory.

#### 7.2 Logging

- Use structured, centralized logging (e.g., JSON logs to a log aggregation system).
- Include correlation IDs and trace IDs in all logs.
- Log key lifecycle events:
  - Connection status changes with Hyperliquid.
  - Signal ingestion and normalization failures.
  - Execution attempts, retries, and failures.

#### 7.3 Tracing

- Use OpenTelemetry or equivalent for distributed tracing.
- Each signal should have a trace that spans:
  - Hyperliquid ingestion.
  - Message bus publish.
  - Subscription matching.
  - Execution planning.
  - Order submission and result handling.

#### 7.4 Dashboards & Alerts (Core)

- Dashboards for:
  - Overall system health and throughput.
  - Per-service latencies and error rates.
  - Message bus backlog and consumer lag.
- Alerts on:
  - Elevated error rates by service.
  - Latency SLO violations.
  - Message bus backlogs exceeding thresholds.
  - Unexpected drops in signal or execution volume.

### 8. Data Model (High-Level)

- **Influencer**
  - `id`, `hyperliquid_address`, `metadata`, `status`.

- **Subscriber/User (Core View)**
  - `id`, `status`, optional `risk_profile` or configuration bundle.
  - No auth details are stored in the core.

- **Subscription**
  - `id`, `subscriber_id`, `influencer_id`, `allocation_mode`, `allocation_amount`, `risk_limits`, `filters`, `status`.

- **Signal**
  - `id`, `influencer_id`, `source_tx_id`, `market`, `side`, `size`, `price`, `leverage`, `timestamp`, `normalized_payload`, `raw_payload_ref`.

- **Execution**
  - `id`, `subscriber_id`, `influencer_id`, `signal_id`, `order_params`, `status`, `exchange_order_id`, `created_at`, `updated_at`, `error_info`.

### 9. Constraints & Assumptions

- Initial release supports **Hyperliquid only** as the trading platform/data source.
- Core services are backend-only, exposed via internal APIs and message bus; separate components (not in this PRD) will handle user interfaces and external API exposure.
- Subscribers manage funding and account connectivity to Hyperliquid via systems outside this PRD.
- Business logic for fee models, revenue sharing, and regulatory compliance may use the data produced here but are defined elsewhere.
- Implementation languages: core pipeline services are primarily implemented in Go, with the architecture explicitly allowing selected services (for example ingestion adapters or HTTP APIs) to be implemented in Node.js/TypeScript where that is a better fit. Detailed language choices are defined in TECHNICAL_SPECS.

### 10. Phased Delivery (Core)

#### Phase 0 – Architecture & PoC

- Finalize detailed architecture and message bus choice.
- Implement a minimal ingestion pipeline from Hyperliquid for a small number of influencers (< 50).
- Implement basic metrics and logging and run initial load tests.

#### Phase 1 – MVP Core

- Support:
  - ~500 influencer addresses.
  - Up to ~10,000 configured subscribers.
- Implement:
  - Production-ready ingestion, normalization, and message bus topics.
  - Subscription matcher, execution planner, and workers.
  - Baseline monitoring, structured logging, and distributed tracing.

#### Phase 2 – Scale-Out

- Scale to **5,000 influencers** and **200,000 subscribers**.
- Optimize partitioning, sharding, and caching for configuration and subscriptions.
- Enhance analytics data pipelines and performance metrics.

#### Phase 3 – Hardening & Extensions

- Improve reliability and latency SLAs.
- Add advanced risk controls and safety checks within the core.
- Prepare for potential multi-exchange/multi-data-source support (beyond Hyperliquid).

### 11. Open Questions (Core-Focused)

- Final choice of message bus technology and hosting (self-managed vs managed cloud).
- Choice of storage solutions for raw events, normalized signals, and analytics (e.g., OLTP vs OLAP separation).
- Exact contract for how external systems manage and update influencer/subscriber/subscription configuration (APIs, schemas, refresh mechanisms).
- Execution model: fully automated order placement vs producing instructions for external executors (and how this affects reliability/latency targets).
