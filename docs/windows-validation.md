# Windows Validation Guide

This guide is the recommended hands-on validation path for Windows 10/11 using PowerShell, Docker Desktop, Go, and Node.js.

## 1. Prerequisites

Verify these tools are available:

```powershell
docker --version
docker compose version
go version
node --version
npm --version
git --version
```

Recommended versions for this repository:

- Go 1.23+
- Node.js 22+
- Docker Desktop with Linux containers

## 2. Clone and enter the repository

```powershell
git clone https://github.com/Niksinikhilesh045/enterprise-workflow-platform.git
cd enterprise-workflow-platform
```

If you already have the repository:

```powershell
git pull origin main
```

## 3. Start MongoDB and Kafka

From the repository root:

```powershell
docker compose up -d mongodb mongo-init kafka
```

Check status:

```powershell
docker compose ps
```

MongoDB should be healthy and Kafka should be running.

If you want to inspect startup logs:

```powershell
docker compose logs mongodb mongo-init kafka
```

## 4. Configure the backend environment

Open a PowerShell terminal:

```powershell
$env:AUTH_SECRET='local-development-secret-change-me'
$env:PERSISTENCE='mongo'
$env:MONGO_URI='mongodb://localhost:27017/?replicaSet=rs0'
$env:MONGO_DB='enterprise_workflow'
$env:KAFKA_BROKERS='localhost:9092'
```

Use the same `AUTH_SECRET` in every terminal that generates or validates tokens.

## 5. Run backend tests first

```powershell
cd services/api
go test -race ./...
```

Expected result: all packages pass.

Return to the repository root when needed:

```powershell
cd ../..
```

## 6. Start the API

Open a new PowerShell terminal, set the environment variables from step 4, then:

```powershell
cd services/api
go run ./cmd/server
```

Leave this terminal running.

In another terminal:

```powershell
Invoke-RestMethod http://localhost:8080/healthz
```

Expected result:

```text
status
------
ok
```

## 7. Start the outbox relay

Open another PowerShell terminal, set the variables from step 4, then:

```powershell
cd services/api
go run ./cmd/outbox-relay
```

Leave it running.

## 8. Start the event worker

Open another PowerShell terminal, set the variables from step 4, then:

```powershell
cd services/api
go run ./cmd/event-worker
```

Leave it running.

## 9. Generate an admin token

Open another PowerShell terminal:

```powershell
cd services/api
$env:AUTH_SECRET='local-development-secret-change-me'
$ADMIN_TOKEN = go run ./cmd/token -subject admin-1 -tenant acme -role admin
```

Confirm it is populated:

```powershell
$ADMIN_TOKEN.Length
```

## 10. Smoke-test the authenticated workflow path

Create a workflow:

```powershell
$headers = @{
    Authorization = "Bearer $ADMIN_TOKEN"
    'Content-Type' = 'application/json'
}

$workflowBody = @{
    name = 'Equipment Request'
    states = @('SUBMITTED', 'APPROVED')
    fields = @(
        @{
            key = 'request'
            label = 'Request'
            type = 'text'
            required = $true
        }
    )
} | ConvertTo-Json -Depth 5

$workflow = Invoke-RestMethod `
    -Method Post `
    -Uri 'http://localhost:8080/api/v1/workflows' `
    -Headers $headers `
    -Body $workflowBody

$workflow
```

Create a record:

```powershell
$recordHeaders = @{
    Authorization = "Bearer $ADMIN_TOKEN"
    'Content-Type' = 'application/json'
    'Idempotency-Key' = 'windows-smoke-001'
}

$recordBody = @{
    workflowId = $workflow.id
    data = @{
        request = 'MacBook Pro'
    }
} | ConvertTo-Json -Depth 5

$record = Invoke-RestMethod `
    -Method Post `
    -Uri 'http://localhost:8080/api/v1/records' `
    -Headers $recordHeaders `
    -Body $recordBody

$record
```

List submitted records:

```powershell
Invoke-RestMethod `
    -Method Get `
    -Uri 'http://localhost:8080/api/v1/records?state=SUBMITTED' `
    -Headers @{ Authorization = "Bearer $ADMIN_TOKEN" }
```

Approve the record using optimistic concurrency:

```powershell
$approveHeaders = @{
    Authorization = "Bearer $ADMIN_TOKEN"
    'Content-Type' = 'application/json'
    'If-Match' = [string]$record.version
}

$approveBody = @{ state = 'APPROVED' } | ConvertTo-Json

Invoke-RestMethod `
    -Method Put `
    -Uri "http://localhost:8080/api/v1/records/$($record.id)" `
    -Headers $approveHeaders `
    -Body $approveBody
```

## 11. Verify idempotency

Re-run the same create-record request using the same `windows-smoke-001` idempotency key.

To inspect the replay response header, use `Invoke-WebRequest`:

```powershell
$replay = Invoke-WebRequest `
    -Method Post `
    -Uri 'http://localhost:8080/api/v1/records' `
    -Headers $recordHeaders `
    -Body $recordBody

$replay.Headers['Idempotent-Replay']
```

Expected value:

```text
true
```

## 12. Run a baseline load test

The repository includes a Go load-test client that exercises authenticated record creation with unique idempotency keys.

From `services/api`:

```powershell
go run ./cmd/loadtest `
    -token $ADMIN_TOKEN `
    -requests 1000 `
    -concurrency 20
```

The tool reports:

- successful/failed requests
- elapsed time
- throughput in requests/second
- p50 latency
- p95 latency
- p99 latency
- maximum latency
- HTTP status counts

Do not put performance numbers on your resume until you have run the test on your own machine and saved the output.

## 13. Run a higher-concurrency profile

If the baseline completes with zero failures:

```powershell
go run ./cmd/loadtest `
    -token $ADMIN_TOKEN `
    -requests 5000 `
    -concurrency 50
```

Record the complete output. This is the run we can use to derive a defensible resume performance statement if it remains stable.

## 14. Verify the React web client

From the repository root in another terminal:

```powershell
cd apps/web
npm install
npm run build
npm run dev
```

Open the local Vite URL shown in the terminal, paste the relevant bearer token, and verify workflow creation and request submission.

## 15. Verify the React Native client

From another terminal:

```powershell
cd apps/mobile
npm install
npm run typecheck
npm start
```

For a physical phone, `localhost:8080` refers to the phone itself. Update the development API base URL to your computer's LAN IP before device testing.

Validate that the approval inbox:

1. loads submitted requests for the signed tenant,
2. approves a request,
3. removes the approved request from the submitted inbox,
4. reports a version conflict if a stale record version is used.

## 16. Stop local infrastructure

When testing is complete:

```powershell
docker compose down
```

To also delete local MongoDB/Kafka volumes:

```powershell
docker compose down -v
```

Only use `-v` when you intentionally want to remove test data.

## Results to share for final verification

Please keep the output from these commands:

```powershell
docker compose ps
cd services/api
go test -race ./...
go run ./cmd/loadtest -token $ADMIN_TOKEN -requests 1000 -concurrency 20
go run ./cmd/loadtest -token $ADMIN_TOKEN -requests 5000 -concurrency 50
```

Also capture any errors from the API, outbox relay, or event-worker terminals. We will use those results to decide which performance and reliability claims are justified on the resume.
