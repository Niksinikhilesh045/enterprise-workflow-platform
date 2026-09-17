# Architecture

## Goal

The platform lets organizations define reusable internal applications and workflows without hard-coding every use case. A workflow definition describes fields and allowed states; records are instances of those definitions.

## Target architecture

```text
React Web -----------\
                      >---- Go API ---- MongoDB
React Native --------/         |
                               | transactional outbox
                               v
                             Kafka
                          /     |      \
                    Audit   Notify   Analytics
```

## Core design decisions

### Tenant isolation
Every workflow, record, audit event, and idempotency key is scoped by tenant. API calls require `X-Tenant-ID`; cross-tenant record reads are deliberately indistinguishable from missing records.

### Optimistic concurrency
Records contain a monotonically increasing `version`. Updates include `expectedVersion` or `If-Match`. Stale writers receive HTTP 409 instead of silently overwriting a newer value.

### Idempotency
Create-record requests may supply `Idempotency-Key`. Replays within the same tenant return the original record and set `Idempotent-Replay: true`.

### Event consistency
The production persistence milestone will introduce a transactional outbox persisted alongside record mutations. A relay publishes outbox events to Kafka so database commits and event delivery can be reconciled safely.

### Persistence boundary
Domain behavior depends on a `Store` interface. The first milestone uses an in-memory implementation for deterministic tests; MongoDB will implement the same contract.
