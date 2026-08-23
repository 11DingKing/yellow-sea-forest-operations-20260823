# Yellow Sea Forest Operations Service

A Go backend for operating the Yellow Sea coastal forest. It tracks a stewardship case from ranger or community intake through ecological survey, restoration planning, field work, inspection, visitor opening, and a follow-up window. The workflow reflects the forest's three-generation hand-off: protect the forest first, then make visitor, education, lodging, understory-economy, and carbon-finance services dependable.

## Runtime model

The service uses SQLite in WAL mode with foreign keys, serializable units of work, optimistic versions, durable idempotency records, and an outbox worker with expiring leases. The schema is migrated during startup and remains durable across process restarts. Forest sites, parcels, managed assets, surveys, stewardship plans, field operations, inspections, and audit events are relational records rather than in-memory counters.

Roles are intentionally separated:

- `resident_liaison`: represents nearby residents and visitor communities when reporting a forest issue.
- `officer`: assigns stewardship cases, approves restoration plans, and closes inspection rounds.
- `inspector`: records ecological surveys and performs post-work inspections.
- `operator`: proposes plans and claims field operations under a bounded lease.
- `admin`: manages forest sites, parcels, and operational assets.

The stewardship lifecycle is:

```text
submitted -> triaged -> surveying -> stewardship_planned -> executing
          -> inspecting -> resolved
                          \-> reopened -> surveying
```

## Requirements

- Go 1.23 or later in the Go 1.23 toolchain line
- Docker 24 or later for container execution
- No external database is required

## Local startup

Set a bootstrap administrator on the first launch:

```bash
export FOREST_BOOTSTRAP_ADMIN_EMAIL=admin@example.com
export FOREST_BOOTSTRAP_ADMIN_PASSWORD='replace-with-a-long-local-password'
go run ./cmd/server
```

The service listens on `:8080`. Liveness is at `/livez`; readiness checks database connectivity and the applied schema at `/readyz`.

Create a session:

```bash
curl -sS http://127.0.0.1:8080/v1/session \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@example.com","password":"replace-with-a-long-local-password"}'
```

Use the returned token as `Authorization: Bearer <token>`. Mutating stewardship intake and reopening endpoints also require an `Idempotency-Key` header.

## Configuration

| Variable | Default | Purpose |
| --- | --- | --- |
| `FOREST_ADDR` | `:8080` | HTTP listen address |
| `FOREST_DATABASE_URL` | SQLite file under `data/` | SQLite DSN and pragmas |
| `FOREST_SESSION_TTL` | `12h` | Session lifetime |
| `FOREST_WORKER_INTERVAL` | `2s` | Outbox poll interval |
| `FOREST_WORKER_LEASE` | `30s` | Durable job lease |
| `FOREST_WORKER_TIMEOUT` | `20s` | Per-job context deadline |
| `FOREST_WORKER_BATCH_SIZE` | `20` | Maximum jobs claimed per poll |
| `FOREST_WORKER_COUNT` | `4` | Maximum parallel handlers |
| `FOREST_SHUTDOWN_TIMEOUT` | `10s` | HTTP graceful shutdown window |
| `FOREST_LOG_LEVEL` | `info` | JSON log level |
| `FOREST_BOOTSTRAP_ADMIN_EMAIL` | empty | Optional first administrator |
| `FOREST_BOOTSTRAP_ADMIN_PASSWORD` | empty | Password paired with bootstrap email |

Bootstrap credentials are only used to create a missing administrator. They do not overwrite an existing account.

## Verification

```bash
make fmt
make vet
make test
make test-race
make build
```

The tests cover state transitions, authorization, survey aggregation, inspection decisions, transaction rollback, database restart recovery, optimistic concurrency, operation leases, outbox retries, dead-letter behavior, and bounded worker concurrency.

## Docker

Build and run the Linux image:

```bash
docker compose up --build
```

The runtime image is non-root, persists the SQLite database in the `light-data` volume, and exposes a readiness-based health check. Change the example bootstrap password before using the compose file outside local development.

## API workflow

1. Create users for the operational roles through `POST /v1/admin/users`.
2. Create a forest site, parcel, and managed assets through the admin endpoints.
3. Submit a stewardship case through `POST /v1/cases` with an idempotency key.
4. Assign the case, start an ecological survey, add at least three readings, and publish it.
5. Propose a stewardship plan with one or more field operations, obtain operator acceptance, then officer approval.
6. Each operation is claimed by its operator and completed before its lease expires. Completion atomically updates the managed asset and operation state.
7. Start and complete an inspection round. A round only passes when the observed metric is within the approved threshold and the community agrees.
8. A resolved case remains eligible for reopening during the published follow-up window.

All responses are JSON except successful no-content operations. Domain validation maps to `422`, authentication to `401`, authorization to `403`, missing resources to `404`, optimistic/state/lease conflicts to `409`, and dependency failures to `503`.
