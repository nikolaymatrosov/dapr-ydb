# Phase 0 Research: Project Scaffold

All decisions below are grounded in version-pinned investigation of the two core SDKs
(June 2026). Each entry: **Decision → Rationale → Alternatives considered**.

## D1. Built-in vs. Pluggable component

**Decision**: Build a **pluggable** component (standalone process, gRPC over a Unix Domain
Socket via `dapr-sandbox/components-go-sdk`).

**Rationale**: Constitution Principle IV mandates pluggable; it ships and deploys without
forking or rebuilding `daprd`, which suits a private/custom DB integration.

**Alternatives considered**: Built-in component in a `components-contrib` fork (rejected —
requires rebuilding `daprd`, only justified for upstreaming); using the existing Postgres v2
component against a Postgres-wire DB (rejected — YDB is not Postgres-wire-compatible).

## D2. Pluggable SDK and its version

**Decision**: `github.com/dapr-sandbox/components-go-sdk@v0.3.0` with the bootstrap pattern
`dapr.Register("ydb", dapr.WithStateStore(factory)); dapr.MustRun()`. The store factory returns
a type implementing `components-go-sdk/state/v1.Store`, which embeds
`github.com/dapr/components-contrib/state.Store`.

**Rationale**: v0.3.0 is the latest released tag and the canonical Go pluggable SDK. The
`state/v1` interface is a thin embed of contrib's `state.Store`, so satisfying the contrib
interface is sufficient.

**Alternatives considered**: Tracking the SDK's `main` branch (rejected — unreleased, and the
official Go state-store doc page shows **stale, non-compiling** pre-1.11 signatures; we follow
the pinned source, not the doc page). Implementing the raw pluggable gRPC proto by hand
(rejected — unnecessary; the SDK handles the gRPC server, socket, and liveness/Ping).

**⚠️ Maintenance note**: The SDK has had no release since mid-2023 and pins a Dapr 1.11-era
contrib snapshot (Go 1.20 minimum). We accept this pin for the contract surface and isolate
YDB logic behind our own package so an SDK swap later is localized.

## D3. Interface surface the store must implement

**Decision**: Implement the full `state.Store` = `BaseStore` + `BulkStore`:
- `Init(ctx, state.Metadata) error`
- `Features() []state.Feature`
- `Get(ctx, *GetRequest) (*GetResponse, error)`
- `Set(ctx, *SetRequest) error`
- `Delete(ctx, *DeleteRequest) error`
- `GetComponentMetadata() map[string]string` **(required at the pinned version)**
- `BulkGet/BulkSet/BulkDelete` — satisfied by embedding `state.NewDefaultBulkStore(self)`.

Optional `TransactionalStore.Multi` and `Querier.Query` are **not** implemented in the scaffold
and their features are **not** advertised.

**Rationale**: Matches the verified interface at SDK v0.3.0. `DefaultBulkStore` delegates bulk
ops to single-key ops (same approach as the Postgres/Mongo components), minimizing scaffold
code. Constitution Principle I forbids advertising unimplemented features.

**Alternatives considered**: Hand-writing bulk methods now (rejected — premature; delegate via
the helper). Advertising `FeatureETag`/`FeatureTransactional` early (rejected — Principle I).

**Important correction vs. the original brief**: There is **no `Ping` method** on the contrib
`state.Store` at this version. Health/liveness is served by the SDK gRPC framework, so spec
FR-004 needs no method from us.

## D4. Health/liveness

**Decision**: Rely on the SDK-provided gRPC health/Ping handling; implement nothing.

**Rationale**: The pluggable gRPC contract's liveness is handled by the SDK framework, not the
component, at the pinned version. The Dapr sidecar's discovery + health check (spec US1/FR-004)
is satisfied by simply running the SDK server.

**Alternatives considered**: Implementing a custom `Ping` (rejected — not part of the interface
at this version; would not compile/serve as expected).

## D5. YDB client SDK and version

