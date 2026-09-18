# Validated Local Performance

This document records performance results that were actually observed on the local Windows development environment. These numbers are **not production SLOs**, cloud benchmarks, or guarantees for other hardware.

## Final end-to-end validation

Test profile:

- 5,000 authenticated record-creation requests
- 50 concurrent load-test workers
- MongoDB replica set running locally in Docker
- Kafka running locally in Docker with 3 partitions
- Go API, outbox relay, and event worker running locally
- transactional record + audit + outbox writes
- batched outbox publication to Kafka
- idempotent notification projection in MongoDB

Observed result:

```text
requests:      5000
concurrency:   50
successful:    5000
failed:        0
elapsed:       6.133s
throughput:    815.32 req/s
latency p50:   55.168ms
latency p95:   112.012ms
latency p99:   148.614ms
latency max:   209.552ms

records:        5000/5000
unpublished:    0
notifications:  5000/5000
kafka lag:      0
post-http drain: 21.575s
end-to-end:     27.708s
e2e throughput: 180.45 records/s

Pipeline stage timings
http complete:          6.133s
outbox fully published: 6.676s (544ms after HTTP)
kafka lag reached zero: 27.708s (21.031s after outbox)
notifications complete: 27.708s (21.575s after HTTP)
pipeline complete:      27.708s
```

## What the measurements support

The run supports these claims:

- the API accepted all 5,000 authenticated transactional record submissions with zero HTTP failures;
- HTTP ingestion reached 815.32 requests/second in this local run;
- the full measured pipeline completed all 5,000 corresponding notification projections with zero final Kafka consumer lag;
- measured end-to-end throughput for this run was 180.45 records/second;
- after batching Kafka publication and MongoDB outbox updates, the outbox was fully published 544 ms after the final HTTP response.

The HTTP throughput and end-to-end throughput are different measurements. The 815.32 req/s figure applies to the synchronous API ingestion phase. The 180.45 records/s figure measures completion through the asynchronous notification projection.

## Bottleneck investigation

Earlier 5,000-request tests showed roughly 3,000 unpublished outbox events immediately after the HTTP workload while Kafka consumer lag remained near zero. Inspection showed that the relay was performing one synchronous Kafka write and one MongoDB update for every event.

The relay was changed to:

- continuously drain while backlog exists,
- publish outbox events to Kafka in batches,
- bulk-update published outbox documents in MongoDB.

After that change, stage-level instrumentation showed that outbox publication was no longer the dominant drain stage: the final validation observed outbox completion only 544 ms after HTTP completion.

The remaining local drain was downstream of the relay. Kafka lag reached zero and the final notification projection completed about 21 seconds after the outbox was fully published. This repository does not claim that the downstream consumer path has been optimized further.

## Reliability properties validated separately

Performance testing is in addition to the repository's correctness tests:

- Go unit tests with the race detector in CI,
- MongoDB transaction integration tests,
- real Kafka-to-Mongo integration tests,
- duplicate Kafka delivery with idempotent notification projection,
- bounded consumer retry and DLQ handling,
- idempotent record creation,
- optimistic concurrency conflict handling.

## Reproducing the end-to-end benchmark

With the API, outbox relay, and event worker running:

```powershell
cd services/api

go run ./cmd/loadtest `
    -token $ADMIN_TOKEN `
    -requests 5000 `
    -concurrency 50 `
    -wait-for-pipeline `
    -pipeline-timeout 45s
```

The benchmark records probe-observed stage milestones. Results will vary by machine load, hardware, Docker configuration, Go version, MongoDB/Kafka state, and background processes.

## Resume/interview wording

Use the results as local validation rather than production-scale claims. A defensible summary is:

> Validated a 5,000-record authenticated transactional workload with 50 concurrent workers and zero HTTP failures; achieved 815 req/s API ingestion and 180 records/s end-to-end through MongoDB transactional outbox, Kafka, and idempotent notification projection, with zero final consumer lag; optimized batched outbox publishing to reduce post-HTTP outbox drain to approximately 0.5 seconds.

Do not describe the 815 req/s API figure as end-to-end throughput, and do not describe the project as deployed on AWS. The Terraform in this repository is validated but has not been applied to an AWS account.
