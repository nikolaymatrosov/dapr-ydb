# dapr-ydb

Two **pluggable [Dapr](https://dapr.io) components** backed by [YDB](https://ydb.tech)
(Yandex Database), served by a single binary and discovered by the Dapr sidecar over a Unix
Domain Socket — **no rebuild of `daprd` required**:

- **`state.ydb`** — a state store (`Get`/`Set`/`Delete` with ETag).
- **`bindings.ydb`** — an output binding exposing `query`/`exec` over raw YQL.

> **State store status: Get/Set/Delete with ETag.** It loads in Dapr, validates its
> configuration, opens a YDB connection, and **idempotently creates the state table**. It
> implements real `Get`/`Set`/`Delete` (and bulk via the default bulk store) against the
> documented KV schema, with **optimistic-concurrency ETag** semantics. Per the project
> [constitution](.specify/memory/constitution.md), `Features()` advertises only conformance-
> verified capabilities — currently **`ETag`** only. TTL, transactions, and query are not yet
> implemented and are **not** advertised.
>
> **Binding status: query/exec.** The output binding runs arbitrary YQL and is verified by an
> integration test against a real YDB instance. `Operations()` advertises exactly `query` and
> `exec`. Both components share the same connection/auth configuration.

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
| `authMethod` | no | `anonymous` | `anonymous` / `static` / `token` / `serviceAccountKey` / `metadata` |
| `username`, `password` | when `static` | — | static credentials (`password` is sensitive) |
| `accessToken` | when `token` | — | IAM/access token (sensitive) |
| `serviceAccountKeyPath` | when `serviceAccountKey` | — | Yandex Cloud SA key file (IAM tokens auto-refreshed) |
| `useInternalCA` | no | `false` | append YC root CAs |

Invalid or missing configuration fails `Init` with a message naming the offending field; it
never crashes the sidecar. The connection/auth fields are parsed by the shared
[internal/ydbconfig](internal/ydbconfig) package and accepted identically by both components.
The binding reuses every field above except `tableName`; see [bindings.metadata.yaml](bindings.metadata.yaml).

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

## Query/Exec output binding (`bindings.ydb`)

The binding runs raw YQL through the standard Dapr binding API. The statement and parameters
travel in the request **metadata**; it mirrors the contrib `bindings/postgres` `query`/`exec`
contract. Every field is documented in [bindings.metadata.yaml](bindings.metadata.yaml).

| Operation | Behavior |
|-----------|----------|
| `query` | Executes a row-returning YQL statement; response `data` is a JSON array of row objects keyed by column name (`[]` when no rows match). |
| `exec` | Executes a non-row-returning YQL statement (DML or DDL); response `metadata` carries an execution summary. DDL needs `queryMode: scheme`. |

**Request metadata fields**: `sql` (required, the YQL statement), `params` (optional, see below),
`queryMode` (optional, `exec` only: `data` (default) / `scheme` / `scripting`).

**Parameters** — `params` is a JSON string, discriminated by kind:

- **Positional** (postgres-style): a JSON **array** bound to `?` placeholders; types are inferred
  (integral number → `Int64`, fractional → `Double`, string → `Utf8`, bool → `Bool`).

  ```json
  { "sql": "SELECT id, name FROM t WHERE id = ?", "params": "[1]" }
  ```

- **Named + typed** (exact types): a JSON **object** keyed by `$name`, each value
  `{"type": "<YdbType>", "value": <json>}`. Use this when JSON inference cannot pick the intended
  type (e.g. `Uint64`, `Timestamp`, `Date`, `String` bytes). Supported types: `Bool`, `Int32`,
  `Uint32`, `Int64`, `Uint64`, `Float`, `Double`, `Utf8`/`Text`, `String`/`Bytes` (base64), `Json`,
  `Timestamp`, `Date`, `Datetime`.

  ```json
  { "sql": "SELECT * FROM t WHERE id = $id", "params": "{\"$id\": {\"type\": \"Uint64\", \"value\": 42}}" }
  ```

Parameter values are always bound as data and can never alter the statement structure. An exact
`rows-affected` count is best-effort (YDB does not surface it the way postgres does) and may be
reported as `unknown`. See [quickstart](specs/004-query-exec-binding/quickstart.md) for `curl` examples.

## Test

```bash
make lint                 # golangci-lint
make test                 # unit tests (cmd/, internal/); no YDB required
make conformance          # brings up YDB, runs the Dapr state conformance suite (build tag: conformance)
make binding-integration  # brings up YDB, runs the output-binding integration tests (build tag: integration)
```

The conformance harness ([tests/conformance](tests/conformance)) runs Dapr's state conformance
suite — the basic CRUD scenarios plus the `etag` optimistic-concurrency scenarios — against a real
YDB container. It is the authoritative gate for the advertised `ETag` capability.

The binding integration suite ([tests/integration](tests/integration)) exercises `query`, `exec`
(DML + DDL), positional and named+typed parameters, an injection-style value treated as data, and
the unknown-operation / missing-statement / invalid-YQL error paths against a real YDB instance — the
authoritative gate for the binding. Coverage grows as each further feature is implemented and verified.

## Project layout

```text
cmd/daprd-ydb/        # entrypoint: dapr.Register("ydb", WithStateStore, WithOutputBinding) + MustRun()
internal/ydbconfig/   # shared connection/auth config (Config, Parse, CredentialOptions, Open)
internal/ydbstate/    # YDBStore (state.Store): store.go, operations.go (Get/Set/Delete + CAS),
                      #   queries.go (YQL), schema.go (idempotent DDL), etag.go, metadata.go
internal/ydbbinding/  # YDBBinding (bindings.OutputBinding): binding.go (Init/Invoke/Operations),
                      #   params.go (positional + named/typed), result.go (rows -> JSON)
components/           # sample component manifests
metadata.yaml         # state-store metadata schema
bindings.metadata.yaml# output-binding metadata schema (operations + shared config fields)
tests/conformance/    # Dapr state conformance harness (build tag: conformance)
tests/integration/    # output-binding integration tests (build tag: integration)
deploy/               # docker-compose (local YDB) + Dockerfile
```

To add a new state operation, implement it on `YDBStore` (operation bodies live in
`internal/ydbstate/operations.go`, YQL in `queries.go`), advertise its capability in `Features()`
**only after** it passes conformance, and update `metadata.yaml` if it introduces new configuration.

## License

See repository.
