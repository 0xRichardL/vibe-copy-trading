# Execution Worker Service ("worker") – Technical Specification

> This is a service-level spec that extends the system-level design in ../TECHNICAL_SPECS.md (see sections 1.5, 2, and 3).

## 1. Overview

- **Service name:** worker
- **Primary role:** Consume execution jobs, submit orders to Hyperliquid (or produce external execution instructions), and track execution lifecycle.
- **Tech stack:** Go (performance-critical core).

## 2. Responsibilities (Summary)

- Consume `execution_jobs` from the message bus.
- Submit orders to Hyperliquid order APIs.
- Implement retries with backoff, rate limiting, and circuit breakers.
- Track order lifecycle and persist audit logs.
- Publish `execution_results` back to the bus.

> See system-level responsibilities in ../TECHNICAL_SPECS.md §1.5.

## 3. Architecture & Dependencies (TBD)

- **Runtime:** Go.
- **Message bus:** Kafka topics `execution_jobs`, `execution_results`.
- **External APIs:** Hyperliquid order/trade APIs.
- **Libraries:** libs/go/hyperliquid, libs/go/messaging, libs/go/observability, libs/go/domain.

(Detail internal modules, package layout, and dependency graph here.)

## 4. Interfaces

### 4.1 Inbound

- Kafka topic `execution_jobs`.

### 4.2 Outbound

- Hyperliquid HTTP/WebSocket APIs.
- Kafka topic `execution_results`.
- Execution/audit log storage.

(Reference or link to proto/contracts definitions once defined.)

## 5. Data Contracts (TBD)

- `ExecutionJob` schema.
- `ExecutionResult` schema.
- Audit log record model.

## 6. Reliability & Rate Limiting (TBD)

- Retry policy and circuit breaker configuration.
- Per-account and global rate limits.
- Handling of partial failures and timeouts.

## 7. Observability (TBD)

- Metrics (jobs/sec, success/failure rates, retries, rate-limit hits).
- Logs (request/response summaries, error conditions, audit events).
- Traces (execution spans including external API calls).

## 8. Operational Considerations (TBD)

- Graceful shutdown and in-flight job handling.
- Rollout/rollback strategy.
- On-call runbook notes.
