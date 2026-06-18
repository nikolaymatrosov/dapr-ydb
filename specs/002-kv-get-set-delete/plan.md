# Implementation Plan: KV Get/Set/Delete with conformance-gated ETag

**Branch**: `002-kv-get-set-delete` | **Date**: 2026-06-18 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/002-kv-get-set-delete/spec.md`

## Summary

Turn the scaffold's stubbed `Get`/`Set`/`Delete` into real persistence against the documented KV
schema (`key`, `value`, `etag`, `expires_at`) in `001-project-scaffold/data-model.md`. The component
creates the table idempotently, stores values as opaque bytes, generates a fresh opaque ETag (UUID) on
every write, filters logically-expired rows on read, and implements optimistic concurrency for
ETag-bearing `Set`/`Delete`. Per constitution Principles I & II, `Features()` keeps returning an empty
slice until the ETag conformance scenarios pass; only then does it return `[]state.Feature{state.FeatureETag}`.
This is delivered in two slices: **P1** unconditional CRUD (no advertised features), **P2** ETag
optimistic concurrency (advertised only after conformance is green).

## Technical Context

**Language/Version**: Go **1.26.4+** (inherited from scaffold; floor set by `dapr/dapr` v1.18 / `dapr/kit`).

**Primary Dependencies** (all already in `go.mod` from the scaffold unless noted):
- `github.com/dapr/components-contrib/state` **v1.18.0** — the `state.Store` contract plus
  `state.GetRequest`/`SetRequest`/`DeleteRequest`/`GetResponse`, `state.ETagMismatch`/`ETagInvalid`,
  and `state.NewETagError(...)`.
- `github.com/dapr-sandbox/components-go-sdk` **v0.3.0** — pluggable host (already wired in `cmd/daprd-ydb`).
- `github.com/ydb-platform/ydb-go-sdk/v3` **v3.140.2** — uses the **query service** (`driver.Query()`)
  for parameterized YQL and `DoTx` serializable transactions for compare-and-set.
- `github.com/google/uuid` (**new, transitive-or-add**) — opaque ETag generation (random UUIDv4).
  If not already pulled in transitively, add via `go get`; YDB SDK already depends on it, so this is
  expected to be a no-cost addition.

**Storage**: YDB. Local dev/test via the `deploy/docker-compose.yml` YDB container
(`grpc://localhost:2136/local`); single configured table per store (default `dapr_state`).

**Testing**: Dapr state conformance suite (`tests/conformance/state`) run against a real containerized
YDB under the `conformance` build tag (existing `tests/conformance` harness — flip the skip and grow the
`operations` list), plus table-driven unit tests in `internal/ydbstate` for ETag parsing/branching and
expiry filtering. Conformance against a real YDB is the merge gate (constitution Principle II).

**Target Platform**: Linux/macOS server process beside `daprd` over a Unix Domain Socket.

**Project Type**: Single Go project — extends the existing `internal/ydbstate` package; no new module.

**Performance Goals**: Single round-trip Get/Set/Delete are single-statement (or single short
serializable tx for the CAS path); no N+1. Targets are correctness-first (SC-001..SC-007); the request
path adds no in-process caching.

**Constraints**:
- `Features()` MUST stay empty until ETag conformance passes, then return exactly `FeatureETag`
  (no TTL/transactional/query) — constitution Principle I.
- Values persisted/returned as raw `[]byte` via the `String` column; never parsed (Principle III).
- Reads MUST filter `expires_at` regardless of native-TTL purge timing (Principle III), even though
  `FeatureTTL` is **not** advertised by this feature.
- Schema creation MUST be explicit + idempotent (Constraint: Schema management).

### Resolved unknowns (see research.md)

- **YQL access path**: query service (`driver.Query()`), not the legacy table/`table.Session` API.
- **CAS atomicity**: ETag `Set`/`Delete` run inside one `driver.Query().DoTx` serializable transaction
  (read current etag → compare → write), satisfying SC-004 / Principle III without app-level locks.
- **ETag error class** (revised during implementation): any non-matching ETag — stale, absent-row, **or
  malformed** — returns `ETagMismatch`. The conformance suite asserts mismatch for a bad token and contrib
  Postgres v2 returns mismatch on UUID-parse failure, so no separate `ETagInvalid` class is produced. See
  research D4; comparison is plain opaque string equality (no `parseETag`).
