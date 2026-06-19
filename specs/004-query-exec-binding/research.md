# Phase 0 Research: YDB Query/Exec Output Binding

All decisions below were validated against the vendored dependency sources in the module cache
(`components-go-sdk@v0.3.0`, `components-contrib@v1.18.0`, `ydb-go-sdk/v3@v3.140.2`).

## D1 — Component type & registration

**Decision**: Implement a Dapr **output binding** (`bindings.OutputBinding`) and register it in the
**same** `dapr.Register("ydb", …)` call as the state store, via `dapr.WithOutputBinding`.

**Rationale**: `registry.go` accumulates each `WithX` option onto a `useGrpcServer []func(*grpc.Server)`
slice and applies them all to one gRPC server, so a single socket can serve multiple component
types. daprd routes by `spec.type` (`state.ydb` vs `bindings.ydb`) + name, so both coexist under
socket name `ydb` with no new binary and no `daprd` rebuild (constitution Principle IV). The
referenced postgres `query`/`exec` operations are themselves an output binding, confirming the
component type.

**Alternatives considered**:
- *Extend `state.Store`* (e.g. implement `Querier`): rejected — the Dapr state Query API is a
  JSON filter language, not raw SQL; it cannot express arbitrary YQL / DDL and would not match the
  postgres `query`/`exec` contract.
- *Separate second binary*: rejected — unnecessary; the SDK serves multiple component types per
  process, and one binary is simpler to deploy.

## D2 — Statement execution layer (`database/sql` vs native query service)

**Decision**: Execute statements via the YDB **`database/sql`** layer:
`sql.OpenDB(ydb.MustConnector(driver, ydb.WithAutoDeclare(), ydb.WithPositionalArgs()))`, where
`driver` is the `*ydb.Driver` opened from shared config (D6). Use `WithNumericArgs()` is *not*
combined simultaneously (positional `?` is the documented postgres-compatible placeholder); numeric
`$1` support is a possible future toggle, out of scope.

**Rationale**: The positional parameter model (Clarifications) requires turning an untyped JSON
array into typed YDB query parameters. `WithPositionalArgs()` rewrites `?` placeholders into named
YQL params and `WithAutoDeclare()` emits the `DECLARE` statements by inferring YDB types from the
Go argument values — precisely the postgres-style behavior, for free. The native query service
(`driver.Query()`, used by the state store) only accepts pre-typed `ydb.ParamsBuilder()` params and
has no `?`/auto-declare path, so choosing it would mean re-implementing type inference by hand.

**Alternatives considered**:
- *Native query service + hand-rolled JSON→YDB type inference*: rejected — duplicates exactly what
  `WithAutoDeclare` already does, more code and more bugs.
- *Numeric (`$1`) placeholders via `WithNumericArgs`*: deferred — postgres binding uses positional;
  `?` is the closest analogue. Revisit only if a user needs `$n`.

**Validation required**: the precise interaction of `WithPositionalArgs`/`WithAutoDeclare` with the
named+typed path and with DDL must be confirmed by the integration test (D5, the conformance gate).

## D3 — DDL / query-mode handling for `exec`

**Decision**: For `exec`, run through the `database/sql` connector and, when a statement is DDL
(scheme) rather than DML, select scheme mode via the SDK's per-request context helper
`ydb.WithQueryMode(ctx, ydb.SchemeQueryMode)`. Default `exec` mode is data-query mode; the binding
chooses scheme mode for statements YDB requires it for. (The state store already runs DDL through
the native query service with `NoTx`, confirming YDB DDL cannot run inside an interactive
transaction.)

**Rationale**: `sql.go` exposes `QueryMode` (`DataQueryMode`, `ScanQueryMode`, `SchemeQueryMode`, …)
and `WithQueryMode(ctx, mode)`. DDL (`CREATE`/`DROP`/`ALTER`) must run in `SchemeQueryMode` and
outside a transaction. Selecting the mode per request keeps `query` (read) and `exec` (write/DDL)
correct without a global setting.

**Open detail for implementation**: how the binding decides "this exec is DDL" — options are (a) a
lightweight statement-prefix check (`CREATE`/`DROP`/`ALTER`/…), or (b) an explicit optional
`queryMode` request-metadata field. Lean toward (b) explicit `queryMode` (values: `data`, `scheme`,
`scripting`) defaulting to `data`, because prefix-sniffing YQL is fragile. Final choice validated by
the integration test creating/dropping a table.

## D4 — Result serialization (`query` → JSON)

