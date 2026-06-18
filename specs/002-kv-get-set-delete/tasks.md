---
description: "Task list for KV Get/Set/Delete with conformance-gated ETag"
---

# Tasks: KV Get/Set/Delete with conformance-gated ETag

**Input**: Design documents from `/specs/002-kv-get-set-delete/`

**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), [research.md](research.md), [data-model.md](data-model.md), [contracts/state-operations.md](contracts/state-operations.md)

**Tests**: Test tasks ARE included — the spec's Success Criteria (SC-002/SC-003/SC-007) and constitution
Principle II make the Dapr conformance suite the merge gate, so conformance + targeted unit tests are required.

**Organization**: Tasks are grouped by user story. US1 (basic CRUD) is the MVP; US2 (ETag) is gated on
conformance per FR-010/FR-011.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: US1 = basic CRUD, US2 = conformance-gated ETag
- All paths are repo-relative from the project root.

## Path Conventions

Single Go project. Store logic extends `internal/ydbstate/`; conformance harness in `tests/conformance/`.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Dependency wiring not already present from the scaffold.

- [X] T001 Ensure `github.com/google/uuid` is a direct dependency for opaque ETag generation: run `go get github.com/google/uuid` then `go mod tidy`; confirm it appears in `go.mod` require block.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Schema creation, YQL builders, ETag generation, and the query-service plumbing that BOTH user stories need.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [X] T002 [P] Create `internal/ydbstate/queries.go` with YQL builders for get/upsert/delete (and the CAS read used later), interpolating the validated `tableName` identifier and binding `$key`/`$value`/`$newEtag` as parameters per [contracts/state-operations.md](contracts/state-operations.md) (research D6); the Get query MUST include the `expires_at IS NULL OR expires_at > CurrentUtcTimestamp()` filter (research D5).
- [X] T003 [P] Create `internal/ydbstate/etag.go` with `newETag() string` returning a random UUIDv4 (`uuid.NewString()`) per research D7.
- [X] T004 Create `internal/ydbstate/schema.go` with `ensureTable(ctx context.Context) error` issuing `CREATE TABLE IF NOT EXISTS <tableName> (...)` matching the DDL in [data-model.md](data-model.md) via the query service (research D2); uses the table-name interpolation from T002.
- [X] T005 Wire `ensureTable(ctx)` into `YDBStore.Init` in `internal/ydbstate/store.go` after `ydb.Open` succeeds; on failure return a wrapped error (never panic) and log via the existing `slog` logger. (depends on T004)
- [X] T006 Create `internal/ydbstate/operations.go` with the shared query-service read helper (run a parameterized `SELECT value, etag` and scan one row → `(value []byte, etag string, found bool, err error)`) used by Get and the CAS path. (depends on T002)

**Checkpoint**: Table is created idempotently on Init; query/etag plumbing ready. CRUD work can begin.

---

## Phase 3: User Story 1 - Basic key/value round-trip (Priority: P1) 🎯 MVP

**Goal**: Real Get/Set/Delete against the KV schema — values as opaque bytes, unconditional upsert,
idempotent delete, not-found on absent/expired keys. `Features()` stays `[]`.

**Independent Test**: Save a value under a fresh key, Get it back byte-identical, Delete it, Get again →
not-found. Verified by the basic CRUD conformance scenarios + unit tests, with no ETag behavior involved.

### Tests for User Story 1

- [X] T007 [P] [US1] Unit tests in `internal/ydbstate/operations_test.go` (against the local YDB container): byte-identical round-trip incl. arbitrary binary (SC-001), Get on absent key → empty `GetResponse{}` no error (FR-003), unconditional Set overwrites existing value (FR-002), Delete absent key → success (FR-004), and a row with past `expires_at` → not-found (FR-009/SC-005). Write FIRST; expect failure until T008–T011.

### Implementation for User Story 1

