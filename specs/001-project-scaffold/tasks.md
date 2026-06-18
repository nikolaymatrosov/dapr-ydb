---
description: "Task list for Project Scaffold — pluggable Dapr state store for YDB"
---

# Tasks: Project Scaffold

**Input**: Design documents from `/specs/001-project-scaffold/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: INCLUDED. The spec requires them (FR-009 build/lint/test commands, FR-010 conformance
harness) and constitution Principle II makes conformance the non-negotiable correctness gate.
US2/US3 acceptance criteria depend on unit + conformance tests.

**Organization**: Tasks are grouped by user story (US1 P1, US2 P2, US3 P3) for independent
implementation and testing.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: US1 / US2 / US3 (Setup, Foundational, Polish carry no story label)
- All paths are relative to the repository root.

## Path Conventions

- Binary: `cmd/daprd-ydb/`
- Store package: `internal/ydbstate/`
- Manifests: `metadata.yaml` (root), `components/ydb.yaml` (sample)
- Tests: `internal/ydbstate/*_test.go` (unit), `tests/conformance/` (conformance)
- Tooling: `Makefile`, `.golangci.yml`, `deploy/`

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Initialize the Go module and project tooling. Includes the dependency-compatibility
spike that gates all subsequent work.

- [X] T001 Initialize Go module in `go.mod` with module path `github.com/nikolaymatrosov/dapr-ydb` and `go 1.24`
- [X] T002 **[BLOCKING SPIKE]** Add `github.com/dapr-sandbox/components-go-sdk@v0.3.0` and `github.com/ydb-platform/ydb-go-sdk/v3` (latest v3.140.x), run `go mod tidy` + a throwaway `go build ./...`, and confirm the 2023-era gRPC/protobuf pin and the 2026 YDB SDK coexist (research.md D6). If the build breaks, apply the mitigation ladder (pin an older ydb-go-sdk → add `replace`/`exclude` → vendor the SDK shim) and record the resolution in `research.md`
- [X] T003 [P] Add `.gitignore` for Go (bin/, vendor/, coverage, env files)
- [X] T004 [P] Add `.golangci.yml` lint configuration
- [X] T005 [P] Create `Makefile` with `build`, `lint`, `test`, `conformance`, and `run` targets per quickstart.md
- [X] T006 [P] Create `deploy/docker-compose.yml` (local YDB instance) and `deploy/Dockerfile` (build the pluggable component image)

**Checkpoint**: `go build ./...` succeeds with both SDKs resolved; tooling commands exist.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The shared `YDBStore` backbone and project skeleton that every user story extends.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [X] T007 Create the directory skeleton: `cmd/daprd-ydb/`, `internal/ydbstate/`, `components/`, `tests/conformance/`, `deploy/`
- [X] T008 Define the `YDBStore` type with a `New()` constructor that embeds `state.NewDefaultBulkStore(self)` for `BulkGet/BulkSet/BulkDelete`, plus the compile-time assertion `var _ state.Store = (*YDBStore)(nil)`, in `internal/ydbstate/store.go`
- [X] T009 [P] Add typed errors including the `errNotImplemented` sentinel in `internal/ydbstate/errors.go`
- [X] T010 [P] Add a structured logger field on `YDBStore` (wired from the SDK-provided logger) in `internal/ydbstate/store.go`

**Checkpoint**: The package compiles and `YDBStore` satisfies `state.Store` at compile time.

---

## Phase 3: User Story 1 - Loadable pluggable component skeleton (Priority: P1) 🎯 MVP

**Goal**: A built component that registers `state.ydb` over a Unix socket and is discovered and
reported healthy by the Dapr sidecar — with persistence stubbed.

**Independent Test**: Build, run so `ydb.sock` appears in the sockets folder, start `daprd` with
`components/ydb.yaml`, and confirm the sidecar loads `state.ydb` and its health check passes
(quickstart.md steps 1–2).

### Tests for User Story 1 ⚠️

> Write these FIRST and ensure they FAIL before implementation.

- [X] T011 [P] [US1] Unit test in `internal/ydbstate/store_test.go`: assert `Features()` returns an empty slice, `YDBStore` satisfies `state.Store`, and `Get`/`Set`/`Delete` return `errNotImplemented`

### Implementation for User Story 1

- [X] T012 [US1] Implement stub `Get`, `Set`, `Delete` returning `errNotImplemented` in `internal/ydbstate/store.go`
- [X] T013 [US1] Implement `Features()` returning an empty `[]state.Feature` (advertise nothing unimplemented — constitution Principle I) in `internal/ydbstate/store.go`
- [X] T014 [US1] Implement `GetComponentMetadata()` in `internal/ydbstate/store.go`
- [X] T015 [US1] Implement `cmd/daprd-ydb/main.go`: `dapr.Register("ydb", dapr.WithStateStore(func() state.Store { return ydbstate.New() }))` + `dapr.MustRun()`
- [X] T016 [P] [US1] Create the sample component manifest `components/ydb.yaml` with `spec.type: state.ydb`, `version: v1`, and an anonymous-auth local DSN
- [X] T017 [US1] Validate end-to-end per quickstart.md steps 1–2: `make build`, `make run`, confirm `ydb.sock` is created and `daprd` loads `state.ydb` and reports it healthy

**Checkpoint**: MVP — the component loads in Dapr and passes its health check.

---

## Phase 4: User Story 2 - Declared, validated configuration (Priority: P2)

**Goal**: Manifest-driven config parsed and validated at `Init`, opening the YDB connection, with
actionable field-named errors and clean shutdown.

**Independent Test**: A valid manifest initializes successfully; a missing/invalid required field
fails `Init` with a message naming the offending field, without crashing the host (quickstart.md
step 3).

### Tests for User Story 2 ⚠️

> Write these FIRST and ensure they FAIL before implementation.

- [X] T018 [P] [US2] Unit tests in `internal/ydbstate/metadata_test.go`: valid manifest parses; missing `connectionString` errors naming it; `authMethod=static` without `username` errors naming it; unparseable `useInternalCA` errors naming it and the expected form; no panics

### Implementation for User Story 2

- [X] T019 [P] [US2] Define the `storeMetadata` struct (fields per data-model.md) in `internal/ydbstate/metadata.go`
- [X] T020 [US2] Implement parsing of `storeMetadata` from `state.Metadata.Properties` in `internal/ydbstate/metadata.go`
- [X] T021 [US2] Implement validation with field-named errors (required `connectionString`, exactly-one auth mode with required sub-fields, bool parsing) in `internal/ydbstate/metadata.go`
- [X] T022 [US2] Wire `Init(ctx, state.Metadata)` in `internal/ydbstate/store.go`: parse+validate → `ydb.Open` with the auth option for the selected `authMethod` → store the driver handle; return (never panic) on any failure
- [X] T023 [US2] Implement clean shutdown (`driver.Close(ctx)`, no orphaned resources) in `internal/ydbstate/store.go`
- [X] T024 [P] [US2] Author the root `metadata.yaml` manifest documenting every field `Init` reads, derived from `contracts/metadata.yaml` (constitution Principle V, FR-007)

**Checkpoint**: Configuration is manifest-only, validated, and connects to YDB; US1 + US2 both work.

---

## Phase 5: User Story 3 - Repeatable build, test, and conformance harness (Priority: P3)

**Goal**: One-command build/lint/test plus a compiling, runnable Dapr state conformance harness.

**Independent Test**: `make build`/`make lint`/`make test` succeed from a clean checkout; the
conformance harness compiles and runs against a YDB target (quickstart.md steps 4–5).

### Tests for User Story 3 ⚠️

- [X] T025 [P] [US3] Create the conformance harness `tests/conformance/conformance_test.go` importing `github.com/dapr/components-contrib/tests/conformance/state` and driving `YDBStore`
- [X] T026 [P] [US3] Create the conformance component config `tests/conformance/ydb.yaml`

### Implementation for User Story 3

- [X] T027 [US3] Finalize the `Makefile` `test` (unit, excludes conformance) and `conformance` (brings up YDB via `deploy/docker-compose.yml`, runs the suite) targets
- [X] T028 [P] [US3] Write `README.md`: prerequisites, build/run, configuration (link `metadata.yaml`), and how to run unit + conformance tests (FR-011)

**Checkpoint**: All three stories independently functional; the quality gate is runnable.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Verification and hygiene across the scaffold.

- [X] T029 [P] Run the full quickstart.md (steps 1–6) end-to-end and fix any gaps
- [X] T030 [P] Ensure `make lint` and `gofmt`/`go vet` are clean across the module
- [X] T031 Constitution compliance re-check: confirm `Features()` advertises only implemented capabilities and that `metadata.yaml` matches the fields `Init` actually parses
- [X] T032 [P] Add a CI workflow in `.github/workflows/ci.yml` running build + lint + unit tests (conformance optional/gated on YDB availability)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies. **T002 is a blocking spike** — if both SDKs cannot coexist, all later work is blocked until the mitigation ladder resolves it.
- **Foundational (Phase 2)**: Depends on Setup (esp. T001, T002). BLOCKS all user stories.
- **User Stories (Phase 3–5)**: All depend on Foundational completion.
  - **US1 (P1)**: independent after Foundational.
  - **US2 (P2)**: independent after Foundational; integrates with US1's `Init` but is testable on its own via metadata unit tests.
  - **US3 (P3)**: the harness (T025–T026, T028) can be authored in parallel after Foundational, but **running** it end-to-end (T027) requires US1 (registration) and US2 (real `Init`/connection).
- **Polish (Phase 6)**: Depends on the desired user stories being complete.

### User Story Dependencies

- US1 → none (MVP).
- US2 → none for its unit tests; shares the `Init` path with US1.
- US3 → authoring independent; full conformance run depends on US1 + US2.

### Within Each User Story

- Tests are written and FAIL before implementation.
- `storeMetadata` (model) before `Init` wiring (service) before end-to-end validation.

### Parallel Opportunities

- Setup: T003, T004, T005, T006 in parallel (after T001; T002 spike should resolve first).
- Foundational: T009, T010 in parallel after T008.
- US1: T011 and T016 in parallel; T012–T014 touch the same `store.go` (sequential).
- US2: T018, T019, T024 in parallel; T020–T023 touch shared files (sequential).
- US3: T025, T026, T028 in parallel.
- Polish: T029, T030, T032 in parallel.

---

## Parallel Example: User Story 1

```bash
# After Foundational completes, launch the independent US1 tasks together:
Task: "T011 [US1] Unit test for Features()/interface/stub ops in internal/ydbstate/store_test.go"
Task: "T016 [US1] Create sample component manifest components/ydb.yaml"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1 Setup (resolve the T002 SDK-compatibility spike first).
2. Complete Phase 2 Foundational.
3. Complete Phase 3 US1.
4. **STOP and VALIDATE**: the component loads in Dapr and passes its health check.

### Incremental Delivery

1. Setup + Foundational → backbone ready.
2. US1 → loadable skeleton (MVP) → demo in Dapr.
3. US2 → manifest-driven, validated config connecting to YDB.
4. US3 → build/test/conformance quality gate.
5. Polish → verification and CI.

Subsequent features (separate specs) then fill in real persistence: Get/Set/Delete → bulk →
ETag → TTL → transactions → query, each advertised in `Features()` only after passing
conformance.

---

## Notes

- [P] = different files, no incomplete-task dependencies.
- The scaffold intentionally advertises **no** `Features()` and stubs persistence ops with
  `errNotImplemented` — never a false success (constitution Principle I).
- T002 is the highest-risk item; do not proceed past Setup until the module builds with both SDKs.
- Commit after each task or logical group.
