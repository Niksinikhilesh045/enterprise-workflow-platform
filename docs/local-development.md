# Local Development Runbook

This runbook reproduces the tested local architecture: MongoDB replica set, Kafka, Go API, outbox relay, event worker, React web client, and React Native approval inbox.

## 1. Start infrastructure

From the repository root:

```bash
docker compose up -d mongodb mongo-init kafka
```

MongoDB runs as a single-node replica set so transactions are available. Kafka runs locally on `localhost:9092`.

Check the containers:

```bash
docker compose ps
```

## 2. Configure backend environment

Use the same secret in every terminal that issues or validates local tokens:

```bash
export AUTH_SECRET='local-development-secret-change-me'
export PERSISTENCE=mongo
export MONGO_URI='mongodb://localhost:27017/?replicaSet=rs0'
export MONGO_DB='enterprise_workflow'
export KAFKA_BROKERS='localhost:9092'
```

On PowerShell, use `$env:NAME='value'` instead of `export`.

## 3. Start the API

```bash
cd services/api
go run ./cmd/server
```

Verify:

```bash
curl http://localhost:8080/healthz
```

Expected response:

```json
{"status":"ok"}
```

## 4. Start the asynchronous workers

Open two additional terminals, set the same backend environment variables, then run:

```bash
cd services/api
go run ./cmd/outbox-relay
```

and:

```bash
cd services/api
go run ./cmd/event-worker
```

The relay publishes transactional outbox events to Kafka. The event worker consumes record events and writes idempotent notification projections to MongoDB.

## 5. Generate role-specific tokens

From `services/api`, with `AUTH_SECRET` set:

```bash
go run ./cmd/token -subject builder-1 -tenant acme -role builder
```

```bash
go run ./cmd/token -subject requester-1 -tenant acme -role requester
```

```bash
go run ./cmd/token -subject approver-1 -tenant acme -role approver
```

```bash
go run ./cmd/token -subject auditor-1 -tenant acme -role auditor
```

For broad local testing, an admin token can access all protected capabilities:

```bash
go run ./cmd/token -subject admin-1 -tenant acme -role admin
```

## 6. Create a workflow

Use a builder or admin token:

```bash
curl -X POST http://localhost:8080/api/v1/workflows \
  -H "Authorization: Bearer $BUILDER_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "name":"Equipment Request",
    "states":["SUBMITTED","APPROVED"],
    "fields":[
      {"key":"request","label":"Request","type":"text","required":true}
    ]
  }'
```

Copy the returned workflow `id`.

## 7. Submit a record

Use a requester, builder, or admin token:

```bash
curl -X POST http://localhost:8080/api/v1/records \
  -H "Authorization: Bearer $REQUESTER_TOKEN" \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: equipment-request-001' \
  -d '{
    "workflowId":"<WORKFLOW_ID>",
    "data":{"request":"MacBook Pro"}
  }'
```

Submitting the same request again with the same tenant and idempotency key should return the original record with `Idempotent-Replay: true`.

## 8. List the approval inbox

Use an approver or admin token:

```bash
curl 'http://localhost:8080/api/v1/records?state=SUBMITTED' \
  -H "Authorization: Bearer $APPROVER_TOKEN"
```

## 9. Approve with optimistic concurrency

Use the record's current `version`:

```bash
curl -X PUT http://localhost:8080/api/v1/records/<RECORD_ID> \
  -H "Authorization: Bearer $APPROVER_TOKEN" \
  -H 'Content-Type: application/json' \
  -H 'If-Match: 1' \
  -d '{"state":"APPROVED"}'
```

Repeating an update with the stale version should return HTTP 409.

## 10. Run the React web client

```bash
cd apps/web
npm install
npm run dev
```

Paste a role-appropriate bearer token into the UI when prompted.

## 11. Run the React Native client

```bash
cd apps/mobile
npm install
npm start
```

The mobile client loads submitted requests for the tenant encoded in the bearer token. When running on a physical device, change the API base URL from `localhost` to the development computer's LAN address.

## 12. Run automated verification

Backend race tests:

```bash
cd services/api
go test -race ./...
```

MongoDB and Kafka integration tests are also run by GitHub Actions using real containers. The Kafka integration test publishes the same event twice and verifies that MongoDB contains exactly one notification projection.

## 13. Stop local infrastructure

```bash
docker compose down
```

To also delete local MongoDB/Kafka volumes:

```bash
docker compose down -v
```
