# Ingestion Service ("ingestion") – Technical Specification

> This is a service-level spec that extends the system-level design in ../TECHNICAL_SPECS.md (see sections 1.1, 2, and 3).

## 1. Overview

- **Service name:** Ingestion
- **Primary role:** Maintain connections to Hyperliquid for configured influencer addresses, ingest and normalize events into canonical `Signal` objects, and publish them to the `influencer_signals` topic.
- **Tech stack:** Go (service implementation), Redis (influencer/config store), Kafka.

## 2. Responsibilities (Summary)

- Maintain resilient WebSocket/REST connections to Hyperliquid for configured influencer accounts.
- Subscribe to influencer account-level streams (fills, orders, and/or position updates) and detect changes in effective position per market.
- Derive canonical `Signal` objects from those events (e.g., OPEN/CLOSE/INCREASE/DECREASE position for a given market).
- Ensure idempotent processing (deduplication by stable event IDs / sequence numbers per influencer+market).
- Persist raw Hyperliquid events for audit/backfill and emit normalized `Signal` messages to the `influencer_signals` topic.

> See system-level responsibilities in ../TECHNICAL_SPECS.md §1.1.

## 3. Architecture & Dependencies

- **Runtime:** Go.
- **Message bus:** Kafka topic `influencer_signals`.
- **Config / influencer store:**
  - Redis as the source of truth for influencer accounts and per-influencer settings.
  - Shared config library/store (read-only) for non-influencer service configuration (endpoints, retry policies, etc.).
- **Libraries (Go):**
  - libs/go/hyperliquid (Hyperliquid WebSocket/REST client).
  - libs/go/messaging (Kafka producer abstraction).
  - libs/go/observability (metrics, logs, traces).
  - libs/go/domain (shared domain types including `Signal`).

### 3.1 High-Level Ingestion Flow

1. **Startup & configuration**

- Load list of influencer accounts and per-influencer settings (markets of interest, throttling, etc.) from Redis via the shared config/influencer library.
- For each influencer, initialize a logical "connection context" (state: last processed seq/ID per market, backoff state, metrics handles).

2. **WebSocket subscription (primary path)**
   - Establish and maintain a Hyperliquid WebSocket connection (or connection pool) using the Hyperliquid client library.
   - For each configured influencer account, subscribe to user/account-level channels that surface:
     - Executions/fills.
     - Open/closed orders (optional, for richer signals).
     - Position snapshots/updates if available.
   - On each incoming message:
     - Validate schema and influencer membership.
     - Deduplicate using an event key (e.g., `txHash`/`eventId`/`sequence`, plus influencer+market).
     - Persist the raw event.
     - Normalize to a `Signal` (see §5) representing the resulting position change.
     - Publish the `Signal` to Kafka topic `influencer_signals`.

3. **REST backfill & reconciliation (secondary path)**
   - On startup, and after reconnects, query Hyperliquid REST APIs to:
     - Fetch current open positions per influencer+market.
     - Optionally fetch recent fills over a configurable lookback window.
   - Reconcile REST state with last processed WebSocket state to:
     - Fill gaps during downtime.
     - Correct any missed or out-of-order events.

4. **Resilience & lifecycle**
   - Auto-reconnect on WebSocket errors with exponential backoff and jitter.
   - Re-subscribe all active influencer channels after reconnection.
   - Use watchdogs/heartbeats to detect stale connections and trigger reconnect/backfill.

> Implementation details (packages, concurrency model, and internal modules) are defined in the service repo code-level docs.

## 4. Interfaces

### 4.1 Inbound

- **Hyperliquid WebSocket API**
  - Channel type: account/user-level events for configured influencer addresses.
  - Subscriptions: per-influencer subscriptions for fills, orders, and/or position updates (exact channel names per Hyperliquid docs).
  - Auth: API key / account auth model as required by Hyperliquid.
  - Rate/connection limits: respect Hyperliquid guidelines (max concurrent connections, message rate, subscription fan-out). Prefer multiplexing multiple influencers over shared connections where possible.

- **Hyperliquid REST API**
  - Endpoints: positions, recent trades/fills, account state.
  - Usage:
    - Startup snapshot of positions.
    - Backfill after reconnects.
    - Periodic reconciliation (optional).

### 4.2 Outbound

- **Kafka topic `influencer_signals`**
  - Payload: normalized `Signal` objects (see §5).
  - Delivery semantics: at-least-once from ingestion; downstream consumers must handle idempotency via `signalId`/event keys.

- **Raw events storage**
  - Store unmodified Hyperliquid events (WebSocket and REST responses) in an append-only store (e.g., OLAP table or log stream).
  - Keyed by influencer, market, event type, and event ID/sequence for audit and replay.