- [X] T008 [US1] Implement `Get` in `internal/ydbstate/operations.go`: run the expiry-filtered select via T006 helper; map found → `&state.GetResponse{Data: value, ETag: &etag}`, absent → `&state.GetResponse{}` (FR-003, FR-006, FR-009). (depends on T006)
- [X] T009 [US1] Implement the no-ETag `Set` path in `internal/ydbstate/operations.go`: one-shot `UPSERT` with `newETag()` (FR-002, FR-005, FR-006). (depends on T006, T003)
- [X] T010 [US1] Implement the no-ETag `Delete` path in `internal/ydbstate/operations.go`: one-shot `DELETE FROM <table> WHERE key=$key`, absent key → nil error (FR-004). (depends on T006)
- [X] T011 [US1] Replace the `errNotImplemented` stubs for `Get`/`Set`/`Delete` in `internal/ydbstate/store.go` so they delegate to the `operations.go` implementations; keep `Features()` returning `[]state.Feature{}` for now (FR-010). (depends on T008, T009, T010)
- [X] T012 [US1] Update `tests/conformance/conformance_test.go`: remove the `t.Skip`, set `operations` to the basic CRUD scenario keys (no eTag), and keep `props` as configured. (depends on T011)
- [X] T013 [US1] Run `make conformance` and confirm the basic CRUD scenarios pass against the containerized YDB (SC-002); fix until green.

**Checkpoint**: US1 fully functional — the store persists and serves CRUD with no features advertised. MVP done.

---

## Phase 4: User Story 2 - Optimistic concurrency via ETag, advertised only after conformance (Priority: P2)

**Goal**: ETag CAS on Set/Delete (mismatch on any non-matching token incl. malformed, etag advances per
write), then advertise `FeatureETag` (FR-007, FR-008, FR-010, FR-011). **Implementation note**: the
conformance suite's eTag block itself asserts `FeatureETag.IsPresent(Features())` (state.go:942), so the
capability must be advertised for the eTag scenarios to even run — advertisement and verified behavior are
coupled, and the suite is green only when both hold. The "advertise only after passing" intent is honored:
the flip and the green suite land together.

**Independent Test**: Run the conformance eTag scenarios → pass; confirm `Features()` lists exactly
`FeatureETag`; a stale-token AND a malformed-token write are both rejected with `ETagMismatch`.

### Tests for User Story 2

- [X] T014 [P] [US2] Unit tests in `internal/ydbstate/operations_test.go`: malformed/stale/absent ETag → `ETagMismatch` (FR-007/FR-008, see T015 note), successful Set advances to a new distinct etag (FR-006), and N concurrent Sets carrying the same prior etag → exactly one succeeds, rest `ETagMismatch` (SC-004). Implemented as `TestIntegration_*` that skip gracefully when no YDB is reachable.

### Implementation for User Story 2

- [X] T015 [US2] **DROPPED during implementation** — `parseETag`/`ETagInvalid` is not needed. The conformance suite asserts `ETagMismatch` for a bad token (`"bad-etag"`), and contrib Postgres v2 returns mismatch on UUID-parse failure. ETag handling is a plain opaque string comparison inside the CAS transaction; a malformed token simply fails to match → `ETagMismatch`. See revised [research.md](research.md) D4 and FR-008.
- [X] T016 [US2] Add a serializable CAS helper to `internal/ydbstate/operations.go` using `driver.Query().DoTx`: read current etag via T006 helper, compare to caller's; absent/mismatch → `state.NewETagError(state.ETagMismatch, nil)`, else perform the write/delete and commit (research D3). (depends on T006, T015)
- [X] T017 [US2] Wire the ETag branch into `Set` (in `operations.go`): empty ETag → existing T009 path; non-empty → `parseETag` then CAS upsert with a fresh `newETag()` (FR-007, FR-008). (depends on T016, T009)
- [X] T018 [US2] Wire the ETag branch into `Delete` (in `operations.go`): empty ETag → existing T010 path; non-empty → `parseETag` then CAS delete (FR-007, FR-008). (depends on T016, T010)
- [X] T019 [US2] Update `tests/conformance/conformance_test.go`: add the eTag scenario key(s) to the `operations` slice. (depends on T017, T018)
- [X] T020 [US2] Run `make conformance` and confirm the eTag scenarios pass (SC-003); fix until green. **This is the gate** — do not proceed to T021 until green.
- [X] T021 [US2] ONLY after T020 is green: change `Features()` in `internal/ydbstate/store.go` to return `[]state.Feature{state.FeatureETag}` (FR-011). Commit the flip and the green suite together (constitution Principle II).
- [X] T022 [US2] Re-run `make conformance` with CRUD + eTag scenarios; confirm the full suite is green and `Features()` advertises exactly `FeatureETag` (SC-007).

