# Subscription Matcher Service ("matcher") – Technical Specification

> This is a service-level spec that extends the system-level design in ../TECHNICAL_SPECS.md (see sections 1.3, 2, and 3).

## 1. Overview

- **Service name:** matcher
- **Primary role:** Consume normalized influencer signals, resolve active subscriptions, apply filters, and emit execution requests.
- **Tech stack:** Go / Golang (performance-critical core).

## 2. Responsibilities (Summary)

- Consume `influencer_signals` from the message bus.
- Resolve active subscriptions for each `influencer_id`.
- Apply filters (markets, status, pre-risk checks).
- Emit one `execution_request` per matched (subscriber, signal) pair.

> See system-level responsibilities in ../TECHNICAL_SPECS.md §1.3.

## 3. Architecture & Dependencies (TBD)

- **Runtime:** Go / Golang.
- **Message bus:** Kafka topics `influencer_signals`, `execution_requests`.
- **Database:** Redis (primary data store/cache for matcher state and subscriptions lookup).
- **Config:** Read-only config from the shared config library/store (no standalone configuration API service in MVP).
- **Libraries:** libs/go/domain, libs/go/messaging, libs/go/observability.

(Detail internal modules, package layout, and dependency graph here.)

### 3.1 Project Layout (Go Packages)

The matcher service is organized as a small Go module with a single binary and internal packages, mirroring the ingestion service layout:

- `/matcher/main.go`: service entry point; initializes configuration, logging/observability, and starts the application.
- `/matcher/internal/app.go`: application wiring and lifecycle management; constructs Kafka consumers/producers, Redis clients, and core services, then runs the main loop.
- `/matcher/internal/config/...`: configuration loading from environment / shared config (Redis, Kafka topics, consumer group, keys).
- `/matcher/internal/services/...`: business-logic services, including:
  - signal matching service (subscription resolution, filters, sizing, ExecutionRequest construction),
  - subscription lookup service (Redis-backed reads and caching),
  - any auxiliary services (metrics hooks, health checks).
- `/matcher/internal/domain/...`: domain models and interfaces shared within matcher (e.g., Subscription and related types).
- `/matcher/internal/kafka/...`: Kafka adapters for consuming `influencer_signals` and publishing `execution_requests`.
- `/matcher/internal/store/...`: Redis-backed repositories for subscriptions and related matcher state.

## 4. Interfaces

### 4.1 Inbound

- Kafka topic `influencer_signals` (normalized signals schema) produced by the ingestion service.

### 4.2 Outbound

- Kafka topic `execution_requests` (execution request schema).

(Reference or link to proto/contracts definitions once defined.)

## 5. Data Contracts

- `ExecutionRequest` schema.
- Subscription model and filter configuration model.

### 5.1 Subscription Model

The matcher operates on a logical **Subscription** entity that links a follower (subscriber) to an influencer and defines how signals should be copied.

- `subscription_id`: stable unique identifier for the subscription.
- `influencer_id`: identifier of the source influencer whose signals are copied.
- `subscriber_id`: identifier of the follower account that will receive executions.
- `status`: enum (e.g., ACTIVE, PAUSED, CANCELLED) used to determine eligibility for matching.
- `allowed_markets`: optional list of market symbols / instruments this subscription applies to; empty means "all markets".
- `size_mode`: enum describing sizing semantics (e.g., NOTIONAL, SIZE_FACTOR, FIXED_SIZE).
- `size_value`: numeric parameter whose interpretation depends on `size_mode` (e.g., notional amount, multiplier vs influencer size, or fixed quantity).
- `max_notional_per_signal`: optional per-signal notional cap for risk limiting.
- `max_open_notional`: optional cap on total open exposure created by this subscription.
- `leverage`: optional leverage override or multiplier relative to influencer leverage, if applicable.
- `created_at` / `updated_at`: timestamps for auditing and replay.

The matcher resolves the set of ACTIVE subscriptions for a given `influencer_id`, applies any market and risk filters (e.g., `allowed_markets`, caps), and generates one `ExecutionRequest` per (subscriber, signal) pair that passes all checks.

### 5.2 ExecutionRequest Model (Outbound)

The matcher emits an **ExecutionRequest** for each (subscriber, signal) pair that passes all filters. This is the payload on the `execution_requests` Kafka topic.

