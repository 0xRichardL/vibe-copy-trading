# Execution Planner Service ("planner") – Technical Specification

> This is a service-level spec that extends the system-level design in ../TECHNICAL_SPECS.md (see sections 1.4, 2, and 3).

## 1. Overview

- **Service name:** planner
- **Primary role:** Turn execution requests into concrete order parameters per subscriber, applying allocation and risk rules.
- **Tech stack:** Go (performance-critical core).

## 2. Responsibilities (Summary)

- Consume `execution_requests` from the message bus.
- For each (subscriber, signal), compute order parameters (side, size, leverage, price behavior).
- Apply subscription and subscriber risk/allocation rules.
- Ensure idempotency per (subscriber, signal) pair.
- Publish execution jobs/orders to downstream workers.

> See system-level responsibilities in ../TECHNICAL_SPECS.md §1.4.

## 3. Architecture & Dependencies (TBD)

- **Runtime:** Go.
- **Message bus:** Kafka topics `execution_requests`, `execution_jobs` (or equivalent pattern).
- **Config:** Read-only config from config-api or cache.
- **Libraries:** libs/go/domain, libs/go/messaging, libs/go/observability.

(Detail internal modules, package layout, and dependency graph here.)

## 4. Interfaces

### 4.1 Inbound

- Kafka topic `execution_requests`.

### 4.2 Outbound

- Kafka topic `execution_jobs` (or enriched `execution_requests`).

(Reference or link to proto/contracts definitions once defined.)

## 5. Data Contracts (TBD)

- `ExecutionRequest` and `ExecutionJob` schemas.
- Risk and allocation configuration models.

## 6. Idempotency & Deduplication (TBD)

- Idempotency keys and storage.
- Handling of retries and duplicate messages.

## 7. Observability (TBD)

- Metrics (requests/sec, jobs/sec, risk rejections, errors).
- Logs (decision logs for risk checks and allocation).
- Traces (planner spans within the end-to-end signal → execution trace).

## 8. Operational Considerations (TBD)

- Backpressure and throughput tuning.
- Failure modes and recovery plans.
- Deployment and scaling strategy.