**Checkpoint**: ETag optimistic concurrency works and is advertised, justified by a green conformance suite.

---

## Phase 5: Polish & Cross-Cutting Concerns

**Purpose**: Align tests, docs, and tooling with the now-implemented feature.

- [X] T023 [P] Update `internal/ydbstate/store_test.go`: assert `Features()` returns `[]state.Feature{state.FeatureETag}` and keep the `var _ state.Store` compile-time assertion.
- [X] T024 [P] Clean up `internal/ydbstate/errors.go`: remove `errNotImplemented` if no longer referenced (all three ops now implemented), or keep only for genuinely-unsupported paths.
- [X] T025 [P] Update `README.md`: advertised features now include ETag, document Get/Set/Delete usage, the idempotent table creation, and that TTL/transactions/query remain unimplemented.
- [X] T026 Run `go mod tidy` and `make lint` (golangci-lint); resolve findings.
- [X] T027 Run the [quickstart.md](quickstart.md) end-to-end (build → unit → conformance → gate verification) and confirm all done-criteria pass.

### Post-analysis gap closure (`/speckit-analyze` follow-up)

- [X] T028 [C1] Update `metadata.yaml` `capabilities` to `["crud", "etag"]` so the manifest matches `Features()` (constitution Dev-Workflow doc gate). Previously `[]` with a stale "scaffold" comment.
- [X] T029 [G1] Add `TestIntegration_ExpiryFilteredOnRead` in `operations_test.go`: seeds rows with past/future `expires_at` directly (via `seedWithExpiry`) and asserts the expired row reads as not-found while the future one is returned (FR-009/SC-005 — previously implemented but untested).
- [X] T030 [G2] Add `TestIntegration_ContextCancellationReturnsError` and `TestIntegration_OperationsAfterCloseReturnError`: pre-cancelled context and post-`Close` operations return errors and never panic (FR-012/SC-006).

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: no dependencies — start immediately.
- **Foundational (Phase 2)**: depends on Setup — BLOCKS both user stories.
- **US1 (Phase 3)**: depends on Foundational. Delivers the MVP.
- **US2 (Phase 4)**: depends on Foundational; its implementation builds on the US1 Set/Delete paths
  (T017↔T009, T018↔T010), so US2 follows US1 rather than running fully parallel.
- **Polish (Phase 5)**: depends on US2 completion (the `Features()` flip lands in T021).

### Critical gating dependency (the core requirement)

- **T021 (advertise `FeatureETag`) MUST NOT start until T020 (eTag conformance) is green.** This encodes
  FR-010/FR-011 and constitution Principle II. Advertising before the suite passes is a contract violation.

### Within Each User Story

- Unit tests (T007, T014) written FIRST and failing before implementation.
- Foundational query/etag/schema plumbing before operation bodies.
- Operation bodies before the `store.go` delegation and conformance wiring.

### Parallel Opportunities

- T002 and T003 (different new files) run in parallel.
- Polish T023/T024/T025 (different files) run in parallel.
- Within Phase 2, T002/T003 are independent; T004→T005 and T006 follow.

---

## Parallel Example: Phase 2 Foundational

```bash
# Independent new files — launch together:
Task: "Create internal/ydbstate/queries.go (YQL builders + table interpolation + expiry filter)"
Task: "Create internal/ydbstate/etag.go (newETag UUIDv4)"
```

---

## Implementation Strategy

### MVP First (User Story 1 only)

1. Phase 1 Setup → Phase 2 Foundational → Phase 3 US1.
2. **STOP and VALIDATE**: `make conformance` basic CRUD green, unit tests green; `Features()` still `[]`.
3. This is a deployable, honest store (CRUD works, advertises nothing it cannot do).

### Incremental Delivery

1. Setup + Foundational → table + plumbing ready.
2. US1 → CRUD works, no features advertised → demo MVP.
3. US2 → ETag CAS, conformance-gated `FeatureETag` flip → demo optimistic concurrency.
4. Polish → docs/tests/lint aligned.

---

## Notes

- [P] = different files, no incomplete-task dependencies.
- The conformance suite runs against a real containerized YDB (constitution: no mocks for persistence).
- T020 → T021 is the single most important ordering constraint: capability advertised ⇔ conformance green.
- Commit after each task or logical group; the `Features()` flip and its green suite go in one commit.
