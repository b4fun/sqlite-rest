# Design: Embed Litestream for SQLite replication

## Background

`sqlite-rest` currently opens a local SQLite database file and serves RESTful access to it. There is no built-in durability story beyond a single node. [Litestream](https://litestream.io/) provides streaming WAL replication and restore for SQLite. Litestream ships a Go library that can be embedded to continuously replicate a database file to durable object storage and restore it at startup. This document proposes how to integrate that library without changing the external REST API.

## Goals

- Offer optional replication for the served SQLite database using the Litestream Go library.
- Provide an opt-in configuration surface (CLI flags/env) to:
  - Restore a database from a configured Litestream replica before the server starts handling traffic.
  - Continuously replicate WAL/snapshots to one or more replicas (initially a single replica).
- Align lifecycle with the existing `serve` command: replication should start/stop with the process and respect graceful shutdown.
- Expose basic observability for replication health (log + Prometheus counters/gauges).

## Non-goals

- Implementing multi-writer/leader election; replication is single-writer with read-only restores.
- Changing the REST API surface or authentication model.
- Building a full Litestream CLI wrapper (only the embedded library flows we need).

## Current state and constraints

- The server opens the database via `openDB` using a DSN passed to `serve`.
- Metrics and pprof servers already share the process lifecycle and respect the same `done` channel.
- Docker image and CLI use a single database file on local disk; WAL mode is implicitly enabled by the SQLite driver.

## Proposed approach

### High-level flow

1. **Configuration** (new `ReplicationOptions`):
   - `--replication-enabled` (bool, default false).
   - `--replication-replica-url` (string, required when enabled; supports Litestream URLs like `s3://bucket/path` or `file:///...` for local testing; multi-replica support would likely rename this to `--replication-replica-urls` or move to a config file).
   - `--replication-snapshot-interval` / `--replication-retention` (optional tuning, passed through to Litestream).
   - `--replication-restore-from` (optional override to restore from a different replica URL).
   - `--replication-restore-interval` (duration, default `0` meaning latest; limits how far back to search for a snapshot when restoring).
   - `--replication-restore-lag` (duration, default `0` meaning no lag allowed; can be set to tolerate small staleness before triggering a restore).
   - Env var mirrors for container use (e.g., `SQLITEREST_REPLICATION_ENABLED`, etc.).

2. **Restore before serving**:
   - If enabled, run a Litestream restore for the configured database path **before** opening the DB handle used by `sqlite-rest`.
   - Restore should be idempotent (skip when the local DB is already ahead) and respect a configurable `--replication-restore-interval` / `--replication-restore-lag` window to avoid long restores on healthy primaries.

3. **Start replication alongside the server**:
   - After opening the DB (once restore is done), create a Litestream replicator instance bound to the same database path and replica URL.
   - Start replication in a goroutine using the same `done` channel used by the HTTP/metrics/pprof servers for coordinated shutdown.
   - Ensure the replicator stops cleanly on context cancellation and flushes pending WAL frames.

4. **Observability**:
   - Log key lifecycle events (restore start/finish, replicate start/stop, errors).
   - Add Prometheus metrics (e.g., `replication_last_snapshot_timestamp`, `replication_bytes_replicated_total`, `replication_errors_total`, `replication_lag_seconds`) populated via Litestream stats callbacks or polling the replicator state.

5. **Failure handling**:
   - If restore fails: abort startup with a clear error.
   - If replication fails at runtime: surface errors via logs/metrics but keep the HTTP server running; rely on process restarts or admin action to recover.

### API surface changes

- Extend `ServerOptions` (or adjacent option struct) with `ReplicationOptions` and bind new CLI flags on `serve`.
- Keep defaults disabled to avoid changing existing deployments.
- No changes to request handlers or DB query path.

### Configuration mapping

- **S3**: use Litestream’s S3 replica driver; accept AWS creds via standard env vars (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_REGION`) and allow custom endpoint for MinIO. Document minimal IAM needs (typically `s3:PutObject`, `s3:GetObject`, `s3:ListBucket`, and `s3:DeleteObject` for the configured prefix) so operators can keep replication credentials least-privileged.
- **File**: support `file://` URLs for local/dev validation.
- Future: allow multiple replicas by expanding the flag surface (e.g., adding `--replication-replica-urls` or a config file section); initial scope is a single replica to minimize surface area.

### Lifecycle integration sketch

```go
restoreIfNeeded(ctx, dbPath, restoreURL, restoreOpts)
db := openDB(...)
replicator := newReplicator(dbPath, replicaURL, tuneOpts)
go replicator.Start(ctx) // ctx tied to serve command cancellation
go metricsServer.Start(ctx)
go pprofServer.Start(ctx)
server.Start(ctx.Done())
// Error handling: monitor replicator error channel/state changes; log and increment metrics,
// and optionally trigger process shutdown if replication is marked as required. On error channel
// receive, cancel the shared context to shut down servers when degraded starts are disallowed.
```

### Testing strategy (future implementation)

- Unit: flag parsing → `ReplicationOptions` defaults/validation.
- Integration (temporary files): start a litestream replicator pointing to a `file://` replica, perform writes via HTTP handlers, assert replica files advance (e.g., WAL or snapshot count).
- Restore path: seed replica, delete local DB, start server with `--replication-enabled --replication-restore-from <replica>`, assert DB is restored before serving.
- Metrics: expose fake replicator stats and assert Prometheus gauges/counters are set.

## Migration & compatibility

- Replication is opt-in; existing CLI invocations keep current behavior.
- Docker image remains the same; enabling replication requires supplying new flags/env and storage credentials.

## Open questions

- Should we expose multiple replicas at launch or keep single-replica until requested?
- How strict should startup be when replication is enabled but the remote is unreachable? **Recommendation:** fail fast by default to avoid running without configured durability, with an explicit `--replication-allow-degraded` escape hatch if operators need to accept the data-loss risk.
- What are the sensible defaults for snapshot/retention to balance durability and cost?

## Implementation plan (for future PRs)

1. Add `ReplicationOptions` with CLI/env bindings and validation.
2. Add restore step before `openDB` in `serve`.
3. Wire Litestream replicator lifecycle to the server context and add metrics/logging.
4. Add targeted tests and minimal docs/README snippet for enabling replication.
