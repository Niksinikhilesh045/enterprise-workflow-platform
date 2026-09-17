# Enterprise Workflow Platform

A portfolio-grade multi-tenant enterprise workflow platform designed to demonstrate production-oriented backend, distributed-systems, web, mobile, and cloud engineering.

## Target stack

- Go backend services
- React + TypeScript web client
- React Native mobile client
- MongoDB persistence
- Kafka event streaming
- AWS deployment
- Docker Compose for local development
- GitHub Actions CI

## Implemented milestone

The current backend foundation includes:

- Tenant-scoped workflow definitions
- Dynamic records driven by workflow schemas
- Idempotent record creation
- Optimistic concurrency control
- Tenant-isolated audit events
- HTTP API with health checks
- Unit and HTTP integration tests

The backend currently uses an in-memory store so its domain and API behavior can be validated without external infrastructure. MongoDB and Kafka adapters are added in later milestones behind the same service boundaries.

## Run the backend

```bash
cd services/api
go test ./...
go run ./cmd/server
```

Then verify:

```bash
curl http://localhost:8080/healthz
```

## Architecture

See [docs/architecture.md](docs/architecture.md).

## Development status

This repository is intentionally built incrementally with tested commits. Resume claims should only be made after the associated capability has been implemented and verified.
