# Subscription Matcher Service ("matcher") – Technical Specification

> This is a service-level spec that extends the system-level design in ../TECHNICAL_SPECS.md (see sections 1.3, 2, and 3).

## 1. Overview

- **Service name:** matcher
- **Primary role:** Consume normalized influencer signals, resolve active subscriptions, apply filters, and emit execution requests.
- **Tech stack:** Go (performance-critical core).

## 2. Responsibilities (Summary)

- Consume `influencer_signals` from the message bus.
- Resolve active subscriptions for each `influencer_id`.
- Apply filters (markets, status, pre-risk checks).
- Emit one `execution_request` per matched (subscriber, signal) pair.

> See system-level responsibilities in ../TECHNICAL_SPECS.md §1.3.

## 3. Architecture & Dependencies (TBD)

- **Runtime:** Go.
- **Message bus:** Kafka topics `influencer_signals`, `execution_requests`.
- **Config:** Read-only config from config-api or cache.
- **Libraries:** libs/go/domain, libs/go/messaging, libs/go/observability.

(Detail internal modules, package layout, and dependency graph here.)

## 4. Interfaces

### 4.1 Inbound

- Kafka topic `influencer_signals` (normalized signals schema).

### 4.2 Outbound

- Kafka topic `execution_requests` (execution request schema).

(Reference or link to proto/contracts definitions once defined.)

## 5. Data Contracts (TBD)

- `ExecutionRequest` schema.
- Subscription filter configuration model.

## 6. Partitioning & Scaling (TBD)

- Consumer group strategy (partition by influencer or market).
- Sharding and horizontal scaling patterns.

## 7. Observability (TBD)

- Metrics (signals/sec, matches/sec, fan-out ratio, errors).
- Logs (filter decisions, drops, and failures).
- Traces (per-signal matching spans in the end-to-end trace).

## 8. Operational Considerations (TBD)

- Backpressure and lag handling.
- Behavior under partial outages (config-api, bus, storage).
- Deployment and scaling strategy.
