---
description: "Task list for YDB Query/Exec Output Binding"
---

# Tasks: YDB Query/Exec Output Binding

**Input**: Design documents from `specs/004-query-exec-binding/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/binding-operations.md

**Tests**: INCLUDED — the spec (FR-014, SC-002) and the constitution (Development Workflow:
integration tests against a real YDB instance are the merge gate) require automated verification.

**Organization**: Tasks grouped by user story (US1 query, US2 exec, US3 parameters), each
independently testable, in priority order.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: US1 / US2 / US3 — only on user-story-phase tasks
- File paths are relative to repo root

## Path Conventions

Single Go project. New code: `internal/ydbbinding/`, `internal/ydbconfig/`, `bindings.metadata.yaml`,
`tests/integration/`. Modified: `cmd/daprd-ydb/main.go`, `internal/ydbstate/`.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Scaffold the new component's files and manifest.

- [X] T001 Create package skeletons: `internal/ydbbinding/binding.go`, `internal/ydbbinding/params.go`, `internal/ydbbinding/result.go` (package decl + doc comments) and `internal/ydbconfig/config.go`; create empty `tests/integration/` directory
- [X] T002 [P] Create `bindings.metadata.yaml` manifest stub: `type: bindings.ydb`, `version: v1`, advertised operations `query`/`exec`, and the reused connection/auth metadata fields (mirroring `metadata.yaml`), per contracts/binding-operations.md

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Shared config extraction + binding shell + dispatcher. MUST complete before US1–US3.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

> **Sequencing note**: T003/T004 relocate `credentialOptions()`, which feature 003 is finishing on
> the parent branch. Rebase on 003's final credential logic before/while doing the extraction so the
> moved code is the final version (plan → Dependencies & Sequencing).

- [X] T003 Extract shared config into `internal/ydbconfig/config.go`: exported `Config` struct, `Parse(props map[string]string) (Config, error)` (moved from `internal/ydbstate/metadata.go`), `CredentialOptions(Config) ([]ydb.Option, error)` (moved from `internal/ydbstate/store.go`, including the YC auth methods + `useInternalCA`), and `Open(ctx, Config) (*ydb.Driver, error)`
- [X] T004 Refactor `internal/ydbstate` (store.go, metadata.go) to delegate metadata parsing + credential options + driver open to `internal/ydbconfig` — behavior-preserving; `TableName` parsing stays in `ydbstate`
- [X] T005 [P] Add/move `internal/ydbconfig/config_test.go`: table-driven unit tests for `Parse` validation (required fields, per-auth-method requirements, `useInternalCA` parse error) and `CredentialOptions` mapping
- [X] T006 Regression gate: run the state conformance/unit suite (`go test ./...` + the existing conformance test) and confirm the `ydbconfig` refactor changed no state-store behavior (constitution Principle II)
- [X] T007 Implement `YDBBinding` lifecycle in `internal/ydbbinding/binding.go`: struct (logger, `Config`, `*ydb.Driver`, `*sql.DB`); `Init` parses via `ydbconfig.Parse`, opens driver via `ydbconfig.Open`, derives `db = sql.OpenDB(ydb.MustConnector(driver, ydb.WithAutoDeclare(), ydb.WithPositionalArgs()))` — field-named errors, never panic; `Close` releases db+driver; `Ping`; `GetComponentMetadata`
- [X] T008 Implement `Operations()` in `internal/ydbbinding/binding.go` returning exactly `bindings.OperationKind{"query", "exec"}` (constitution Principle I analogue)
- [X] T009 Implement `Invoke` dispatcher in `internal/ydbbinding/binding.go`: route by `req.Operation`; reject unsupported operation with an error listing `query`/`exec` (FR-011); require non-empty `metadata["sql"]` before any DB call (FR-011); map all DB/input errors to returned errors without panic (FR-008); `query`/`exec` handlers stubbed until their stories
- [X] T010 Register the binding in `cmd/daprd-ydb/main.go`: `dapr.Register("ydb", dapr.WithStateStore(...), dapr.WithOutputBinding(func() bindings.OutputBinding { return ydbbinding.New() }))`

**Checkpoint**: Binding loads on the existing socket, advertises `query`/`exec`, rejects unknown
operations and missing statements; state store unaffected.

---

## Phase 3: User Story 1 - Run a read query and get rows back (Priority: P1) 🎯 MVP

**Goal**: `query` executes a YQL SELECT and returns result rows as a JSON array of column-keyed objects.

**Independent Test**: Invoke `query` with a SELECT over a seeded table → response data is a JSON
array matching the rows; a no-match SELECT → `[]`.

- [X] T011 [P] [US1] Implement result serialization in `internal/ydbbinding/result.go`: `Rows`→`[]map[string]any` keyed by `rows.Columns()`; JSON-marshal; NULL→null, numbers/bool natural, `[]byte`→base64, Timestamp/Date/Datetime→RFC3339; empty result → `[]` (FR-003, FR-010, data-model mapping)
- [X] T012 [US1] Implement the `query` handler in `internal/ydbbinding/binding.go`: run the statement (no-param path), serialize via `result.go`, set `InvokeResponse.Data` + `ContentType="application/json"` + metadata (`operation`,`sql`,`start-time`,`end-time`,`duration`)
- [X] T013 [P] [US1] Unit tests in `internal/ydbbinding/result_test.go`: empty set→`[]`, NULL→null, integer/float/bool/bytes/timestamp serialization, column-name keying
- [X] T014 [US1] Integration test in `tests/integration/binding_test.go`: seeded SELECT returns matching rows as JSON; zero-row SELECT → `[]`+success; invalid YQL → error and component still serves a subsequent request (FR-008, SC-003)

**Checkpoint**: `query` fully functional and independently testable — MVP.

---

## Phase 4: User Story 2 - Execute a write or DDL statement (Priority: P2)

**Goal**: `exec` runs DML and DDL statements and reports success with an execution summary.

**Independent Test**: `exec` INSERT then `query` confirms the row; `exec` CREATE/DROP TABLE succeeds.

- [X] T015 [US2] Implement the `exec` handler in `internal/ydbbinding/binding.go`: run the statement; honor optional `metadata["queryMode"]` (`data` default | `scheme` for DDL via `ydb.WithQueryMode(ctx, ydb.SchemeQueryMode)` | `scripting`); response metadata (`operation`,`sql`, timing, best-effort `rows-affected` or `unknown`) per research D3/D7
- [X] T016 [US2] Integration test in `tests/integration/binding_test.go`: `exec` INSERT then verify via `query`; `exec` CREATE TABLE + DROP TABLE with `queryMode=scheme`; `exec` of a DB-rejected statement → descriptive error (FR-004, FR-008)

**Checkpoint**: `query` and `exec` both work independently.

---

## Phase 5: User Story 3 - Pass parameters safely (Priority: P3)

**Goal**: Both operations accept parameters — positional `params` array (types inferred) and
named+typed `params` object — bound safely so values can never alter statement structure.

**Independent Test**: Parameterized `query`/`exec` produce the same result as the equivalent literal
statement; a value containing YQL syntax is treated as data.

- [X] T017 [P] [US3] Implement parameter construction in `internal/ydbbinding/params.go`: parse `metadata["params"]` JSON; **array** → positional `[]any` (pass-through to `?`/auto-declare); **object** → named+typed → `[]sql.Named` with `types.*Value` per the supported type vocabulary (Bool/Int32/Uint32/Int64/Uint64/Float/Double/Utf8/String/Json/Timestamp/Date/Datetime); errors name the offending parameter (FR-006, FR-006a, FR-007, data-model)
- [X] T018 [US3] Wire `params.go` into the `query` and `exec` handlers in `internal/ydbbinding/binding.go` so both pass built args to the driver
- [X] T019 [P] [US3] Unit tests in `internal/ydbbinding/params_test.go`: positional inference (int→Int64, float→Double, string→Utf8, bool); named+typed for each supported type; unknown type name → error naming param; missing value → error naming param; malformed `params` JSON → error
- [X] T020 [US3] Integration test in `tests/integration/binding_test.go`: positional-param `query` and `exec`; named+typed `query` forcing `Uint64`/`Timestamp`; a param value containing YQL-like text returns it as data, not executed (SC-005)

**Checkpoint**: All three stories independently functional.

---

## Phase 6: Polish & Cross-Cutting Concerns

- [X] T021 [P] Document the binding in `README.md` and finalize `bindings.metadata.yaml`: operations, request fields (`sql`/`params`/`queryMode`), positional vs named+typed params, the type-inference table, and response shape (FR-013, SC-006)
- [X] T022 [P] Update `Makefile` with a binding integration-test target (mirroring the state test target) so `tests/integration` runs against a YDB instance
- [X] T023 Run `quickstart.md` end-to-end against a real/containerized YDB; confirm every Edge Case in spec returns a descriptive error with the state store still serving (SC-004)
- [X] T024 Final review: `Operations()` advertises exactly `query`/`exec`; structured logs on lifecycle/error paths (Principle V); `go vet ./...` and `golangci-lint run` clean

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: no dependencies.
- **Foundational (Phase 2)**: depends on Setup; **blocks all user stories**. Within it: T003 → T004 → T006; T007 → T008/T009 → T010.
- **User Stories (Phase 3–5)**: all depend on Foundational. US1 is the MVP; US2 and US3 build on the shared handlers but are independently testable. US3 adds params to handlers created in US1/US2 (so it integrates with, but does not block, them).
- **Polish (Phase 6)**: depends on all targeted stories.

### User Story Dependencies

- **US1 (P1)**: after Foundational. No dependency on other stories.
- **US2 (P2)**: after Foundational. Independent of US1 (separate handler).
- **US3 (P3)**: after Foundational. Touches the US1/US2 handlers to pass params; if a story is not yet built, US3 wires params into whichever handlers exist. Independently testable via parameterized statements.

### Within Each User Story

- Result/param helper modules ([P]) before the handler that uses them.
- Unit tests ([P]) alongside their module; integration test after the handler is wired.

### Parallel Opportunities

- T002 ‖ T001-followups; T005 runs parallel to T007–T010 logic (different files).
- US1 `result.go` (T011) + its unit test (T013) parallel to early US2 work once Foundational is done.
- Polish T021 ‖ T022.

---

## Parallel Example: User Story 1

```bash
# After Foundational checkpoint:
Task: "T011 Implement result serialization in internal/ydbbinding/result.go"
Task: "T013 Unit tests in internal/ydbbinding/result_test.go"
# T012 (handler) waits on T011; T014 (integration) waits on T012.
```

---

## Implementation Strategy

### MVP First (User Story 1 only)

1. Phase 1 Setup → 2. Phase 2 Foundational (CRITICAL — extraction + binding shell + registration) →
3. Phase 3 US1 (`query`) → **STOP & VALIDATE** the `query` path against real YDB → demo.

### Incremental Delivery

Foundation ready → add `query` (MVP) → add `exec` → add parameters. Each increment is independently
testable and leaves the state store untouched.

---

## Notes

- [P] = different files, no incomplete dependencies.
- The integration test (`tests/integration/binding_test.go`) is the authoritative correctness gate
  (constitution Principle II analogue for bindings); it must run against a real/containerized YDB.
- Commit after each task or logical group; keep `bindings.metadata.yaml` in sync with `Operations()`
  and the parsed metadata fields (Principle IV/V).
- Do not regress the state store: T006 re-runs state conformance after the shared-config extraction.