- `execution_request_id`: stable unique identifier for idempotency and de-duplication downstream.
- `signal_id`: identifier of the originating influencer signal.
- `influencer_id`: identifier of the influencer that produced the signal.
- `subscriber_id`: identifier of the follower account that will execute the trade.
- `subscription_id`: identifier tying the request back to the subscription that produced it.
- `market`: symbol / instrument identifier (e.g., BTC-PERP, ETH-PERP).
- `side`: enum (BUY / SELL / OPEN / CLOSE) depending on venue semantics.
- `order_type`: enum (e.g., MARKET, LIMIT) describing desired order style.
- `quantity`: base asset quantity to trade, if applicable.
- `notional`: notional size in quote currency, if applicable.
- `price`: optional limit price when `order_type` requires it.
- `leverage`: effective leverage to apply, if supported by the venue.
- `time_in_force`: enum (e.g., GTC, IOC, FOK) for order lifetime semantics.
- `risk_checks_passed`: boolean or enum indicating pre-risk evaluation result at match time.
- `rejection_reason`: optional string/enum populated if `risk_checks_passed` is false.
- `source`: enum/tag describing the upstream source (e.g., MATCHER_V1).
- `created_at`: timestamp when the execution request was created.
- `trace_id` / `correlation_id`: identifiers for end-to-end tracing and debugging.

Downstream services (planner, worker, and execution adapters) consume `ExecutionRequest` messages and translate them into venue-specific orders while preserving idempotency guarantees via `execution_request_id`.

## 6. Flows

### 6.1 Subscription Adding / Updating Flow

This flow describes how new subscriptions are created or existing ones are modified and made visible to the matcher.

1. A user or upstream subscription service creates/updates a Subscription (see §5.1) via an external API/UI.
2. The subscription service persists the Subscription in the system of record and projects a matcher-friendly representation into Redis (e.g., hash or JSON blob keyed by `subscription_id` and indexed by `influencer_id`).
3. The matcher maintains a read-only view of subscriptions by querying Redis:
   - On-demand lookups by `influencer_id` during matching, and/or
   - Periodic background refresh / warm-up of caches for hot influencers.
4. When a subscription is created, updated, or cancelled, the subscription service updates Redis accordingly (e.g., status, market filters, sizing parameters).
5. Subsequent signals for the affected `influencer_id` immediately see the updated subscription state (subject to Redis propagation and any local cache TTLs).

### 6.2 Matching Flow

This flow describes how an inbound influencer signal is transformed into one or more ExecutionRequests.

1. Ingestion publishes a normalized influencer signal to the `influencer_signals` Kafka topic.
2. The matcher Kafka consumer (part of this service) receives the message and deserializes it into the internal signal domain model.
3. The matcher resolves all ACTIVE subscriptions for the signal's `influencer_id` using Redis indices/lookups.
4. For each candidate Subscription, the matcher applies filters:
   - Status (must be ACTIVE).
   - Market filters (e.g., `allowed_markets`).
   - Basic pre-risk checks (e.g., `max_notional_per_signal`, `max_open_notional`).
5. For each Subscription that passes filters, the matcher computes the desired trade size (quantity/notional) based on `size_mode`, `size_value`, leverage, and the signal payload.
6. The matcher constructs an `ExecutionRequest` (see §5.2) including identifiers, market/side/order parameters, and risk evaluation flags.
7. The matcher publishes one message per (subscriber, signal) pair to the `execution_requests` Kafka topic, ensuring idempotency via `execution_request_id` and any necessary producer semantics.
8. Downstream services (planner, worker, execution adapters) consume `ExecutionRequest` messages and continue the lifecycle of the order.

## 7. Partitioning & Scaling (TBD)

- Consumer group strategy (partition by influencer or market).
- Sharding and horizontal scaling patterns.

## 8. Observability (TBD)

- Metrics (signals/sec, matches/sec, fan-out ratio, errors).
- Logs (filter decisions, drops, and failures).
- Traces (per-signal matching spans in the end-to-end trace).

## 9. Operational Considerations (TBD)

- Backpressure and lag handling.
- Behavior under partial outages (config store, bus, storage).
- Deployment and scaling strategy.
