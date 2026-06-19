# Phase 1 Data Model: YDB Query/Exec Output Binding

The binding is a stateless SQL pass-through; it owns no persisted schema of its own. The "data
model" here is the in-memory shape of requests, parameters, results, and configuration.

## Config (shared — `internal/ydbconfig`)

Parsed from component-manifest properties (`map[string]string`), identical fields for both
components. (Mirrors today's `storeMetadata`; `TableName` is state-store-only and stays in
`ydbstate`.)

| Field | Source property | Type | Notes |
|-------|-----------------|------|-------|
| `ConnectionString` | `connectionString` | string | Required; YDB DSN `grpc[s]://host/db`. |
| `AuthMethod` | `authMethod` | enum | `anonymous`\|`static`\|`token`\|`serviceAccountKey`\|`metadata`; default `anonymous`. |
| `Username` / `Password` | `username` / `password` | string | Required when `authMethod=static`. |
| `AccessToken` | `accessToken` | string | Required when `authMethod=token`. |
| `ServiceAccountKeyPath` | `serviceAccountKeyPath` | string | Required when `authMethod=serviceAccountKey`. |
| `UseInternalCA` | `useInternalCA` | bool | Optional; YC internal CA. |

**Validation rules** (unchanged from `ydbstate`, now shared): required-field and per-auth-method
checks return field-named errors; never panic (constitution Principle V).

## Binding invocation request

Derived from `bindings.InvokeRequest{ Operation, Metadata, Data }`.

| Element | Carrier | Required | Notes |
|---------|---------|----------|-------|
| Operation | `Operation` | yes | Must be `query` or `exec`; anything else → "operation not supported" error listing supported ops. |
| Statement | `Metadata["sql"]` | yes | The YQL statement. Missing/empty → descriptive error, no DB call (FR-011). |
| Parameters | `Metadata["params"]` | no | JSON string; **array** ⇒ positional, **object** ⇒ named+typed (see below). Absent ⇒ no params. |
| Query mode | `Metadata["queryMode"]` | no | `exec` only; `data` (default) \| `scheme` (DDL) \| `scripting`. |

`Data` (request body) is unused by `query`/`exec` (statement travels in metadata, per postgres
binding convention).

## Parameter set

Exactly one form per invocation (FR-006).

**Positional** — `params` is a JSON array; statement uses `?` placeholders bound left-to-right:

```json
{ "sql": "SELECT * FROM t WHERE id = ? AND name = ?", "params": "[42, \"alice\"]" }
```

Types are inferred from JSON values (FR-006a). Inference table (documented for callers):

| JSON value | Inferred YDB type |
|------------|-------------------|
| number (no fraction) | `Int64` |
| number (fraction/exponent) | `Double` |
| string | `Utf8` (Text) |
| boolean | `Bool` |
| null | typed NULL by position (driver-dependent; prefer named+typed for nullable typed columns) |

> Inference is lossy: every integral JSON number infers to `Int64`. To target `Uint64`,
> `Timestamp`, `Date`, `String`(bytes), `Json`, etc., use the named+typed form.

**Named + typed** — `params` is a JSON object keyed by parameter name (`$name`); each entry carries
an explicit YDB type and value; statement references `$name`:

```json
{
  "sql": "SELECT * FROM t WHERE id = $id AND created >= $since",
  "params": "{\"$id\": {\"type\": \"Uint64\", \"value\": 42}, \"$since\": {\"type\": \"Timestamp\", \"value\": \"2026-06-18T00:00:00Z\"}}"
}
```

**Supported type vocabulary** (named form): `Bool`, `Int32`, `Uint32`, `Int64`, `Uint64`, `Float`,
`Double`, `Utf8`/`Text`, `String`/`Bytes` (base64-encoded JSON string), `Json`, `Timestamp`,
`Date`, `Datetime`. An unknown type name → error naming the parameter (FR-007). A referenced
parameter with no supplied value, or a value that cannot be coerced to its declared type → error
naming the parameter, statement not executed (FR-007).

## Query result (`query` response)

`InvokeResponse.Data` = JSON array of row objects, one object per row, keyed by column name;
`ContentType = "application/json"`.

| Result situation | Serialization |
|------------------|---------------|
| N matching rows | `[{col: val, …}, …]` preserving column names |
| Zero rows | `[]` (success, not error — FR-010) |
| NULL column | JSON `null` |
| Integer/float column | JSON number |
| Bool column | JSON boolean |
| Bytes/String column | base64-encoded JSON string |
| Timestamp/Date/Datetime | RFC3339 string |

## Execution summary (`exec`, and metadata for `query`)

`InvokeResponse.Metadata`:

| Key | Operation | Meaning |
|-----|-----------|---------|
| `operation` | both | `query` or `exec`. |
| `sql` | both | The executed statement. |
| `start-time` / `end-time` | both | RFC3339Nano timestamps. |
| `duration` | both | Execution wall time. |
| `rows-affected` | `exec` | Best-effort affected-row count if the driver reports it; omitted/`unknown` otherwise (spec Assumptions). |

## Lifecycle / state

The binding holds: parsed `Config`, the opened `*ydb.Driver`, and the derived `*sql.DB`. `Init`
opens them (field-named errors, never panic); `Close` releases the `*sql.DB`/driver; `Ping`
verifies connectivity. No per-request state is retained.
