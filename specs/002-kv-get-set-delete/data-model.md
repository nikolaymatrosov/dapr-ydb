# Phase 1 Data Model: KV Get/Set/Delete with conformance-gated ETag

This feature **implements** the persisted KV record that `001-project-scaffold/data-model.md` documented
as a design contract. The schema is unchanged; this file pins the runtime behavior of each field and the
request/response mapping. The canonical schema and DDL live in the scaffold data-model — this is the
feature-scoped operational view.

## Entity: Key/Value Record (now persisted)

| Column | YDB type | Required | Written by this feature | Read semantics |
|--------|----------|----------|-------------------------|----------------|
| `key` | `Utf8` (PRIMARY KEY) | yes | Set | exact-match lookup; the runtime composes `appID||key` upstream |
| `value` | `String` | yes | Set | returned as raw `[]byte`; never parsed (Principle III) |
| `etag` | `Utf8` | yes | Set (fresh UUIDv4 per write) | returned with the value; opaque to callers |
| `expires_at` | `Timestamp` (nullable) | no | **not written** by this feature (always `NULL`) | read query filters past-expiry rows out (D5) |

**DDL** (created idempotently in `Init` via `ensureTable`; see scaffold data-model for the canonical form):

```sql
CREATE TABLE IF NOT EXISTS dapr_state (
    key        Utf8,
    value      String,
    etag       Utf8,
    expires_at Timestamp,
    PRIMARY KEY (key)
) WITH (
    TTL = Interval("PT0S") ON expires_at
);
```

## Operation: Get

**Input** (`state.GetRequest`): `Key`. (Consistency/options ignored — no feature advertised that changes them.)

**Behavior**:
1. Run `SELECT value, etag FROM <table> WHERE key = $key AND (expires_at IS NULL OR expires_at > CurrentUtcTimestamp())`.
2. No row → return `&state.GetResponse{}` (empty data, nil ETag) — "not found", **no error** (FR-003, FR-009).
3. Row found → `&state.GetResponse{Data: value, ETag: &etag}` (FR-006).

**Errors**: backend/query failure → wrapped error (never panic).

## Operation: Set

**Input** (`state.SetRequest`): `Key`, `Value` (`[]byte` or other Go value), optional `ETag`, `Options`.

**Behavior**:
- **No ETag** (or empty): one-shot `UPSERT INTO <table> (key, value, etag) VALUES ($key, $value, $newEtag)`
  → unconditional create-or-overwrite (FR-002, assumption "empty token = unconditional").
- **With ETag**: inside a serializable `DoTx`, read current `etag`; if no row **or** stored etag ≠
  provided (including a malformed token) ⇒ `ETagMismatch` (FR-007); else `UPSERT` with `$newEtag` and
  commit (FR-006). See research D4 — a malformed token yields `ETagMismatch`, not `ETagInvalid`, to
  match the conformance suite and contrib Postgres v2.
- `$newEtag = newETag()` (fresh UUIDv4) on every successful write; never reused (Principle III, D7).

**Value encoding**: `[]byte` is stored verbatim into the `String` column; any other type is JSON-encoded
via `stateutils.Marshal(value, json.Marshal)`. Returned identically by Get (SC-001). A zero-length value
is a valid stored value, distinct from absent (edge case).

**Errors**: `state.NewETagError(state.ETagMismatch, ...)` (incl. malformed token); backend failure wrapped.

## Operation: Delete

**Input** (`state.DeleteRequest`): `Key`, optional `ETag`, `Options`.

**Behavior**:
- **No ETag**: one-shot `DELETE FROM <table> WHERE key = $key`. Absent key ⇒ success (idempotent, FR-004).
- **With ETag**: inside a serializable `DoTx`, read current `etag`; no row **or** mismatch (incl. malformed
  token) ⇒ `ETagMismatch`; else `DELETE` and commit (FR-007).

**Errors**: same ETag error class as Set (`ETagMismatch`); backend failure wrapped.

## ETag lifecycle (per key)

```
absent --Set(no etag)--> present(etag₁)
present(etagₙ) --Set(etag=etagₙ)--> present(etagₙ₊₁)      (advance)
present(etagₙ) --Set(etag=stale)--> present(etagₙ)        (rejected: ETagMismatch)
present(etagₙ) --Set(etag=garbage)--> present(etagₙ)      (rejected: ETagMismatch — see D4)
present(etagₙ) --Delete(etag=etagₙ)--> absent
present(etagₙ) --Delete(etag=stale)--> present(etagₙ)     (rejected: ETagMismatch)
```

Each successful Set advances to a new opaque UUID etag. A logically-expired row behaves as `absent` on
read regardless of native-TTL purge timing (D5).

## Advertised Features (this feature)

`Features()` evolution, gated by conformance (constitution Principles I & II, FR-010/FR-011):

| Stage | `Features()` returns | Precondition |
|-------|----------------------|--------------|
| P1 (CRUD landed) | `[]state.Feature{}` | basic CRUD conformance scenarios pass |
| P2 (ETag landed) | `[]state.Feature{state.FeatureETag}` | **eTag conformance scenarios pass** |

No `FeatureTTL`, `FeatureTransactional`, or `FeatureQueryAPI` is advertised by this feature.
