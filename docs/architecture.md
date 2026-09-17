# Architecture

## Goal

The platform lets organizations define reusable internal workflows without hard-coding every use case. A workflow definition describes dynamic fields and allowed states; records are tenant-scoped instances of those definitions.

## Implemented architecture

```text
React Web ------------------\
                             >--- Go API --- MongoDB
React Native Approval Inbox-/       |
                                     | record + audit + outbox
                                     | in one MongoDB transaction
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

## Authentication and authorization

The production HTTP server requires `AUTH_SECRET` and validates HMAC-SHA256 bearer tokens. Tokens carry subject, tenant, role, and expiry claims.

Supported roles are:

- `admin`
- `builder`
- `requester`
- `approver`
- `auditor`

The tenant used for data access comes from the signed token. If a client also sends `X-Tenant-ID`, it must match the signed tenant claim. This prevents clients from selecting another tenant through an untrusted header.

## Tenant isolation

Workflow definitions, records, idempotency keys, audit events, and notification projections are tenant-scoped. MongoDB queries include the tenant identifier alongside resource identifiers. Cross-tenant record access returns the same not-found behavior as an absent record.

## Optimistic concurrency

Records contain a monotonically increasing `version`. Updates include `expectedVersion` or an `If-Match` header. MongoDB updates match both record ID, tenant ID, and the expected version. Stale writers receive HTTP 409 rather than silently overwriting newer data.

## Idempotent commands

Record creation accepts `Idempotency-Key`. A tenant-scoped unique idempotency record maps the key to the created record. A replay returns the original record and the API adds `Idempotent-Replay: true`.

## Transactional outbox

Record mutations write three things in the same MongoDB transaction:

1. the record mutation,
2. an audit event,
3. an outbox event.

The relay polls unpublished outbox events and publishes them to Kafka. Successfully published events are marked with `publishedAt`; failed attempts record retry information. This avoids committing a database state change while losing the intent to publish its corresponding event.

## Kafka delivery semantics

Kafka consumption is intentionally at-least-once. The consumer:

- joins a consumer group,
- retries transient handler failures with bounded backoff,
- publishes exhausted/malformed messages to a dead-letter topic,
- commits the Kafka message only after successful handling or successful DLQ publication.

Notification projections use the source event ID as the MongoDB document `_id`. Duplicate Kafka delivery therefore becomes an idempotent persistence operation. CI verifies this against a real Kafka broker and MongoDB instance by publishing the same event twice and asserting one notification document.

## Persistence boundary

Application behavior depends on a `Store` interface. The repository includes:

- an in-memory store for deterministic unit/HTTP tests,
- a MongoDB store for transactional persistence and integration tests.

The production server selects MongoDB when `PERSISTENCE=mongo`; otherwise it uses the in-memory implementation.

## Web and mobile clients

The React web client sends bearer tokens for protected API calls.

The React Native client implements a tenant-scoped approval inbox. It loads submitted records via `GET /api/v1/records?state=SUBMITTED` and approves them using each record's current version in `If-Match`.

## AWS blueprint

Terraform under `infra/terraform` defines an AWS-ready deployment with:

- ECR for the shared Go service image,
- ECS/Fargate services for the API, outbox relay, and event worker,
- an Application Load Balancer for the API,
- CloudWatch log groups,
- existing Secrets Manager ARNs injected into task definitions,
- a private S3 bucket for the React build,
- CloudFront with Origin Access Control.

The current portfolio blueprint uses public-IP Fargate tasks with security-group-controlled ingress to avoid NAT Gateway cost in a demonstration environment. A stricter production topology would normally place workloads in private subnets with appropriate egress and private service connectivity.

Terraform is formatted, initialized with the real AWS provider schema, and validated in CI. It has not yet been applied to an AWS account.
