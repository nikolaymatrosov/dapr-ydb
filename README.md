# dapr-ydb

A **pluggable [Dapr](https://dapr.io) state store** backed by [YDB](https://ydb.tech)
(Yandex Database). It runs as its own process and is discovered by the Dapr sidecar over a
Unix Domain Socket — **no rebuild of `daprd` required**.

> **Status: scaffold.** The component loads in Dapr, validates its configuration, and opens a
> YDB connection. Persistence operations (`Get`/`Set`/`Delete`, bulk, ETags, TTL, transactions,
> query) are **stubbed** and delivered by subsequent features. Per the project
> [constitution](.specify/memory/constitution.md), `Features()` advertises only capabilities
> that are actually implemented — currently **none**.

## Prerequisites

- Go **1.24+**
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

## Test

```bash
make lint          # golangci-lint
make test          # unit tests (cmd/, internal/) — no YDB needed
make conformance   # brings up YDB, runs the Dapr state conformance suite (build tag: conformance)
```

The conformance harness ([tests/conformance](tests/conformance)) is wired to Dapr's state
conformance suite. Because the scaffold advertises no features, it currently skips with a clear
message; coverage grows as each persistence feature is implemented.

## Project layout

```text
cmd/daprd-ydb/      # entrypoint: dapr.Register("ydb", ...) + dapr.MustRun()
internal/ydbstate/  # YDBStore (state.Store), metadata parsing/validation, errors
components/         # sample component manifest
metadata.yaml       # component metadata schema (documents all config fields)
tests/conformance/  # Dapr state conformance harness (build tag: conformance)
deploy/             # docker-compose (local YDB) + Dockerfile
```

To add a new state operation, implement it on `YDBStore` in `internal/ydbstate/store.go`,
advertise its capability in `Features()` **only after** it passes conformance, and update
`metadata.yaml` if it introduces new configuration.

## License

See repository.
