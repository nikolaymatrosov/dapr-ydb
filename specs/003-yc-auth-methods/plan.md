# Implementation Plan: Yandex Cloud Production Authentication Methods

**Branch**: `003-yc-auth-methods` | **Date**: 2026-06-18 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/003-yc-auth-methods/spec.md`

## Summary

Wire up the two Yandex Cloud production authentication paths — `serviceAccountKey` (key-file →
auto-refreshed IAM tokens) and `metadata` (instance metadata service) — that are currently parsed
and validated but dead-end in `credentialOptions()` with a "not yet supported" error
([store.go:85-88](../../internal/ydbstate/store.go#L85)). The same change activates the already-parsed
but currently-ignored `useInternalCA` flag so managed-endpoint TLS is trusted. A single new dependency,
`github.com/ydb-platform/ydb-go-yc`, supplies all three driver options
(`WithServiceAccountKeyFileCredentials`, `WithMetadataCredentials`, `WithInternalCA`). No new
`state.Feature` is advertised; the change is confined to connection setup, so existing CRUD/ETag
behavior and the conformance suite are untouched.

## Technical Context

**Language/Version**: Go 1.26.4 (pinned in `go.mod`)

**Primary Dependencies**: `ydb-platform/ydb-go-sdk/v3` v3.140.2 (existing); **new**:
`github.com/ydb-platform/ydb-go-yc` (Yandex Cloud credential options — re-exports the metadata-service
and internal-CA helpers from `ydb-go-yc-metadata`, so one module covers all three paths).

**Storage**: YDB (managed, Yandex Cloud) — unchanged; this feature only affects how the driver
authenticates when opening the connection.

**Testing**: `go test` (unit) for `credentialOptions()` mapping and service-account-key pre-flight
errors; existing conformance suite (`make conformance`, anonymous auth) continues to gate CRUD/ETag.
Live `serviceAccountKey`/`metadata` paths require a real Yandex Cloud environment and are validated
manually / via the quickstart, not in CI.

**Target Platform**: Linux pluggable component process (Unix Domain Socket); the `metadata` path
additionally requires a Yandex Cloud workload (VM/function) exposing the instance metadata endpoint.

**Project Type**: Single Go module — Dapr pluggable state store.

**Performance Goals**: No new hot path. Credential acquisition/refresh is handled by the YC SDK in the
background; token refresh MUST not require operator intervention or restart (SC-005).

**Constraints**: Manifest-only configuration (no env coupling, Principle V); never log key contents or
derived tokens; configuration errors surface at `Init` with field-named messages and never panic.

**Scale/Scope**: Small, surgical change — one `switch` extended, one latent flag honored, one
dependency added, one manifest doc-string corrected. No schema, no data-model change.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **I. State Store Contract Fidelity**: PASS. No change to `state.Store` methods or `Features()`. Auth
  is connection configuration, not a `state.Feature`; nothing new is advertised, so the
  "advertise only what's verified" rule is not engaged.
- **II. Conformance-Verified (NON-NEGOTIABLE)**: PASS with note. No advertised feature changes, so the
  conformance contract is unchanged and the existing anonymous-auth conformance run still gates
  CRUD/ETag. The new auth paths are not `state.Feature`s and cannot be exercised by the suite without a
  live cloud; they are covered by unit tests (option mapping + pre-flight validation) plus a documented
  manual/integration check against real Yandex Cloud (quickstart). See research D5.
- **III. Concurrency/Consistency/TTL**: PASS. Untouched — no read/write/tx/TTL logic changes.
- **IV. Idiomatic, Pluggable YDB Integration**: PASS. Uses the official YC SDK helper module
  (`ydb-go-yc`) rather than re-implementing IAM token exchange; credentials are wired via `ydb.Option`
  in `Init`, consistent with the existing pattern. No `daprd` fork.
- **V. Observability & Operability**: PASS, and improves it — replaces two "not yet supported" dead-ends
  with working paths and field-named config errors; activates `useInternalCA` (previously parsed but
  silently ignored — a latent operability gap). No secrets logged.

**Result**: No violations. Complexity Tracking not required.

## Project Structure

### Documentation (this feature)

```text
specs/003-yc-auth-methods/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   └── auth-methods.md   # Phase 1 output — auth-method → driver-option contract
└── checklists/
    └── requirements.md  # From /speckit-specify
```

### Source Code (repository root)

```text
internal/ydbstate/
├── store.go             # credentialOptions(): wire serviceAccountKey + metadata + useInternalCA
├── metadata.go          # (already parses authMethod, serviceAccountKeyPath, useInternalCA — no change expected)
├── store_test.go        # NEW or extended: unit tests for credentialOptions() mapping + SA-key pre-flight
└── metadata_test.go     # existing — confirm validation coverage for the two methods

metadata.yaml            # remove "not yet wired" caveats from authMethod + serviceAccountKeyPath
go.mod / go.sum          # add github.com/ydb-platform/ydb-go-yc
```

**Structure Decision**: Single Go module, existing layout. All production changes land in
`internal/ydbstate/store.go` (and its unit test); `metadata.yaml` documentation is corrected to match
reality (FR-009/SC-006). No new packages or directories.

## Complexity Tracking

> No Constitution Check violations — section intentionally empty.
