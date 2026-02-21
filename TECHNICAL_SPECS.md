## Vibe Copy Trading Core – Technical Specification

This document translates the product requirements in [PRD.md](PRD.md) into a concrete technical design for the Vibe Copy Trading Core.

---

## 1. Logical Components → Services

The logical components from the PRD are realized as the following stateless services plus shared infrastructure. All services are designed to be horizontally scalable behind a message bus and/or HTTP APIs.

### 1.1 Signal Ingestion Service ("ingestion")

- **Responsibility**
  - Maintain WebSocket/REST connections to Hyperliquid for configured influencer addresses.
  - Ingest trades, orders, and/or position updates.
  - Ensure idempotent processing via event IDs / timestamps and deduplication logic.
  - Persist raw events to an append-only event store (hot storage tier).
  - Normalize events into canonical `Signal` objects and publish to the message bus topic `influencer_signals`.
- **Key interfaces**
  - **Inbound**: Hyperliquid APIs (WebSocket + REST for backfill).
  - **Outbound**:
    - Message bus: `influencer_signals` (normalized signals).
    - Storage: raw events table / stream (for 90+ days retention via tiering).
    - Metrics + tracing exporters.
- **Scaling model**
  - Stateless workers each handle a subset of influencers (partitioned by `influencer_id` hash or explicit sharding configuration).
  - Automatic reconnect/backoff per connection; connection pools per region.

### 1.2 Configuration Service ("config-api")

- **Responsibility**
  - Provide read-only access to configuration for Influencers, Subscribers, and Subscriptions.
  - Cache configuration in-memory and/or via a distributed cache for low-latency lookups.
  - Support cache invalidation and periodic refresh (e.g., polling an external config DB or receiving change events).
- **Key interfaces**
  - **Inbound**:
    - Admin/external config source (out of scope for this core, assumed to be a DB or config service).
    - Internal GRPC/HTTP from matcher/execution services (read-only calls).
  - **Outbound**:
    - Underlying config store (e.g., Postgres, MySQL, or key-value store).
- **Scaling model**
  - Stateless API nodes behind a load balancer.
  - Aggressive caching of read operations (configuration is low-churn, read-heavy).

### 1.3 Subscription Matcher Service ("matcher")

- **Responsibility**
  - Consume `influencer_signals` from the message bus.
  - For each signal, determine all active subscriptions for `influencer_id`.
  - Apply filters (markets, status, risk pre-checks) on the signal.
  - Emit one `execution_request` per matched (subscriber, signal) pair.
- **Key interfaces**
  - **Inbound**: message bus topic `influencer_signals`.
  - **Outbound**:
    - Message bus topic `execution_requests`.
    - Read-only access to Configuration Service (GRPC/HTTP) or shared cache.
    - Metrics and tracing exporters.
- **Scaling model**
  - Consumer group on `influencer_signals` partitioned by `influencer_id` or market.
  - Horizontally scalable by adding more matcher instances to the consumer group.

### 1.4 Execution Planner Service ("planner")

- **Responsibility**
  - Consume `execution_requests` from the message bus.
  - For each request, compute concrete order parameters:
    - Side, size, leverage, price behavior.
    - Risk and allocation rules based on subscription and subscriber configuration.
  - Ensure idempotency per (subscriber, signal) pair to avoid duplicates.
  - Publish `execution_jobs` (can reuse `execution_requests` topic with a sub-type or dedicated `execution_jobs` topic) for workers.
- **Key interfaces**
  - **Inbound**: message bus topic `execution_requests`.
  - **Outbound**:
    - Message bus topic `execution_jobs`.
    - Read-only Configuration Service.
    - Metrics and tracing exporters.
- **Scaling model**
  - Stateless planners in a consumer group on `execution_requests` (partitioned by `subscriber_id` or `influencer_id`).

### 1.5 Execution Worker Service ("worker")

