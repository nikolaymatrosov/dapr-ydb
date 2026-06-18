# Phase 0 Research: KV Get/Set/Delete with conformance-gated ETag

All unknowns below are resolved; none remain marked NEEDS CLARIFICATION. Decisions build on the
scaffold (`001-project-scaffold`) and constitution v1.0.0.

## D1 — YDB access path: query service vs legacy table API

**Decision**: Use the **query service** — `driver.Query()` with `Query().Do`/`Query().DoTx` and
parameterized YQL via `ydb.ParamsBuilder()`.

**Rationale**: `ydb-go-sdk/v3` v3.140.2 ships a stable query service that is the SDK's recommended path
forward; it provides serializable interactive transactions (needed for compare-and-set) with a single,
consistent API for both one-shot statements and multi-statement txns. The legacy `table.Session` API
still works but is being de-emphasized and forces a second mental model.

**Alternatives considered**: Legacy `table` service (`driver.Table().Do`) — rejected to avoid mixing two
query APIs and to keep the transactional path (D3) uniform with the single-statement path.

## D2 — Idempotent schema creation

**Decision**: `ensureTable(ctx)` issues `CREATE TABLE IF NOT EXISTS <tableName> (...)` matching the
documented DDL, called once from `Init` after the driver opens. Native row TTL is declared on
`expires_at` so the engine eventually purges expired rows.

**Rationale**: Constitution requires explicit, idempotent schema management. `IF NOT EXISTS` makes
concurrent initializers and restarts safe (FR-001 edge case). Declaring native TTL now means the column
is reclaimed by the engine; reads still filter defensively (D5).

**Alternatives considered**: (a) Migration tool — overkill for one table. (b) Assume table exists —
violates the schema-management constraint. (c) `CREATE TABLE` without `IF NOT EXISTS` and swallow the
"already exists" error — brittle string matching, rejected.

**Note**: YQL cannot bind a table name as a parameter. `tableName` is already validated as a YDB
identifier in `metadata.go`, so it is safely interpolated into DDL/DML strings (see D6).

## D3 — Optimistic concurrency (ETag) atomicity

**Decision**: ETag-bearing `Set`/`Delete` run inside one **serializable** interactive transaction via
`driver.Query().DoTx`: read the current `etag` for the key `FOR UPDATE`-style within the tx, compare to
the caller's ETag, then `UPSERT`/`DELETE` and commit. Non-ETag writes use a single one-shot
`UPSERT`/`DELETE` (no tx needed). The SDK retries the tx on transient/serialization errors.

**Rationale**: A read-compare-write must be atomic to guarantee "at most one of N racing writers with the
same prior token succeeds" (SC-004, Principle III). YDB serializable isolation provides this without any
application-level locking. `DoTx`'s built-in retry handles serialization conflicts idiomatically.

**Alternatives considered**: (a) Single conditional `UPDATE ... WHERE key=$k AND etag=$e` and inspect
rows-affected — works for `Set` on an existing row but cannot distinguish "absent key" from "etag
mismatch" cleanly, and complicates the upsert-or-insert case; the tx form is clearer and uniform. (b)
App-level mutex — does not scale across processes and is rejected by Principle IV.

## D4 — ETag error class: always ETagMismatch (REVISED during implementation)

**Decision**: Any ETag-bearing write/delete whose token does not equal the stored token — whether the
token is well-formed-but-stale, the key is absent/expired, **or the token is malformed** — returns
`state.NewETagError(state.ETagMismatch, nil)`. We do **not** emit `ETagInvalid`.

**Rationale (verified against the suite)**: The Dapr state conformance suite uses
`config.BadEtag = "bad-etag"` (not a UUID) and asserts `etagErr.Kind() == state.ETagMismatch`
(`tests/conformance/state/state.go:992,1026,1110`). The closest analog — contrib **Postgres v2**, which
also uses UUID etags — returns `ETagMismatch` even when `uuid.Parse` fails
(`state/postgresql/v2/postgresql.go:395-396,600-601`). Since the user's directive and constitution
Principle II make conformance the authoritative gate, the malformed→`ETagInvalid` split originally
planned here (and in FR-008 / data-model / Principle III) is **unsatisfiable while passing conformance**.
Resolved in favor of conformance: malformed → `ETagMismatch`.