**Decision**: For `query`, read rows via `database/sql` `Rows`, use `rows.Columns()` for column
names, `rows.Scan` into a `[]any` (one `*any` per column), and build `[]map[string]any` (one map
per row, keyed by column name), then `json.Marshal` into `InvokeResponse.Data` with
`ContentType: "application/json"`. An empty result set serializes to `[]` (FR-010). NULL → JSON
`null`. `[]byte` columns marshal to base64 strings (Go `encoding/json` default), documented in the
contract.

**Rationale**: Mirrors the postgres binding's "JSON array of row objects." Column-name keying is the
natural, language-agnostic shape callers parse. Using `[]any` scan targets lets the YDB driver pick
Go types; documenting the JSON mapping (numbers, bool, base64 bytes, RFC3339 timestamps) makes the
contract testable (SC-003).

**Alternatives considered**:
- *Array-of-arrays + separate column list*: rejected — less ergonomic for callers than named keys
  and diverges from the postgres binding shape.

## D5 — Verification strategy (the gate)

**Decision**: Authoritative correctness gate is an **integration test against a real/containerized
YDB instance** (reuse the project's existing test harness under `tests/`), exercising: `query`
returning rows, `query` returning zero rows (`[]`), `exec` INSERT then `query` verify, `exec`
CREATE/DROP TABLE, positional parameters, named+typed parameters, an injection-style value treated
as data (SC-005), unknown-operation error, and missing-statement error. Plus fast unit tests
(no DB) for param construction, result serialization, and error mapping.

**Rationale**: The Dapr **state** conformance suite does not cover bindings, but the constitution's
Development-Workflow rule ("integration tests MUST run against a YDB instance, not a mock, for
persistence paths") and spec FR-014/SC-002 make a real-DB integration test the equivalent objective
gate. Unit tests cover pure logic cheaply; the integration test covers the contract.

**Alternatives considered**:
- *contrib bindings conformance suite*: usable in principle but heavier to wire for a pluggable
  component; a targeted integration test covers the same advertised behavior with less scaffolding.
  May be adopted later.

## D6 — Shared connection/auth configuration

**Decision**: Extract the state store's metadata parsing (`parseAndValidateMetadata`) and credential
mapping (`credentialOptions`) into a new `internal/ydbconfig` package exposing an exported `Config`,
`Parse(props map[string]string) (Config, error)`, `CredentialOptions(Config) ([]ydb.Option, error)`,
and an `Open(ctx, Config) (*ydb.Driver, error)` helper. Both `ydbstate` and `ydbbinding` consume it.
The binding then derives its `database/sql` DB from the opened driver (D2).

**Rationale**: FR-012 requires the binding to reuse the same manifest fields and auth — including
the Yandex Cloud auth methods from feature 003. Duplicating security-sensitive credential logic
across two components is a maintenance and correctness hazard; a single shared package keeps them in
lockstep and keeps each `metadata.yaml` honest. `state.Metadata` and `bindings.Metadata` both embed
`metadata.Base{Properties map[string]string}`, so a parser over `map[string]string` serves both.

**Sequencing / risk**: relocating `credentialOptions` intersects feature 003's in-flight work;
perform the extraction after 003's credential logic is final, and treat it as behavior-preserving —
the state **conformance** suite passing post-refactor is the regression gate (Principle II). Each
component still opens its **own** `*ydb.Driver` in its own `Init` (independent pluggable instances);
sharing is of *configuration/code*, not of a live connection.

## D7 — Request/response field names (postgres alignment)

**Decision**: Statement in request metadata under `sql` (matching the postgres binding). Parameters
under `params` (a JSON string): a JSON **array** ⇒ positional; a JSON **object** ⇒ named+typed,
where each value is `{"type": "<YdbType>", "value": <json>}`. Optional `queryMode` for `exec`
(D3). Response: `query` → `Data` = JSON row array, `Metadata{operation, sql, ...timing}`; `exec` →
`Data` empty, `Metadata{operation, sql, rows-affected?(best-effort), ...timing}`.

**Rationale**: Reusing postgres's `sql`/`params` names minimizes surprise for users migrating from
the postgres binding. Type-discriminating `params` by JSON kind (array vs object) cleanly expresses
the "one form or the other" rule (FR-006) in a single field. `rows-affected` is best-effort because
YDB does not surface it the way postgres `CommandTag` does (spec Assumptions).

**Alternatives considered**:
- *Separate `params` (positional) and `paramsTyped` (named) fields*: rejected — two fields invite
  "both supplied" ambiguity; JSON-kind discrimination is simpler and unambiguous.
- *Rename `sql` → `yql`/`query`*: rejected — `sql` maximizes postgres familiarity; documented as
  "the YQL statement."