- **Responsibility**
  - Consume `execution_jobs` from the bus.
  - Submit orders to Hyperliquid (or generate instructions for external executors, depending on integration model).
  - Implement retries with exponential backoff, circuit breakers, and rate-limiting.
  - Track order lifecycle (placement, amendment, cancellation), and persist an audit log.
  - Publish `execution_results` events back to the bus.
- **Key interfaces**
  - **Inbound**: message bus topic `execution_jobs`.
  - **Outbound**:
    - Hyperliquid order APIs.
    - Message bus topic `execution_results`.
    - Storage for executions/audit logs.
- **Scaling model**
  - Horizontally scalable workers with global/per-account rate limiting.
  - Concurrency limits and token-bucket throttling to honor Hyperliquid and account constraints.

### 1.6 Analytics Processor ("analytics-processor")

- **Responsibility**
  - Consume `influencer_signals` and `execution_results` streams.
  - Build influencer-level, subscriber-level, and system-level metrics.
  - Write data into analytics-friendly storage (OLAP DB or data warehouse).
- **Key interfaces**
  - **Inbound**: `influencer_signals`, `execution_results` (and optionally raw events).
  - **Outbound**: analytics store (e.g., ClickHouse, BigQuery, Snowflake, or columnar DB).
- **Scaling model**
  - Stateless stream processors or micro-batch jobs.

### 1.7 Observability Stack ("observability")

- **Responsibility**
  - Centralized metrics (Prometheus/OpenTelemetry collector).
  - Centralized logging (e.g., Loki/ELK, structured JSON logs).
  - Distributed tracing backend (e.g., Tempo/Jaeger).
- **Key interfaces**
  - **Inbound**: OTLP traces/metrics, structured logs from all services.
  - **Outbound**: dashboards (Grafana), alerting channels.

### 1.8 Shared Libraries

- **`proto` / `contracts`**
  - Shared schemas for message bus topics and service APIs (e.g., Protobuf).
- **`pkg-sdks`**
  - Internal client libraries (e.g., Hyperliquid SDK, config client, tracing helpers, metrics helpers) used across services.

---

## 2. Monorepo Layout

The system is organized as a polyglot monorepo with clear boundaries between services, shared libraries, and infrastructure.

### 2.1 Top-Level Structure

```text
vibe-copy-trading/
	PRD.md
	TECHNICAL_SPECS.md

	services/
		ingestion/            # Signal Ingestion Service (Go)
		config-api/           # Configuration API (Go)
		matcher/              # Subscription Matcher (Go)
		planner/              # Execution Planner (Go)
		worker/               # Execution Worker (Go)
		analytics-processor/  # Analytics stream processor (Go)

	libs/
		go/
			hyperliquid/        # Hyperliquid client + models
			configclient/       # Config service client
			messaging/          # Message bus abstraction
			observability/      # Metrics, logging, tracing helpers
			domain/             # Shared domain types: Signal, Execution, etc.
		ts/
			sdk/                # TypeScript SDK for external consumers (future frontend/API)

	proto/
		bus/                  # Protobuf schemas for message topics
		api/                  # Service API definitions (e.g., config-api)

	infra/
		k8s/                  # Kubernetes manifests / Helm charts
		terraform/            # Cloud infra (message bus, databases, observability stacks)
		docker/               # Base Dockerfiles and build tooling

	tools/
		ci/                   # CI/CD pipelines, lint/test tools
		scripts/              # Local dev scripts (bootstrap, dev env)

	config/
		base/                 # Base configs shared across envs
		dev/
		staging/
		prod/
```

### 2.2 Build and Dependency Management

- **Golang**
  - Use a Go workspace (`go.work`) to manage multiple Go modules (one per service or a small set of service + shared libs modules).
  - Example modules:
    - `services/ingestion`
    - `services/matcher`
    - `libs/go/domain`
- **Typescript**
  - Use `pnpm` or `yarn` workspaces for TypeScript libraries (starting with `libs/ts/sdk`).
  - Shared TypeScript config via `tsconfig.base.json`.
