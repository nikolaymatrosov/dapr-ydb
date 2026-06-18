# dapr-ydb

A **pluggable [Dapr](https://dapr.io) state store** backed by [YDB](https://ydb.tech)
(Yandex Database). It runs as its own process and is discovered by the Dapr sidecar over a
Unix Domain Socket — **no rebuild of `daprd` required**.

> **Status: Get/Set/Delete with ETag.** The component loads in Dapr, validates its
> configuration, opens a YDB connection, and **idempotently creates the state table**. It
> implements real `Get`/`Set`/`Delete` (and bulk via the default bulk store) against the
> documented KV schema, with **optimistic-concurrency ETag** semantics. Per the project
> [constitution](.specify/memory/constitution.md), `Features()` advertises only conformance-
> verified capabilities — currently **`ETag`** only. TTL, transactions, and query are not yet
> implemented and are **not** advertised.

## Prerequisites

- Go **1.26+**
- Docker (for a local YDB instance and the conformance suite)
- Dapr CLI / `daprd` **1.11+** (to load the component)
- [`golangci-lint`](https://golangci-lint.run) (for `make lint`)

## Build

```bash
make build        # -> ./bin/daprd-ydb
```

## Run and load in Dapr

```bash
# 1. Start a local YDB
make ydb-up       # docker compose up -d ydb  (grpc://localhost:2136/local)

# 2. Run the pluggable component (creates ydb.sock in the sockets folder)
make run          # uses DAPR_COMPONENTS_SOCKETS_FOLDER (default /tmp/dapr-components-sockets)

# 3. In another shell, start daprd pointed at ./components
daprd --app-id demo --resources-path ./components --dapr-grpc-port 50001
```

`daprd` discovers `ydb.sock`, loads the component as `state.ydb`, and reports it healthy.
The registered socket name (`ydb` in [cmd/daprd-ydb/main.go](cmd/daprd-ydb/main.go)) maps to the
manifest `spec.type: state.ydb` — see [components/ydb.yaml](components/ydb.yaml).

## Configuration

The store is configured entirely through the component manifest. Every field is documented in
[metadata.yaml](metadata.yaml). Summary:

| Field | Required | Default | Notes |
|-------|----------|---------|-------|
| `connectionString` | yes | — | YDB DSN `grpc[s]://endpoint/database` |
| `tableName` | no | `dapr_state` | backing table name |
| `authMethod` | no | `anonymous` | `anonymous` / `static` / `token` (wired); `serviceAccountKey` / `metadata` (documented, later feature) |
| `username`, `password` | when `static` | — | static credentials (`password` is sensitive) |
| `accessToken` | when `token` | — | IAM/access token (sensitive) |
| `serviceAccountKeyPath` | when `serviceAccountKey` | — | YC SA key file (later feature) |
| `useInternalCA` | no | `false` | append YC root CAs |

Invalid or missing configuration fails `Init` with a message naming the offending field; it
never crashes the sidecar.

## State operations

| Operation | Behavior |
|-----------|----------|
| `Get` | Returns the value (raw bytes) and current ETag, or an empty not-found response. Logically-expired rows (past `expires_at`) are never returned. |
| `Set` | Unconditional upsert, or optimistic compare-and-set when an ETag is supplied. Every successful write assigns a fresh opaque ETag (random UUID). |
| `Delete` | Idempotent delete (absent key succeeds), or compare-and-delete when an ETag is supplied. |
| `BulkGet`/`BulkSet`/`BulkDelete` | Delegated to the single-key operations via the default bulk store. |

**ETag semantics**: a write/delete carrying an ETag that does not match the stored token — including
a malformed token — is rejected with `state.ETagMismatch` and leaves data unchanged (this matches the
Dapr conformance suite and the contrib Postgres v2 component). Values are stored and returned as raw
bytes and are never parsed. The compare-and-set runs in a single YDB serializable transaction.

## Test

```bash
make lint          # golangci-lint
make test          # unit tests (cmd/, internal/); YDB-backed tests skip if no YDB is reachable
make conformance   # brings up YDB, runs the Dapr state conformance suite (build tag: conformance)
```

The conformance harness ([tests/conformance](tests/conformance)) runs Dapr's state conformance
suite — the basic CRUD scenarios plus the `etag` optimistic-concurrency scenarios — against a real
YDB container. It is the authoritative gate for the advertised `ETag` capability. Coverage grows as
each further persistence feature (TTL, transactions, query) is implemented and verified.

## Project layout

```text
cmd/daprd-ydb/      # entrypoint: dapr.Register("ydb", ...) + dapr.MustRun()
internal/ydbstate/  # YDBStore (state.Store): store.go, operations.go (Get/Set/Delete + CAS),
                    #   queries.go (YQL), schema.go (idempotent DDL), etag.go, metadata.go
components/         # sample component manifest
metadata.yaml       # component metadata schema (documents all config fields)
tests/conformance/  # Dapr state conformance harness (build tag: conformance)
deploy/             # docker-compose (local YDB) + Dockerfile
```

To add a new state operation, implement it on `YDBStore` (operation bodies live in
`internal/ydbstate/operations.go`, YQL in `queries.go`), advertise its capability in `Features()`
**only after** it passes conformance, and update `metadata.yaml` if it introduces new configuration.

## License

See repository.
