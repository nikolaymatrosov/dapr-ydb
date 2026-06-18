# Phase 1 Data Model: Project Scaffold

This scaffold **defines** the data model but only **wires** the configuration entity and the
connection lifecycle. The persisted KV schema is documented here as the contract that later
persistence features implement; the scaffold does not yet perform real reads/writes.

## Entity: Store Configuration (`storeMetadata`)

Parsed in `Init` from `state.Metadata.Properties` (the manifest `spec.metadata` map). Validated
fail-fast with field-named errors; never panics (constitution Principle V).

| Field (manifest key) | Go type | Required | Default | Validation |
|----------------------|---------|----------|---------|------------|
| `connectionString` | `string` | yes | — | non-empty; must parse as a YDB DSN (`grpc://` or `grpcs://`, host, database path) |
| `tableName` | `string` | no | `dapr_state` | valid YDB table identifier |
| `authMethod` | `string` (enum) | no | `anonymous` | one of `anonymous`, `static`, `token`, `serviceAccountKey`, `metadata` |
| `username` | `string` | conditional | — | required when `authMethod=static` |
| `password` | `string` (secret) | conditional | — | required when `authMethod=static` |
| `accessToken` | `string` (secret) | conditional | — | required when `authMethod=token` |
| `serviceAccountKeyPath` | `string` | conditional | — | required when `authMethod=serviceAccountKey`; file must exist |
| `useInternalCA` | `bool` | no | `false` | parseable bool; appends Yandex Cloud root CAs when true |

**Validation rules**:
1. `connectionString` MUST be present and DSN-parseable, else `Init` returns an error naming
   `connectionString`.
2. Exactly one auth mode is configured; required sub-fields for the selected `authMethod` MUST
   be present, else error naming the missing field.
3. Unknown/booleans that fail to parse return an error naming the field and the expected form.
4. Secret fields (`password`, `accessToken`) MUST be marked as secrets in `metadata.yaml` so the
   runtime/operator handle them appropriately.

**Lifecycle**:
- `Init`: parse + validate → open `ydb.Open(ctx, connectionString, authOpts...)` → store driver
  handle on the `YDBStore` value. On any failure, return the error (do not crash the host).
- Shutdown: the driver is closed cleanly (`driver.Close(ctx)`); no orphaned resources
  (spec FR-013, constitution Principle IV).

## Entity: Key/Value Record (persisted; design contract for later features)

YDB table created idempotently (schema management is explicit per constitution). Scaffold ships
the DDL design; table creation/migration is implemented with the first persistence feature.

| Column | YDB type | Meaning |
|--------|----------|---------|
| `key` | `Utf8` (PRIMARY KEY) | Dapr state key (the runtime composes `appID||key`) |
| `value` | `String` | opaque raw `[]byte` value — never parsed as JSON (Principle III) |
| `etag` | `Utf8` | opaque optimistic-concurrency token (random UUID per write) |
| `expires_at` | `Timestamp` (nullable) | TTL anchor; `NULL` = never expires |

**DDL (reference)**:

```sql
CREATE TABLE dapr_state (
    key        Utf8,
    value      String,
    etag       Utf8,
    expires_at Timestamp,
    PRIMARY KEY (key)
) WITH (
    TTL = Interval("PT0S") ON expires_at
);
```

**Semantics the schema encodes (enforced by later features, not the scaffold)**:
- **ETag / optimistic concurrency**: writes carry an optional ETag; mismatch →
  `state.NewETagError(state.ETagMismatch, ...)`, malformed → `ETagInvalid`. New ETag is a random
  UUID per successful write.
- **TTL**: native YDB TTL purges expired rows eventually; reads MUST additionally filter
  `expires_at IS NULL OR expires_at > CurrentUtcTimestamp()` so logically-expired rows are never
  returned before purge.
- **Value as bytes**: stored/returned as raw `[]byte` via the `String` column.
- **Transactions**: when `Multi`/`FeatureTransactional` is later advertised, operations run in a
  single YDB serializable interactive transaction (`db.Query().DoTx`).

**State transitions** (per key): `absent → present(etag₁) → present(etag₂) … → absent`
(via Delete or TTL purge). Each Set with a matching/empty ETag advances to a new opaque etag.

## Advertised Features (scaffold)

`Features()` returns an **empty** slice in the scaffold — no `FeatureETag`,
`FeatureTransactional`, `FeatureTTL`, or `FeatureQueryAPI` until each is actually implemented
and conformance-verified (constitution Principle I). This list grows feature-by-feature.
