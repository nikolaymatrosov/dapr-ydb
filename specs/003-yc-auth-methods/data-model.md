# Phase 1 Data Model: Yandex Cloud Production Authentication Methods

This feature changes *connection configuration*, not persisted data. There is no schema change, no new
table, and no change to the stored row shape. The "data" here is the in-memory configuration parsed from
the component manifest and the mapping from that configuration to YDB driver options.

## Configuration entity: `storeMetadata` (existing — no field changes)

Defined in [internal/ydbstate/metadata.go](../../internal/ydbstate/metadata.go). All relevant fields
already exist and are already parsed/validated. This feature adds **behavior** (wiring), not fields.

| Field | Manifest key | Type | Relevance to this feature |
|-------|--------------|------|---------------------------|
| `ConnectionString` | `connectionString` | string (required) | Unchanged. `grpcs://…` for managed endpoints. |
| `AuthMethod` | `authMethod` | enum | Selects the credential option. Two values become live. |
| `Username` / `Password` | `username` / `password` | string | Used by `static` only. Unchanged. |
| `AccessToken` | `accessToken` | string | Used by `token` only. Unchanged. |
| `ServiceAccountKeyPath` | `serviceAccountKeyPath` | string | **Now consumed** for `serviceAccountKey`. Pre-flight read. |
| `UseInternalCA` | `useInternalCA` | bool | **Now consumed** — appends `WithInternalCA()` for any method. |
| `TableName` | `tableName` | string | Unrelated to auth. Unchanged. |

## Auth-method enum (existing — no changes)

```text
anonymous | static | token | serviceAccountKey | metadata
```

All five are already accepted by `parseAndValidateMetadata`. Validation rules per method are unchanged:

- `static` → requires `username` + `password`
- `token` → requires `accessToken`
- `serviceAccountKey` → requires `serviceAccountKeyPath` (presence; readability checked at wiring time)
- `metadata` → requires nothing (secret-less)
- unknown → rejected with the full accepted-values list

## State / behavior transition (the actual change)

The transition is in `credentialOptions()` — from "two arms error out" to "all five arms produce driver
options, with internal-CA composed on top":

```text
            authMethod                         base credential option (ydb.Option)
            ----------                         -----------------------------------
            anonymous          ──────────────▶ ydb.WithAnonymousCredentials()
            static             ──────────────▶ ydb.WithStaticCredentials(user, pass)
            token              ──────────────▶ ydb.WithAccessTokenCredentials(tok)
            serviceAccountKey  ──pre-flight──▶ yc.WithServiceAccountKeyFileCredentials(path)
                               (os.ReadFile)        └─ read fails ⇒ field-named Init error
            metadata           ──────────────▶ yc.WithMetadataCredentials()

            then, for ALL methods:
            if UseInternalCA ⇒ append yc.WithInternalCA()
```

Lifecycle (unchanged surrounding code): `Init` → `parseAndValidateMetadata` → `credentialOptions` →
`ydb.Open(ctx, connStr, opts...)` → `ensureTable`. Connect-time credential failures (bad key JSON,
unreachable metadata service, rejected SA) surface from `ydb.Open` and are wrapped as
`failed to open YDB connection: %w`.

## Validation rules (consolidated)

| Rule | Source | Outcome on violation |
|------|--------|----------------------|
| `serviceAccountKeyPath` present when `authMethod=serviceAccountKey` | existing `metadata.go` | field-named Init error |
| `serviceAccountKeyPath` readable | **new** pre-flight in `credentialOptions` | field-named Init error naming the key file |
| metadata service reachable/authorized | connect time (`ydb.Open`) | wrapped connection error |
| SA key valid + accepted by server | connect time (`ydb.Open`) | wrapped connection error |
| unknown `authMethod` | existing `metadata.go` | error listing accepted values |

## Non-goals (explicitly unchanged)

- Stored row schema, ETag generation, TTL/expiry filtering, Get/Set/Delete logic.
- `Features()` output — no new `state.Feature`.
- The set of accepted `authMethod` values (still exactly five).