**Decision**: `github.com/ydb-platform/ydb-go-sdk/v3` (pin a specific v3.140.x tag). Connect
with `ydb.Open(ctx, dsn, opts...)`, close with `driver.Close(ctx)`. Use the **Query service**
(`db.Query()`) for all data access in later features; not the legacy `db.Table()`.

**Rationale**: v3 is the maintained line; the Query service is the currently recommended API
and supports interactive serializable transactions needed later for `Multi`/ETags. Requires
Go 1.24, which our toolchain (go1.26.1) satisfies.

**Alternatives considered**: `db.Table()` legacy client (rejected — Query service is the
forward path); `database/sql` wrapper (rejected for the hot path — native driver gives better
session-pool control; may still be used in tests if convenient).

## D6. Go version & dependency-compatibility risk

**Decision**: Set our module to `go 1.24`. Let Go module MVS resolve gRPC/protobuf to the
highest version required across the SDK (2023-era) and ydb-go-sdk (2026-era) graphs.

**Rationale**: `ydb-go-sdk/v3` requires Go ≥1.24; the SDK's `go 1.20` is only a minimum, so no
conflict on the language version.

**⚠️ Risk**: `components-go-sdk@v0.3.0` pinned `grpc v1.54.0` (2023). Pulling a 2026
`ydb-go-sdk` will bump gRPC/protobuf well beyond that via MVS. The SDK uses stable gRPC
server-registration APIs, so this **should** compile, but it is unverified until the module is
assembled. **Mitigation order if the build breaks**: (a) pin a slightly older `ydb-go-sdk/v3`
that aligns with a compatible gRPC; (b) add targeted `replace`/`exclude` directives; (c) as a
last resort, vendor or fork the small SDK shim. This risk is surfaced as an explicit early
build task in tasks.md.

**✅ RESOLVED (2026-06-18, task T002)**: The spike assembled the module with both SDKs.
MVS upgraded `google.golang.org/grpc` v1.54.0 → **v1.78.0** and `google.golang.org/protobuf`
v1.30.0 → **v1.36.10** (plus `ydb-go-sdk/v3 v3.140.2`, `ydb-go-genproto`, `golang.org/x/net
v0.48.0`, etc.). All five key import packages — `components-go-sdk`,
`components-go-sdk/state/v1`, `components-contrib/state`, `ydb-go-sdk/v3`, and
`ydb-go-sdk/v3/query` — **compile cleanly** against the bumped gRPC/protobuf. **No mitigation
was required**; the 2023 SDK is source-compatible with the 2026 gRPC. Module pinned to `go 1.24`
(MVS raised it to `1.24.0`).

## D11. Upgrade components-contrib and dapr/dapr to latest

**Decision**: Upgrade `github.com/dapr/components-contrib` to **v1.18.0** and
`github.com/dapr/dapr` to **v1.18.1** (both latest), keeping `components-go-sdk` at v0.3.0.

**Rationale**: Aligns the state-contract types and pluggable gRPC proto with the daprd runtime
we deploy against (1.18.1), rather than the 2023 (1.11-era) snapshot the SDK originally pinned.
Despite the SDK being unchanged since 2023, its `state/v1` embed of `contribState.Store` and its
proto usage **compile and run cleanly** against the 1.18 dependencies.

**Verified (2026-06-18)**: After the bump — `go build ./...`, `go vet ./...`, unit tests,
`golangci-lint`, and the conformance harness compile all pass. Runtime re-verified with daprd
**1.18.1**: the component registers (`dapr.proto.components.v1.StateStore`), loads
(`Component loaded: ydb-state (state.ydb/v1)`), `Init` connects to YDB, `/v1.0/healthz` returns
204, and stubbed ops surface the honest `not implemented` error through the full stack.

**Consequence**: minimum Go rose **1.24 → 1.26.4** (driven by `dapr/dapr` v1.18 / `dapr/kit`
v0.18); gRPC `v1.78.0 → v1.80.0`. The Go toolchain auto-downloads 1.26.4 as needed.

**Alternatives considered**: Staying on the 1.11-era pin (rejected — drifts from the runtime and
forgoes newer contrib fixes); also moving `components-go-sdk` to `main` (deferred — no released
tag, and v0.3.0 already works against 1.18, so the extra risk is unwarranted for now).

