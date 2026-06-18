# Contract: Get / Set / Delete operations

The component satisfies `github.com/dapr/components-contrib/state.Store`. This contract pins the observable
behavior of the three single-key operations plus the conformance-gated capability advertisement. It is the
acceptance reference for the conformance suite and the unit tests.

## Capability advertisement

```
Features() ⟶ []state.Feature{state.FeatureETag}     // implemented + conformance-verified
```

Invariant (constitution I/II): the slice contains `FeatureETag` **iff** the conformance eTag scenarios
pass. Note: the suite itself asserts `FeatureETag.IsPresent(Features())` as part of its eTag block
(`tests/conformance/state/state.go:942`), so advertisement and verified behavior are coupled — the eTag
scenarios cannot pass unless the capability is advertised **and** the behavior is correct. Never
`FeatureTTL`/`FeatureTransactional`/`FeatureQueryAPI`.

## Get(ctx, *state.GetRequest) → (*state.GetResponse, error)

| Condition | Result |
|-----------|--------|
| key present, not expired | `GetResponse{Data: <bytes>, ETag: &<etag>}`, nil error |
| key absent | `GetResponse{}` (empty), nil error — **not** an error |
| key present but `expires_at` in the past | treated as absent → `GetResponse{}`, nil error |
| backend failure | nil response, wrapped non-panic error |

Guarantees: returned `Data` is byte-identical to the bytes last Set (SC-001); value is never parsed.

## Set(ctx, *state.SetRequest) → error

| Condition | Result |
|-----------|--------|
| no/empty ETag | unconditional upsert; key now has a fresh UUID etag; nil error |
| ETag present, malformed (not a UUID) | `state.NewETagError(state.ETagMismatch, …)` — see D4 |
| ETag present, key absent | `state.NewETagError(state.ETagMismatch, …)` |
| ETag present ≠ stored etag | `state.NewETagError(state.ETagMismatch, …)`; stored data unchanged |
| ETag present = stored etag | upsert; key advances to a new UUID etag; nil error |
| backend failure | wrapped non-panic error |

Guarantees: every successful Set assigns a new opaque etag never previously used for that key. Under N
concurrent Sets carrying the same prior etag, at most one succeeds; the rest get `ETagMismatch` (SC-004).

## Delete(ctx, *state.DeleteRequest) → error

| Condition | Result |
|-----------|--------|
| no/empty ETag, key present | row removed; nil error |
| no/empty ETag, key absent | nil error (idempotent) |
| ETag present, key absent or ≠ stored (incl. malformed) | `state.NewETagError(state.ETagMismatch, …)` |
| ETag present = stored etag | row removed; nil error |
| backend failure | wrapped non-panic error |

## Bulk operations

`BulkGet`/`BulkSet`/`BulkDelete` continue to delegate to the single-key methods above via
`state.NewDefaultBulkStore` (unchanged from scaffold). No bulk-specific capability is advertised.

## Reference YQL (informative — see research D1/D3/D6)

```sql
-- Get
SELECT value, etag FROM <table>
WHERE key = $key AND (expires_at IS NULL OR expires_at > CurrentUtcTimestamp());

-- Set, no etag (one-shot)
UPSERT INTO <table> (key, value, etag) VALUES ($key, $value, $newEtag);

-- Set/Delete with etag: inside one serializable DoTx —
SELECT etag FROM <table> WHERE key = $key;          -- compare to caller etag in Go
UPSERT INTO <table> (key, value, etag) VALUES ($key, $value, $newEtag);   -- or:
DELETE FROM <table> WHERE key = $key;

-- Delete, no etag (one-shot)
DELETE FROM <table> WHERE key = $key;
```

`<table>` is the validated `tableName` identifier (interpolated, not bound). `$key`/`$value`/`$etag`/
`$newEtag` are always bound parameters.

## Conformance mapping

| Contract row | Conformance scenario / unit test |
|--------------|----------------------------------|
| Get present/absent, Set unconditional, Delete idempotent | basic CRUD conformance scenarios (P1) |
| ETag mismatch (incl. bad etag) on Set & Delete, etag advances | eTag conformance scenarios (P2, gate for `FeatureETag`) |
| concurrent CAS at-most-one winner | unit test (`operations_test.go`, `TestIntegration_ConcurrentCASAtMostOneWinner`) |
| value-as-bytes round-trip | conformance + unit test with arbitrary binary payload |