**Consequence**: `parseETag()` is not needed; comparison is a plain opaque string equality. `ETagInvalid`
is never produced. The simplest correct implementation compares the caller's token to the stored token
inside the serializable CAS transaction.

**Constitution note (resolved)**: Principle III originally required malformed ETag → `ETagInvalid`,
which conflicted with Principle II (conformance is authoritative). The constitution was amended to
**v1.1.0**: Principle III now requires any non-matching ETag (including malformed) → `ETagMismatch`,
with `ETagInvalid` optional only where it does not conflict with a conformance assertion. This research
decision and the constitution are now consistent.

**Alternatives considered**: Keep malformed→`ETagInvalid` — **fails** conformance (`bad-etag` expects
mismatch). Rejected.

## D5 — Logical-expiry read filtering

**Decision**: The `Get` query filters `WHERE key = $key AND (expires_at IS NULL OR expires_at >
CurrentUtcTimestamp())`. A row past expiry returns "not found" even if native TTL has not purged it.

**Rationale**: Native TTL purge is eventual; correctness (FR-009, SC-005, Principle III) requires reads
never to surface logically-expired data. This is a read-side filter only — this feature does **not**
write `expires_at` (TTL-on-Set is out of scope) nor advertise `FeatureTTL`. In practice `expires_at` is
always `NULL` for rows this feature writes, but the filter is correct and future-proof.

**Alternatives considered**: Rely solely on native TTL — leaves a window where expired rows are
returned. Rejected.

## D6 — Table-name interpolation safety

**Decision**: Interpolate the validated `tableName` into query strings; bind only `key`, `value`, `etag`
as YQL parameters.

**Rationale**: YQL/YDB does not support binding identifiers (table/column names) as parameters. The
`tableName` is validated as a YDB identifier at `Init` (existing `metadata.go` rule), so interpolation
is not an injection vector. Values and etags are always bound parameters, never interpolated.

**Alternatives considered**: None viable — identifier binding is unsupported by the engine.

## D7 — ETag generation

**Decision**: `newETag()` returns `uuid.NewString()` (random UUIDv4) on every successful write; stored in
the `etag` `Utf8` column; never reused.

**Rationale**: Principle III mandates opaque, never-reused ETags; random UUIDv4 satisfies both and matches
the documented schema ("random UUID per write"). `github.com/google/uuid` is already a transitive
dependency of the YDB SDK, so no meaningful dependency cost.

**Alternatives considered**: Monotonic counter / row version column — couples etag to storage internals
and risks reuse across delete+recreate; rejected for the opaque-UUID approach the schema already specifies.

## D8 — Conformance gating sequence (the core requirement)

**Decision**: Two-step landing.
1. **P1**: implement CRUD; flip the harness `t.Skip` off; set `operations` to the basic CRUD scenarios;
   `Features()` still returns `[]state.Feature{}`. Basic CRUD conformance must be green.
2. **P2**: implement ETag CAS; add the eTag scenario to `operations`; run conformance; **only when the
   eTag scenarios pass**, change `Features()` to return `[]state.Feature{state.FeatureETag}`. The code
   flip and the green suite ship in the same commit (Principle II: advertised ⇒ verified).

**Rationale**: Directly encodes FR-010/FR-011 and SC-002/SC-003/SC-007 and constitution Principles I & II.
Advertising before the suite is green is a contract violation; this sequence makes the gate mechanical.

**Alternatives considered**: Advertise `FeatureETag` alongside P1 implementation and "fix later" —
explicitly forbidden by the constitution. Rejected.

## Open risks

- **YDB local container TTL semantics**: native `Interval("PT0S")` TTL behavior in the local single-node
  container may differ from cloud; mitigated because reads filter expiry independently (D5), so
  correctness does not depend on purge timing during tests.
- **uuid dependency surfacing**: if `github.com/google/uuid` is not promoted to a direct dependency,
  `go get github.com/google/uuid` during P1 makes it explicit; verified via `go mod tidy`.