- **Proto / contracts**
  - `buf` or `protoc`-based pipeline to generate Go and TypeScript clients from `.proto` definitions into respective `libs/` directories.

### 2.3 Testing & Quality Gates

- Each service has:
  - Unit tests (Go `*_test.go`).
  - Integration tests using local containers (e.g., Docker Compose for message bus & DBs).
- CI pipeline:
  - Lint + format (golangci-lint, gofmt, eslint/prettier for TS libs).
  - Run tests per service.
  - Build Docker images.
  - Optionally run smoke tests against an ephemeral environment.

---

## 3. Language Choices (Go and TypeScript)

We will use **Golang** for all core backend services in this monorepo and **TypeScript** for SDKs and future user-facing or external integration layers.

### 3.1 Golang for Core Services

**Services in Go**

- `ingestion`, `config-api`, `matcher`, `planner`, `worker`, `analytics-processor`.

**Reasons**

- **High-concurrency, low-latency**
  - Goroutines and channels map well to high-throughput event pipelines and I/O-bound workloads (WebSockets, message bus, HTTP APIs).
  - Garbage collector and runtime are mature for long-running services.
- **Predictable performance and resource usage**
  - Compiled binaries, relatively small memory footprint versus many Node.js processes at the same throughput.
  - Easier capacity planning for services with strict P95/P99 latency and throughput SLOs.
- **Ecosystem fit**
  - Strong tooling for Kubernetes, microservices, and observability (OpenTelemetry, Prometheus, gRPC).
  - Rich ecosystem for message buses (Kafka, NATS, etc.) and database drivers.
- **Operational simplicity**
  - Single static binary per service, simple container images.
  - Easier cross-platform builds and smaller runtime surface area.

### 3.2 TypeScript for SDKs and External Integrations

**Components in TypeScript**

- `libs/ts/sdk`: Client SDKs for future web/mobile apps or external integrators.
- Any thin API gateway or BFF (in future phases) can be in TypeScript/Node.js if needed, using the same contracts.

**Reasons**

- **Developer experience for clients**
  - Many clients/partners and any eventual frontend will likely be TypeScript/JavaScript-based, making a TS SDK the natural fit.
- **Type safety and shared contracts**
  - Generate TypeScript types from Protobuf or OpenAPI so that client code is strongly typed and aligned with backend contracts.
- **Decoupling core from presentation**
  - Keep the latency-critical core in Go while allowing flexible, ergonomic TypeScript layers at the edges.

### 3.3 Why not TypeScript-only?

- The core requirements (100–500 signals/sec, 1,000–5,000 execution requests/sec, strict P95/P99 targets) favor a compiled, concurrency-optimized language.
- Node.js can handle high throughput, but Go’s concurrency model and binary deployment story are better aligned with long-term SRE and scaling goals.

---

## 4. Message Bus Evaluation and Choice

The PRD requires a message bus with at-least-once delivery, horizontal scalability, partitioning by influencer/market, and strong observability. We evaluate common candidates and select one.

### 4.1 Requirements Summary

- **Topics/streams**
  - `influencer_signals`, `execution_requests`, `execution_jobs` (optional), `execution_results`.
- **Semantics**
  - At-least-once delivery.
  - Consumer groups for scaling matchers, planners, and workers.
  - Ordering guarantees within a partition by `influencer_id` or `subscriber_id`.
- **Scale**
  - 100–500 signals/sec.
  - 1,000–5,000 execution requests/sec internal fan-out.
- **Operational features**
  - Strong observability (lag, throughput, error visibility).
  - Horizontal scalability.
  - Suitable for cloud-managed deployment.

### 4.2 Candidates

We compare **Kafka**, **NATS JetStream**, and **RabbitMQ**.

#### 4.2.1 Apache Kafka

