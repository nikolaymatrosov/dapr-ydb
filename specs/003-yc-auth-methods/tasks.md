---
description: "Task list for Yandex Cloud Production Authentication Methods"
---

# Tasks: Yandex Cloud Production Authentication Methods

**Input**: Design documents from `specs/003-yc-auth-methods/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/auth-methods.md

**Tests**: Unit tests are INCLUDED — research D5 designates `credentialOptions()` mapping + the
service-account-key pre-flight as the primary automatable coverage (the live cloud paths cannot run in
CI). Conformance (anonymous auth) is unchanged and re-run as a regression gate.

**Organization**: Tasks are grouped by the three user stories from spec.md. All three modify the single
function `credentialOptions()` in `internal/ydbstate/store.go`, so cross-story `[P]` is limited; each
story is nonetheless an independently shippable increment.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: US1 = serviceAccountKey, US2 = metadata, US3 = useInternalCA

## Path Conventions

Single Go module. Production code in `internal/ydbstate/`; manifest at repo root `metadata.yaml`.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Pull in the one new dependency that supplies all three Yandex Cloud driver options.

- [X] T001 Add the Yandex Cloud credential module: run `go get github.com/ydb-platform/ydb-go-yc@latest` then `go mod tidy`, and confirm `github.com/ydb-platform/ydb-go-yc` appears in `go.mod` and `go.sum`.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Reshape `credentialOptions()` into the "build one base credential option, then extend" form that all three stories plug into — with **no behavior change** and no new import yet.

**⚠️ CRITICAL**: Blocks US1/US2/US3.

- [X] T002 Refactor `credentialOptions()` in `internal/ydbstate/store.go` so the `switch s.md.AuthMethod` assigns a single base `ydb.Option` for `anonymous`/`static`/`token` and returns `[]ydb.Option{base}, nil`; leave the `serviceAccountKey` and `metadata` arms returning their existing "not yet supported" error for now, and the `default` arm unchanged. Verify `go build ./...` and `go test ./internal/ydbstate/...` still pass (behavior identical).

**Checkpoint**: `credentialOptions()` has the extension point; stories can now fill the two arms and add the CA append.

---

## Phase 3: User Story 1 - Connect using a service account key (Priority: P1) 🎯 MVP

**Goal**: `authMethod=serviceAccountKey` produces a working managed-YDB connection with auto-refreshed credentials; misconfiguration fails `Init` with a field-named error.

**Independent Test**: Unit-test that a missing/unreadable `serviceAccountKeyPath` returns a field-named error and a readable key file returns a non-empty option slice; end-to-end via quickstart §1 against real Yandex Cloud.

### Tests for User Story 1 ⚠️ (write first, expect failure until T011)

- [X] T003 [P] [US1] In `internal/ydbstate/store_test.go` (new file, package `ydbstate`, reuse `newMeta` from `metadata_test.go`), add `TestCredentialOptions_ServiceAccountKey_MissingPath`: build a store with `authMethod=serviceAccountKey` and no `serviceAccountKeyPath`, assert `Init`/`credentialOptions` returns an error naming `serviceAccountKeyPath`.
- [X] T004 [P] [US1] In `internal/ydbstate/store_test.go`, add `TestCredentialOptions_ServiceAccountKey_UnreadableFile`: point `serviceAccountKeyPath` at a non-existent path, assert a field-named "cannot read key file" error.
- [X] T005 [P] [US1] In `internal/ydbstate/store_test.go`, add `TestCredentialOptions_ServiceAccountKey_ReadableFile`: write a temp file (`t.TempDir()`), set `serviceAccountKeyPath` to it, assert `credentialOptions()` returns a non-empty option slice and `nil` error.

### Implementation for User Story 1

- [X] T006 [US1] In `internal/ydbstate/store.go`, add the import `yc "github.com/ydb-platform/ydb-go-yc"` and implement the `serviceAccountKey` arm of `credentialOptions()`: pre-flight `os.ReadFile(s.md.ServiceAccountKeyPath)` and on error return `fmt.Errorf("metadata field 'serviceAccountKeyPath': cannot read key file %q: %w", path, err)`; otherwise set base `= yc.WithServiceAccountKeyFileCredentials(s.md.ServiceAccountKeyPath)`. Update the `credentialOptions` doc comment to drop the "later feature" wording. (`os` is already imported.)

**Checkpoint**: US1 fully functional — T003–T005 pass; MVP shippable.

---

## Phase 4: User Story 2 - Connect using the instance metadata service (Priority: P2)

**Goal**: `authMethod=metadata` produces a working connection on a cloud workload with no secret in the manifest.

**Independent Test**: Unit-test that `metadata` returns a non-empty option slice with no required config; end-to-end via quickstart §2 on a Yandex Cloud VM.

**Depends on**: T006 (the `yc` import added in US1).

### Tests for User Story 2 ⚠️

- [X] T007 [P] [US2] In `internal/ydbstate/store_test.go`, add `TestCredentialOptions_Metadata`: build a store with `authMethod=metadata` and only `connectionString`, assert `credentialOptions()` returns a non-empty option slice and `nil` error (no secret required).

### Implementation for User Story 2

- [X] T008 [US2] In `internal/ydbstate/store.go`, implement the `metadata` arm of `credentialOptions()`: set base `= yc.WithMetadataCredentials()` and drop its "not yet supported" error and "later feature" doc wording.

**Checkpoint**: US1 AND US2 both functional and independently testable.

---

## Phase 5: User Story 3 - Trust the managed-endpoint internal CA (Priority: P3)

**Goal**: Honor the previously-ignored `useInternalCA` flag, composable with every auth method.

**Independent Test**: Unit-test that `useInternalCA=true` yields exactly one more option than `false`, across at least two auth methods.

**Depends on**: T006 (the `yc` import added in US1).

### Tests for User Story 3 ⚠️

- [X] T009 [P] [US3] In `internal/ydbstate/store_test.go`, add `TestCredentialOptions_InternalCA_AppendsOption`: for `authMethod=anonymous` and again for `metadata`, assert the option-slice length with `useInternalCA=true` is exactly one greater than with `useInternalCA=false`.

### Implementation for User Story 3

- [X] T010 [US3] In `internal/ydbstate/store.go`, after the `switch` in `credentialOptions()` builds the base option, append `yc.WithInternalCA()` to the returned slice when `s.md.UseInternalCA` is true — applied uniformly to all auth methods.

**Checkpoint**: All three auth/CA behaviors functional and independently tested.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Reconcile the manifest with reality, regression-gate, and run the live validation.

- [X] T011 [P] Update `metadata.yaml`: remove the "not yet wired / later feature" caveats from the `authMethod` and `serviceAccountKeyPath` descriptions (keep `allowedValues` intact) so the published manifest matches runtime behavior (FR-009, SC-006).
- [X] T012 [P] In `internal/ydbstate/metadata_test.go`, confirm (add if missing) cases asserting `serviceAccountKey` with no path errors and an unknown `authMethod` is rejected with the full accepted-values list (FR-010).
- [X] T013 Run `go build ./... && go vet ./... && go test ./internal/ydbstate/...` and confirm all pass.
- [X] T014 Run `make conformance` (anonymous auth) and confirm CRUD/ETag scenarios still pass — regression gate for Principle II (no advertised-feature change).
- [ ] T015 [P] Execute `quickstart.md` §1 (serviceAccountKey) and §2 (metadata) against a real Yandex Cloud environment; record the outcome (these live paths are not covered by CI).

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (T001)**: no dependencies — start immediately.
- **Foundational (T002)**: depends on T001 — BLOCKS all stories.
- **US1 (T003–T006)**: depends on T002. Adds the `yc` import (T006).
- **US2 (T007–T008)** and **US3 (T009–T010)**: depend on T002 and on T006 (for the `yc` import). Independent of each other in logic, but edit the same function — sequence US2 then US3 to avoid merge churn.
- **Polish (T011–T015)**: depends on the stories whose behavior they validate; T013/T014 after all code lands.

### Within Each User Story

- Write the story's `[P]` unit tests first (they fail against the still-erroring arm), then implement to green.

### Parallel Opportunities

- T003, T004, T005 (US1 tests) are `[P]` together (same new file, independent functions — author together, or serialize trivially).
- Within a story, tests are `[P]`; the single implementation task per story is **not** `[P]` with another story's implementation task (all edit `credentialOptions()` in `store.go`).
- Polish: T011 (metadata.yaml) and T012 (metadata_test.go) and T015 (manual) are `[P]`; T013/T014 are sequential gates.

---

## Parallel Example: User Story 1

```bash
# Author the three US1 unit-test functions together (same file, independent funcs):
Task: "TestCredentialOptions_ServiceAccountKey_MissingPath in internal/ydbstate/store_test.go"
Task: "TestCredentialOptions_ServiceAccountKey_UnreadableFile in internal/ydbstate/store_test.go"
Task: "TestCredentialOptions_ServiceAccountKey_ReadableFile in internal/ydbstate/store_test.go"
# Then implement T006 to turn them green.
```

---

## Implementation Strategy

### MVP First (User Story 1 only)

1. T001 (dependency) → T002 (foundational refactor).
2. T003–T006 (serviceAccountKey + pre-flight + tests).
3. **STOP and VALIDATE**: `go test ./internal/ydbstate/...` green; optionally quickstart §1 against real cloud.
4. Ship — serviceAccountKey is the primary production path and delivers most of the value alone.

### Incremental Delivery

1. Setup + Foundational → extension point ready.
2. US1 (serviceAccountKey) → test → ship (MVP).
3. US2 (metadata) → test → ship.
4. US3 (useInternalCA) → test → ship.
5. Polish: reconcile `metadata.yaml`, run build/vet/test + conformance, live-validate.

---

## Notes

- All three stories edit `credentialOptions()` in `internal/ydbstate/store.go`; treat implementation tasks across stories as sequential, not parallel.
- The `yc` import is introduced in T006 (US1); US2/US3 reuse it — keep US1 first in the sequence.
- No `state.Feature` changes; `Features()`, the stored schema, and Get/Set/Delete are untouched.
- Never log key file contents or derived tokens.
- Commit after each task or logical group.
