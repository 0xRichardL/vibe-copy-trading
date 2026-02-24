# Ingestion Service ("ingestion") – Technical Specification

> This is a service-level spec that extends the system-level design in ../TECHNICAL_SPECS.md (see sections 1.1, 2, and 3).

## 1. Overview

- **Service name:** Ingestion
- **Primary role:** Maintain redundant connections to Hyperliquid for configured influencer addresses, ingest and normalize events into canonical `Signal` objects, and publish them to the `influencer_signals` topic while minimizing dropped signals.
- **Tech stack:** Go (service implementation), Redis (influencer/config store), Kafka.

## 2. Responsibilities (Summary)

- Maintain resilient, redundant WebSocket connections to Hyperliquid for configured influencer accounts (double-listener strategy).
- Subscribe to influencer account-level streams (fills, orders, and/or position updates) and detect changes in effective position per market.
- Derive canonical `Signal` objects from those events (e.g., OPEN/CLOSE/INCREASE/DECREASE position for a given market).
- Ensure idempotent processing (deduplication by stable event IDs / sequence numbers per influencer+market across both listeners).
- Persist raw Hyperliquid events for audit and emit normalized `Signal` messages to the `influencer_signals` topic.

> See system-level responsibilities in ../TECHNICAL_SPECS.md §1.1.

## 3. Architecture & Dependencies

- **Runtime:** Go.
- **Message bus:** Kafka topic `influencer_signals`.
- **Config / influencer store:**
  - Redis as the source of truth for influencer accounts and per-influencer settings.
  - Shared config library/store (read-only) for non-influencer service configuration (endpoints, retry policies, etc.).
- **Libraries (Go):**
  - libs/go/hyperliquid (Hyperliquid WebSocket client and related helpers).
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

3. **Double-listener redundancy (secondary path)**

- For every influencer, maintain two independent listener loops that consume the same subscription set.
- Listeners can share a physical WebSocket connection or operate on separate connections when fan-out pressure is high; either way, messages flow into isolated pipelines.
- Each listener forwards events into a deduplicating fan-in queue using deterministic event IDs, ensuring downstream processing is single-writer per influencer+market.
- If the primary listener lags or disconnects, the secondary stream continues emitting signals; the unhealthy listener can reconnect in the background without halting signal emission.
- Periodic health probes compare listener lag/skew and rotate primaries to avoid starvation.

4. **Resilience & lifecycle**
   - Auto-reconnect on WebSocket errors with exponential backoff and jitter.

- Re-subscribe all active influencer channels after reconnection for both listeners.
- Use watchdogs/heartbeats to detect stale connections per listener and trigger targeted reconnect/failover.

> Implementation details (packages, concurrency model, and internal modules) are defined in the service repo code-level docs.

## 4. Interfaces

### 4.1 Inbound

- **Hyperliquid WebSocket API**
  - Channel type: account/user-level events for configured influencer addresses.
  - Subscriptions: per-influencer subscriptions for fills, orders, and/or position updates (exact channel names per Hyperliquid docs).
  - Auth: API key / account auth model as required by Hyperliquid.
  - Rate/connection limits: respect Hyperliquid guidelines (max concurrent connections, message rate, subscription fan-out). Prefer multiplexing multiple influencers over shared connections where possible while still provisioning two logical listeners per influencer.

### 4.2 Outbound

- **Kafka topic `influencer_signals`**
  - Payload: normalized `Signal` objects (see §5).
  - Delivery semantics: at-least-once from ingestion; downstream consumers must handle idempotency via `signalId`/event keys.

- **Raw events storage**
  - Store unmodified Hyperliquid WebSocket events in an append-only store (e.g., OLAP table or log stream).
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
  - WebSocket endpoint(s) and listener allocation strategy (shared connection vs. dedicated per influencer per listener).
  - Connection limits that account for dual listeners without violating Hyperliquid quotas.
  - Backoff policy (initial delay, max delay, jitter) for reconnects per listener, including failover thresholds.

- **Idempotency & state**
  - Storage for last processed event ID/sequence per influencer+market shared across both listeners.

- **Kafka / storage**
  - Kafka brokers, topic config, and producer tuning (batch size, linger, acks).
  - Raw event storage target and retention.

## 7. Observability

- **Metrics**
  - Active WebSocket connections and reconnect count, segmented by listener role (primary/secondary).
  - Listener lag/skew and failover frequency.
  - Messages received/sec per influencer+market and per channel type.
  - Signals emitted/sec and per-action breakdown.
  - Deduplicated/dropped events counts.
  - WebSocket error rates and latency.

- **Logs**
  - Connection lifecycle (connect, disconnect, reconnect, auth failures) per listener.
  - Subscription changes (influencer added/removed, resubscribe events).
  - Listener failover decisions and dedupe outcomes.
  - Ingestion and normalization failures (with references to `sourceEventId`).

- **Traces**
  - Spans from raw Hyperliquid event reception (per listener) through `Signal` publish to Kafka.
  - Correlation IDs tying `Signal` to raw events and downstream processing.

## 8. Operational Considerations

- **Rate limiting & backpressure**
  - Respect Hyperliquid rate limits; throttle subscription fan-out and connection churn while maintaining double listeners.
  - Apply internal backpressure when Kafka or raw storage is slow (e.g., bounded queues, shedding low-priority signals if configured).

- **Failure modes & recovery**
  - WebSocket disconnects and transient network errors handled via listener-specific reconnect with automatic failover to the healthy listener (no external replay phase assumed).
  - Poison events (malformed or repeatedly failing normalization) routed to a dead-letter path with alerts.
  - Idempotent reprocessing across restarts using stored last event IDs/sequences.

- **Deployment & scaling**
  - Horizontal scaling via partitioning influencers across ingestion instances while budgeting for redundant listeners per influencer allocation.
  - Ensure signal ordering guarantees per influencer+market (e.g., pin those to a single worker or ordered Kafka partitioning scheme).
  - Safe rollout/rollback via blue-green or canary deployments, with close monitoring of signal rates, listener health, and error metrics.