## D7. Storage schema & semantics (design intent for later features)

**Decision**: Single key/value table — `key Utf8` PK, `value String` (opaque bytes),
`etag Utf8`, `expires_at Timestamp` with **native YDB TTL** (`TTL = Interval("PT0S") ON
expires_at`). Reads will additionally filter `expires_at IS NULL OR expires_at >
CurrentUtcTimestamp()` because YDB TTL purge is eventual. ETags are opaque random UUIDs;
mismatches return `state.NewETagError(state.ETagMismatch, ...)`. Values stored/returned as raw
`[]byte` (no JSON assumptions).

**Rationale**: Mirrors the constitution (Principles I & III) and the Postgres-v2 semantics the
brief calls out (UUID ETags for engines without a natural version column; raw bytes; TTL with
read-time filtering). YDB has **native** TTL, so we prefer it over a hand-rolled GC goroutine —
but still filter on read for correctness against eventual purge.

**Scaffold scope**: The schema is **documented** (data-model.md, contracts/) and the table-
creation is designed, but real read/write wiring and TTL/ETag enforcement are implemented in
later features. The scaffold only opens the connection and validates config.

**Alternatives considered**: Expiry column + background GC goroutine (rejected as primary —
YDB's native TTL is simpler and authoritative; the GC pattern is only needed for engines
lacking native TTL); storing `value` as `Utf8`/JSON (rejected — Principle III requires raw
bytes, and `String` is YDB's binary type).

## D8. Configuration model

**Decision**: Manifest-driven config parsed in `Init` from `state.Metadata.Properties`. Initial
fields (finalized in data-model.md / contracts/metadata.yaml):
`connectionString` (required), `tableName` (default `dapr_state`), `useTLS`,
and auth fields (`authToken` / `serviceAccountKeyPath` / `useMetadataCredentials` /
`username`+`password`) — exactly one auth mode. Validation fails fast at `Init` with
field-named errors; never panics.

**Rationale**: Constitution Principle V (manifest-only config, actionable startup validation).
YDB DSN format is `grpc[s]://endpoint/database`; auth maps to `ydb.With*Credentials` options.

**Alternatives considered**: Reading env vars directly (rejected — Principle V). Separate
endpoint+database fields instead of a single `connectionString` (deferred — a single DSN
matches YDB conventions and the console-provided string; can be revisited).

## D9. Conformance & test strategy

**Decision**: A `tests/conformance` harness importing
`github.com/dapr/components-contrib/tests/conformance/state`, driven against a YDB container via
`deploy/docker-compose.yml`. Unit tests cover metadata parse/validate. `make conformance` runs
the suite for advertised features.

**Rationale**: Constitution Principle II makes conformance the authoritative gate. Wiring it now
(US3) ensures every later feature is validated immediately. With no features advertised yet, the
suite asserts the honest minimal baseline.

**Alternatives considered**: Deferring conformance wiring until persistence exists (rejected —
Principle II and spec US3 require the harness from the start). Mock-only tests for persistence
(rejected — Principle II/workflow requires a real YDB instance for persistence paths).

## D10. Module path / repository identity

**Decision**: `github.com/nikolaymatrosov/dapr-ydb`.

**Rationale**: No git remote is configured; this matches the local repo (`dapr-ydb`) and the
git user. It is a single-edit change if the canonical remote differs.

**Alternatives considered**: A placeholder like `example.com/dapr-ydb` (rejected — avoids a
realistic import path); waiting for a remote (rejected — would block scaffolding).

## Open risks carried forward

1. **gRPC/protobuf version skew** between the 2023 pluggable SDK and the 2026 YDB SDK (D6) —
   verified only when the module is assembled; first build task must confirm or trigger the
   mitigation ladder.
2. **`query.TxActor` exact method set** and `yc.*` credential helper signatures — confirm on
   pkg.go.dev at the pinned tag when persistence/auth features land (not needed for scaffold).
3. **SDK staleness** (D2) — accepted; YDB logic is isolated to localize a future SDK swap.