> Reference proto/contracts definitions for `Signal` and raw event envelopes once defined at the system level.

## 5. Data Contracts

### 5.1 `Signal` Event Schema (Ingestion View)

Minimal ingestion-time view of a `Signal` (exact formal schema lives in shared domain/proto definitions):

- `signalId`: globally unique, deterministic ID for the signal (e.g., hash of influencer+market+sourceEventId+action).
- `influencerId`: internal ID or reference for the influencer (not necessarily the raw Hyperliquid address).
- `exchange`: constant `"hyperliquid"` for this service.
- `market`: Hyperliquid market identifier (e.g., `ETH-PERP`).
- `action`: enum representing the semantic change:
  - `OPEN`, `CLOSE`, `INCREASE`, `DECREASE`, `FLIP` (if opening in the opposite direction).
- `side`: enum `LONG` | `SHORT` | `FLAT` representing resulting position side after the event.
- `size`: resulting position size (base units) after applying this event.
- `deltaSize`: signed change in position size from the previous state.
- `price`: relevant execution or average entry price.
- `timestamp`: event time from Hyperliquid (fallback: ingestion time).
- `sourceEventId`: reference to the underlying Hyperliquid event (e.g., tx hash / event ID / sequence).
- `metadata`: optional map for additional attributes (e.g., leverage, margin mode, raw symbol).

### 5.2 Raw Event Schema / Storage Model

- `id`: unique identifier (may reuse `sourceEventId`).
- `influencerAddress`: Hyperliquid account/address.
- `eventType`: enum (e.g., `FILL`, `ORDER_UPDATE`, `POSITION_SNAPSHOT`, `POSITION_UPDATE`).
- `market`: Hyperliquid market identifier.
- `rawPayload`: opaque JSON or binary blob of the original Hyperliquid event/response.
- `timestamp`: event timestamp from Hyperliquid.
- `ingestedAt`: ingestion timestamp.
- Indexes/partitioning:
  - By `influencerAddress`, `market`, `timestamp` for efficient replay and debugging.

> See ../TECHNICAL_SPECS.md §4 and shared proto/domain modules for the authoritative schemas.

## 6. Configuration

- **Influencers (Redis-backed)**
  - Influencer accounts/addresses and optional metadata (internal ID, label, priority, markets of interest) stored in Redis.
  - Per-influencer overrides stored in Redis for:
    - Subscribed markets.
    - Max concurrency / parallel streams.
    - Backoff/retry tuning.
  - Ingestion service reads this configuration on startup and may periodically refresh or subscribe to change notifications (if available).

- **Connection & retry**
  - WebSocket endpoint(s) and REST base URL(s).
  - Connection limits and shared-connection vs. per-influencer connection strategy.
  - Backoff policy (initial delay, max delay, jitter) for reconnects.

- **Idempotency & state**
  - Storage for last processed event ID/sequence per influencer+market.
  - Lookback window for REST backfill.

- **Kafka / storage**
  - Kafka brokers, topic config, and producer tuning (batch size, linger, acks).
  - Raw event storage target and retention.

## 7. Observability

- **Metrics**
  - Active WebSocket connections and reconnect count.
  - Messages received/sec per influencer+market and per channel type.
  - Signals emitted/sec and per-action breakdown.
  - Deduplicated/dropped events counts.
  - WebSocket/REST error rates and latency.

- **Logs**
  - Connection lifecycle (connect, disconnect, reconnect, auth failures).
  - Subscription changes (influencer added/removed, resubscribe events).
  - Ingestion and normalization failures (with references to `sourceEventId`).

- **Traces**
  - Spans from raw Hyperliquid event reception through `Signal` publish to Kafka.
  - Correlation IDs tying `Signal` to raw events and downstream processing.

## 8. Operational Considerations

- **Rate limiting & backpressure**
  - Respect Hyperliquid rate limits; throttle subscription and REST calls.
  - Apply internal backpressure when Kafka or raw storage is slow (e.g., bounded queues, shedding low-priority signals if configured).

- **Failure modes & recovery**
  - WebSocket disconnects and transient network errors handled via reconnect+backfill.
  - Poison events (malformed or repeatedly failing normalization) routed to a dead-letter path with alerts.
  - Idempotent reprocessing across restarts using stored last event IDs/sequences.

- **Deployment & scaling**
  - Horizontal scaling via partitioning influencers across ingestion instances.
  - Ensure signal ordering guarantees per influencer+market (e.g., pin those to a single worker or ordered Kafka partitioning scheme).
  - Safe rollout/rollback via blue-green or canary deployments, with close monitoring of signal rates and error metrics.
