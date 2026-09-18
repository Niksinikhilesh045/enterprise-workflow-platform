# Enterprise Workflow Platform

A portfolio-grade, multi-tenant workflow platform built to demonstrate production-oriented backend, distributed-systems, web, mobile, and cloud engineering.

## What is implemented

- Go HTTP API with tenant-scoped workflow definitions and records
- HMAC-signed bearer authentication with tenant-bound claims and RBAC
- Roles for admin, builder, requester, approver, and auditor
- MongoDB persistence with transactions and tenant isolation
- Idempotent record creation using `Idempotency-Key`
- Optimistic concurrency using record versions and `If-Match`
- Transactional audit + outbox writes
- Kafka outbox relay
- Kafka consumer-group worker with bounded retry and dead-letter handling
- Idempotent MongoDB notification projection for at-least-once Kafka delivery
- React + TypeScript web client
- React Native approval inbox for submitted requests
- Docker Compose development environment with MongoDB replica set and Kafka
- Multi-stage production Go container
- Terraform AWS blueprint for ECS/Fargate, ALB, ECR, CloudWatch, Secrets Manager integration, S3, and CloudFront
- GitHub Actions coverage for Go race tests, MongoDB integration, Kafka-to-Mongo integration, web build, mobile type checking, Docker image build, and Terraform validation

## Architecture

```text
React Web ------------------\
                             >--- Go API --- MongoDB
React Native Approval Inbox-/       |
                                     | transaction: record + audit + outbox
                                     v
                                  Outbox Relay
                                     |
                                     v
                                   Kafka
                                     |
                              Consumer Group Worker
                               /             \
                         Notifications       DLQ
                              (MongoDB)      (Kafka)
```

Authentication is enforced at the API boundary. Signed bearer-token claims determine the tenant and role; clients do not control authorization by supplying a tenant header.

See [docs/architecture.md](docs/architecture.md) for design details.

## Local development

Prerequisites:

- Go 1.23+
- Docker with Docker Compose
- Node.js 22+ for the web/mobile clients

Start MongoDB and Kafka:

```bash
docker compose up -d mongodb mongo-init kafka
```

Run the API with MongoDB persistence:

```bash
cd services/api
export AUTH_SECRET='local-development-secret-change-me'
export PERSISTENCE=mongo
export MONGO_URI='mongodb://localhost:27017/?replicaSet=rs0'
go run ./cmd/server
```

Generate a local token in another terminal:

```bash
cd services/api
export AUTH_SECRET='local-development-secret-change-me'
go run ./cmd/token -tenant acme -role admin
```

Health check:

```bash
curl http://localhost:8080/healthz
```

For the full requester-to-approver flow, Kafka workers, and client setup, see [docs/local-development.md](docs/local-development.md).

## Testing

Backend unit and race tests:

```bash
cd services/api
go test -race ./...
```

The GitHub Actions backend workflow also runs MongoDB transaction integration tests and a real Kafka-to-Mongo test that delivers the same event twice and verifies one idempotent notification projection.

Frontend CI performs a production React build and strict React Native TypeScript checking. Infrastructure CI builds the Go container and runs Terraform formatting, provider initialization, and validation.

## Validated local performance

A Windows local validation run completed 5,000 authenticated transactional record submissions with 50 concurrent workers and zero HTTP failures. The same run measured 815.32 req/s API ingestion and 180.45 records/s end-to-end through the MongoDB transactional outbox, Kafka, and idempotent notification projection, with zero final consumer lag.

The optimized outbox relay was observed fully published 544 ms after HTTP completion. These are local benchmark results, not production SLOs or cloud guarantees.

See [docs/performance-validation.md](docs/performance-validation.md) for the exact test profile, stage timings, caveats, and reproducible command.

## AWS deployment status

The repository contains CI-validated Terraform for an AWS deployment using ECS/Fargate, ALB, ECR, CloudWatch, Secrets Manager references, private S3, and CloudFront Origin Access Control.

**The Terraform has been validated but has not been applied to an AWS account from this repository.** Do not describe the project as deployed on AWS until an actual account deployment has been completed and verified.

See [docs/aws-deployment.md](docs/aws-deployment.md).

## Engineering properties

- Multi-tenant isolation is enforced in storage queries and signed auth claims.
- Stale writers fail with a conflict rather than overwriting newer data.
- API retries can safely replay record creation through idempotency keys.
- Database mutations, audit records, and event intent are committed together through a transactional outbox.
- Kafka consumption uses at-least-once semantics with idempotent persistence and dead-letter handling.
- CI exercises the real MongoDB replica-set and Kafka infrastructure paths rather than only mocks.

## Repository status

The project has been built incrementally through tested pull requests. Resume and interview claims should match verified capabilities in `main`; in particular, distinguish **AWS-ready/validated IaC** from an **actual AWS deployment**.
