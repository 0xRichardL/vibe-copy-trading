# Ingestion Service ("ingestion") – Technical Specification

> This is a service-level spec that extends the system-level design in ../TECHNICAL_SPECS.md (see sections 1.1, 2, and 3).

## 1. Overview

- **Service name:** Ingestion
- **Primary role:** Maintain connections to Hyperliquid for configured influencer addresses, ingest and normalize events into canonical `Signal` objects, and publish them to the `influencer_signals` topic.
- **Tech stack:** Go or Node.js/TypeScript (see root language choices).

## 2. Responsibilities (Summary)

- Maintain resilient WebSocket/REST connections to Hyperliquid.
- Ingest trades, orders, and/or position updates for configured influencers.
- Ensure idempotent processing (deduplication by IDs/timestamps).
- Persist raw events and emit normalized `Signal` messages to the bus.

> See system-level responsibilities in ../TECHNICAL_SPECS.md §1.1.

## 3. Architecture & Dependencies (TBD)

- **Runtime:** Go or Node.js/TS.
- **Message bus:** Kafka topic `influencer_signals`.
- **Config source:** Read-only configuration from config-api or shared config store.
- **Libraries:**
  - Go: libs/go/hyperliquid, libs/go/messaging, libs/go/observability, libs/go/domain (if implemented in Go).
  - Node: libs/ts/sdk and generated contracts (if implemented in Node).

(Detail internal modules, package layout, and dependency graph here.)

## 4. Interfaces

### 4.1 Inbound

- Hyperliquid WebSocket/REST APIs (describe endpoints, auth, rate limits).

### 4.2 Outbound

- Kafka topic `influencer_signals` (normalized signals schema).
- Raw events storage (table/collection/stream).

(Reference or link to proto/contracts definitions once defined.)

## 5. Data Contracts (TBD)

- `Signal` event schema.
- Raw event schema / storage model.

(Reference ../TECHNICAL_SPECS.md §4 and proto definitions when available.)

## 6. Configuration (TBD)

- Influencer list and connection parameters.
- Backoff/retry settings.
- Concurrency and sharding configuration.

## 7. Observability (TBD)

- Metrics (connections, messages/sec, errors, lag).
- Logs (connection lifecycle, ingestion failures, normalization errors).
- Traces (per-signal span from ingestion through publish).

## 8. Operational Considerations (TBD)

- Rate limiting and backpressure.
- Failure modes and recovery.
- Deployment, scaling, and rollout strategies.