- **TableName injection**: table name is a validated identifier, not a bind parameter (YQL cannot bind
  table names); it is interpolated after identifier validation (already enforced in `metadata.go`).

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Constitution v1.1.0 — evaluation:

| Principle | Gate | Status |
|-----------|------|--------|
| I. State Store Contract Fidelity | Real `Get`/`Set`/`Delete`; `Bulk*` still via `DefaultBulkStore`; `Features()` advertises only what is verified | ✅ PASS — `Features()` stays empty through P1; returns exactly `FeatureETag` only after P2 conformance is green. No TTL/transactional/query advertised. |
| II. Conformance-Verified (NON-NEGOTIABLE) | Advertised features pass conformance before merge | ✅ PASS — sequencing is explicit: CRUD conformance scenarios pass in P1; ETag scenarios must pass before the `FeatureETag` flip in P2. The flip and the green suite land in the same change. |
| III. Concurrency/Consistency/TTL/bytes | Any non-matching ETag (incl. malformed)→`ETagMismatch`, opaque never-reused UUIDs; values as `[]byte`; expired rows never returned | ✅ PASS — CAS in a serializable tx; new UUID per write; `String` column round-trips bytes; read query filters `expires_at`. Aligns with constitution v1.1.0 Principle III (malformed→mismatch). |
| IV. Idiomatic, Pluggable YDB | UDS, no `daprd` rebuild, YDB Go SDK, idempotent schema, clean lifecycle | ✅ PASS — query service + `CREATE TABLE IF NOT EXISTS` in/after `Init`; driver still closed in `Close`. |
| V. Observability & Operability | Structured logs on error paths, no panic, manifest-only config | ✅ PASS — operations log on failure via existing `slog` logger; typed errors, never panic; no new config/env coupling. |

**Result**: PASS, no violations. Complexity Tracking not required.

## Project Structure

### Documentation (this feature)

```text
specs/002-kv-get-set-delete/
├── plan.md              # This file (/speckit-plan output)
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output — feature-scoped view of the KV schema
├── quickstart.md        # Phase 1 output — run CRUD + conformance locally
├── contracts/
│   └── state-operations.md   # Get/Set/Delete behavioral contract + reference YQL
└── checklists/
    └── requirements.md  # Spec quality checklist (/speckit-specify output)
```

### Source Code (repository root)

```text
internal/ydbstate/
├── store.go             # Get/Set/Delete bodies replace the errNotImplemented stubs;
│                        #   Features() flips to {FeatureETag} once P2 conformance passes
├── store_test.go        # extend: Features() expectation, interface compliance
├── schema.go            # NEW: ensureTable() — idempotent CREATE TABLE IF NOT EXISTS (called from Init)
├── queries.go           # NEW: YQL builders (get/upsert/delete + CAS variants), table-name interpolation
├── etag.go              # NEW: newETag() (UUIDv4). No parseETag — malformed token → ETagMismatch (D4)
├── operations.go        # NEW: Get/Set/Delete implementations + CAS tx helpers (keeps store.go thin)
├── operations_test.go   # NEW: unit tests for etag branching, expiry filter, request mapping
├── metadata.go          # unchanged (TableName already validated as an identifier)
└── errors.go            # errNotImplemented removed once all three ops are implemented

tests/conformance/
├── conformance_test.go  # remove t.Skip; P1 sets operations for CRUD; P2 adds the eTag scenario
└── ydb.yaml             # unchanged

metadata.yaml            # unchanged (no new fields; ETag/TTL are not configurable)
README.md                # update: advertised features now include ETag (after P2), CRUD usage
```

**Structure Decision**: Continue the single-package design from the scaffold. Operation bodies and YQL
live in new files within `internal/ydbstate` so `store.go` stays a thin interface surface and later
features (TTL, transactions, query) extend the same package without restructuring. The conformance
harness is the existing one — this feature flips its skip and grows the `operations` slice.

## Complexity Tracking

> No constitution violations — section intentionally empty.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| — | — | — |
