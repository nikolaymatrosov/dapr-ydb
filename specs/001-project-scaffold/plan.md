# Implementation Plan: Project Scaffold

**Branch**: `001-project-scaffold` | **Date**: 2026-06-18 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/001-project-scaffold/spec.md`

## Summary

Stand up the foundational repository for a **pluggable Dapr state store backed by YDB**: a Go
module, a runnable pluggable-component binary that registers a `state.ydb` store over a Unix
Domain Socket and is discoverable/health-checkable by the Dapr sidecar, manifest-driven
configuration parsed and validated at `Init`, and a build/lint/test toolchain wired to Dapr's
state conformance suite. Persistence semantics (real Get/Set/Delete, bulk, ETags, TTL,
transactions, query) are intentionally stubbed here and delivered by later features; the
scaffold advertises **no** capabilities it does not implement (constitution Principle I).

## Technical Context

**Language/Version**: Go 1.24+ (toolchain present: go1.26.1). Floor is dictated by
`ydb-go-sdk/v3` which requires Go 1.24.

**Primary Dependencies**:
- `github.com/dapr-sandbox/components-go-sdk` (pluggable host; latest tag **v0.3.0**) — provides
  `dapr.Register(...)`, `dapr.WithStateStore(...)`, `dapr.MustRun()`, and the
  `components-go-sdk/state/v1` interface that embeds contrib's `state.Store`.
- `github.com/dapr/components-contrib/state` (the `state.Store` contract types) — version is
  transitively pinned by the SDK (Dapr 1.11-era, `v1.11.3-0.2023...`).
- `github.com/ydb-platform/ydb-go-sdk/v3` (YDB client; latest **v3.140.x**) for the connection
  pool opened in `Init`.

**Storage**: YDB (Yandex Database). Local dev/test via a YDB container; production via remote/
serverless YDB over `grpcs://`.

**Testing**: Go's `testing` + Dapr state conformance suite
(`github.com/dapr/components-contrib/tests/conformance/state`) run against a real YDB instance.
Unit tests for metadata parsing/validation. `golangci-lint` for linting.

**Target Platform**: Linux/macOS server process (the pluggable component runs as its own
process beside `daprd`, communicating over a Unix Domain Socket).

**Project Type**: Single Go project — a CLI/daemon binary plus an internal library package.

**Performance Goals**: Scaffold-level only — clean build, sidecar load + health check on first
attempt (spec SC-002), checkout-to-running in under 15 minutes (SC-001). Request-path
performance targets belong to later persistence features.

**Constraints**:
- Component MUST be loadable without rebuilding `daprd` (pluggable model).
- `Features()` MUST return only implemented capabilities → scaffold returns an empty/feature-
  minimal set.
- Configuration MUST come solely from the declared manifest; no hidden env coupling
  (constitution Principle V). (The Dapr-standard `DAPR_COMPONENTS_SOCKETS_FOLDER` for socket
  placement is an SDK/runtime convention, not store configuration.)

**Scale/Scope**: Foundation only: ~1 binary, 1 internal package, 1 metadata manifest, 1
conformance harness, build tooling, and docs.

### Resolved unknowns (see research.md)

- **Module path**: `github.com/nikolaymatrosov/dapr-ydb` (no git remote configured yet;
  changeable with a single `go.mod` edit + import rewrite).
- **Health/Ping**: the contrib `state.Store` at the pinned SDK version exposes **no `Ping`
  method**; liveness is served by the SDK gRPC framework. FR-004 is satisfied by the framework,
  not by a method we implement.
- **Register/type mapping**: `dapr.Register("ydb", ...)` creates socket `ydb.sock` and is
  addressed by component manifests with `spec.type: state.ydb`.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Constitution v1.0.0 — evaluation for this scaffold feature:

| Principle | Gate | Status |
|-----------|------|--------|
| I. State Store Contract Fidelity | Implements `state.Store` (incl. required `GetComponentMetadata`, `Bulk*` via `DefaultBulkStore`); `Features()` advertises only what is implemented | ✅ PASS — scaffold implements the full interface with stub single-key ops and advertises **no** unimplemented features |
| II. Conformance-Verified (NON-NEGOTIABLE) | Conformance harness wired before release | ✅ PASS — US3 delivers a compiling, runnable harness; because the scaffold advertises no features, the suite has minimal surface to assert, which is the correct honest baseline |
| III. Correct Concurrency/Consistency/TTL Semantics | ETag/transaction/TTL/bytes semantics honored when advertised | ✅ PASS (vacuously) — none advertised yet; deferred to later features, no false advertisement |
| IV. Idiomatic, Pluggable YDB Integration | UDS, no `daprd` rebuild, registers `state.ydb`, ships `metadata.yaml`, uses YDB Go SDK, clean lifecycle | ✅ PASS — all established by the scaffold |
| V. Observability & Operability | Structured logs, `Init`-time validation with actionable errors, fail-safe, manifest-only config | ✅ PASS — delivered by US2 + logging setup |

**Result**: PASS, no violations. Complexity Tracking not required.

## Project Structure

### Documentation (this feature)

```text
specs/001-project-scaffold/
├── plan.md              # This file (/speckit-plan command output)
├── research.md          # Phase 0 output (/speckit-plan command)
├── data-model.md        # Phase 1 output (/speckit-plan command)
├── quickstart.md        # Phase 1 output (/speckit-plan command)
├── contracts/           # Phase 1 output (/speckit-plan command)
│   ├── state-store-interface.md   # The state.Store contract the component satisfies
│   └── metadata.yaml              # Component metadata schema (fields the store reads)
└── checklists/
    └── requirements.md  # Spec quality checklist (/speckit-specify output)
```

### Source Code (repository root)

```text
cmd/
└── daprd-ydb/
    └── main.go              # dapr.Register("ydb", dapr.WithStateStore(...)) + dapr.MustRun()

internal/
└── ydbstate/
    ├── store.go             # YDBStore: implements state.Store (stubbed single-key ops)
    ├── store_test.go        # unit tests for Features()/interface compliance
    ├── metadata.go          # storeMetadata struct + parse/validate from state.Metadata
    └── metadata_test.go     # unit tests for config parsing + validation errors

components/
└── ydb.yaml                 # sample/operator-facing component manifest (spec.type: state.ydb)

metadata.yaml                # component metadata manifest (documents every field Init reads)

tests/
└── conformance/
    ├── conformance_test.go  # wires Dapr's state conformance suite to YDBStore
    └── ydb.yaml             # conformance component config

deploy/
├── docker-compose.yml       # local YDB instance for tests/dev
└── Dockerfile               # build the pluggable component image

Makefile                     # build / lint / test / conformance / run targets
.golangci.yml                # lint configuration
go.mod / go.sum
README.md                    # prerequisites, build/run, configuration, testing
```

**Structure Decision**: Single Go project. The binary lives under `cmd/daprd-ydb` and the
store logic under `internal/ydbstate` so later persistence features extend one package without
restructuring (spec FR-012, SC-006). The metadata manifest sits at repo root (the conventional
location for a Dapr component manifest) with a runnable sample under `components/`.

## Complexity Tracking

> No constitution violations — section intentionally empty.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| — | — | — |