- **Pros**
  - Designed for high-throughput streaming; handles far above our target load with headroom.
  - Strong partitioning model with ordered logs; easy to partition by `influencer_id` or market.
  - Consumer groups fit matcher, planner, and worker scaling patterns directly.
  - Persistent, durable storage with configurable retention.
  - Rich ecosystem: Kafka Connect (for offloading to warehouses), Schema Registry, well-known operational patterns.
  - Mature observability (consumer lag, partition metrics, broker health).
  - Many managed offerings (Confluent Cloud, AWS MSK, Azure Event Hubs Kafka-compatible).
- **Cons**
  - Operationally heavier than some alternatives if self-managed.
  - More complex for request/reply patterns (not a concern here).

#### 4.2.2 NATS JetStream

- **Pros**
  - Lightweight, simple, and fast messaging with optional persistence (JetStream).
  - Low latency, good for microservices and control-plane communications.
  - Simple client libraries and configuration.
- **Cons**
  - Ecosystem and tooling for large-scale analytics streaming, consumer lag monitoring, and connectors are less mature than Kafka’s.
  - Fewer fully managed, battle-tested offerings at scale.
  - Operational patterns for long-retention streams and heavy analytics workloads are less standardized.

#### 4.2.3 RabbitMQ

- **Pros**
  - Mature, widely-used message broker with flexible routing (exchanges + queues).
  - Strong for traditional task queues and request/reply.
- **Cons**
  - Not primarily designed as a high-throughput log/stream store; less ideal for analytics replays and long-term event history.
  - Scaling high fan-out stream workloads can be more complex than Kafka’s partition/consumer-group model.
  - Monitoring consumer lag and replay semantics is more involved compared with Kafka.

### 4.3 Chosen Message Bus: Kafka (Managed)

Given the requirements and the need for both real-time processing and historical replay for analytics, **Apache Kafka (preferably a managed offering)** is the best fit.

**Reasons**

- **Streaming-first design**
  - Kafka’s log-based storage and partitioning model match well with our need to store normalized signals and allow downstream replay (e.g., analytics, re-computation) over at least 90 days.
- **Scalable fan-out**
  - Multiple independent consumer groups (matcher, analytics, diagnostics) can consume the same stream at their own pace.
  - Easy to meet and exceed the 5,000 execution requests/sec target with moderate cluster sizing.
- **Ordering guarantees**
  - Per-partition ordering ensures deterministic handling for a given `influencer_id` or `subscriber_id`, simplifying idempotency and risk logic.
- **Ecosystem and tooling**
  - Mature observability tools for consumer lag, throughput, broker health.
  - Schema Registry for `Signal`, `ExecutionRequest`, and `ExecutionResult` event schemas, reducing schema drift and improving safety.
  - Connectors to push data into warehouses (supporting analytics requirements).
- **Managed services**
  - Using a managed Kafka (e.g., Confluent Cloud or AWS MSK) reduces operational burden and lets the team focus on core logic.

### 4.4 Topic Design

- `influencer_signals`
  - **Key**: `influencer_id` or `market`.
  - **Partitions**: Sized to support target throughput and headroom (e.g., 32–64 to start, adjustable).
- `execution_requests`
  - **Key**: `subscriber_id` (or `subscriber_id` + `influencer_id`) to ensure per-subscriber ordering.
  - **Consumers**: Execution Planner.
- `execution_jobs`
  - **Key**: `subscriber_id` / execution job ID.
  - **Consumers**: Execution Workers.
- `execution_results`
  - **Key**: `execution_id`.
  - **Consumers**: Analytics Processor, monitoring dashboards.

---

## 5. Summary

- Core backend services (ingestion, config-api, matcher, planner, worker, analytics-processor) are implemented in Go for performance, operational simplicity, and concurrency.
- The monorepo organizes services, shared libraries, contracts, and infra in a way that supports polyglot development and strong consistency of interfaces.
- TypeScript is used for SDKs and future client-facing layers, generated from shared contracts.
- Kafka (managed) is selected as the message bus due to its streaming-first design, scalability, ecosystem maturity, and fit for both real-time processing and analytics replay.
