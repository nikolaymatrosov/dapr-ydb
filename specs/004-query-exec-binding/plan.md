# Implementation Plan: YDB Query/Exec Output Binding

**Branch**: `004-query-exec-binding` | **Date**: 2026-06-18 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/004-query-exec-binding/spec.md`

## Summary

Add a second Dapr component — a YDB **output binding** (`bindings.ydb`) exposing `query` and
`exec` operations over raw YQL — served by the existing plugin binary alongside the current
`state.ydb` state store. A caller invokes the binding through the standard binding API; the
component runs the supplied statement against YDB and returns result rows as JSON (`query`) or an
execution summary (`exec`). Parameters are accepted in two forms (resolved in Clarifications): a
postgres-compatible **positional** `params` array, and an optional **named+typed** form for exact
YDB types.

Technical approach: register the binding via `dapr.WithOutputBinding` in the same `dapr.Register`
call as the state store (the SDK appends both onto one gRPC server / socket). Execute statements
through the YDB **`database/sql`** layer (`sql.OpenDB(ydb.MustConnector(driver, …))`) because its
`WithAutoDeclare()` + `WithPositionalArgs()` connector options give postgres-style `?`-positional
parameters with automatic type declaration for free — exactly the positional model the spec wants.
Connection and authentication configuration (including the Yandex Cloud auth methods) is shared
with the state store by extracting the existing metadata parsing + credential-option logic into a
new `internal/ydbconfig` package that both components consume.

## Technical Context

**Language/Version**: Go (version per `go.mod`; currently toolchain 1.26, matching `components-go-sdk` / `components-contrib`).

**Primary Dependencies**: `dapr-sandbox/components-go-sdk` v0.3.0 (pluggable host, `WithOutputBinding`), `dapr/components-contrib` v1.18.0 (`bindings` interface types), `ydb-platform/ydb-go-sdk/v3` v3.140.2 (persistence + `database/sql` connector with `WithAutoDeclare`/`WithPositionalArgs`/`WithNumericArgs`). No new module dependency required — all three are already in `go.mod`.

**Storage**: YDB (Yandex Database), addressed by the same connection string the state store uses.

**Testing**: `go test` unit tests for param building / result serialization (no DB); integration test against a real/containerized YDB instance exercising `query`, `exec`, and parameterized statements (the authoritative gate, per constitution Development Workflow).

**Target Platform**: Linux server (pluggable component process; gRPC over Unix Domain Socket).

**Project Type**: Single Go project — a pluggable Dapr component binary serving multiple component types.

**Performance Goals**: Not latency-critical; result sets assumed to fit a single binding response (no streaming) per spec Assumptions.

**Constraints**: Must not crash the host process or the co-resident state store on any input/DB error (FR-008); configuration is manifest-only (constitution Principle V); the state store's behavior and advertised features are unchanged (spec Assumptions).

**Scale/Scope**: One new component; two operations; bounded result sizes. New code: `internal/ydbbinding/` package, extracted `internal/ydbconfig/` package, one binding `metadata.yaml`, `main.go` registration, integration test.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

The constitution (`v1.1.0`) is written around the **state store**; Principles I–III are largely
state-store-specific. The binding adheres to the applicable principles and the project-wide gates:

- **I. Contract Fidelity (analogue)**: `Operations()` MUST advertise exactly the operations the
  binding implements (`query`, `exec`) — the binding analogue of `Features()` honesty. ✔ Planned.
  The state store's `state.Store` contract is untouched.
- **II. Conformance-Verified (NON-NEGOTIABLE)**: the state conformance suite does not cover
  bindings. The equivalent objective gate is an **integration test against a real YDB instance**
  exercising every advertised operation + the parameterized path before done (mirrors the
  Development-Workflow "real YDB, not a mock" rule and spec FR-014/SC-002). ✔ Planned. The existing
  state conformance run still gates the unchanged store and the `ydbconfig` refactor.
- **III. Concurrency/Consistency/TTL**: N/A to a stateless SQL pass-through. "Value as bytes" does
  not apply — the binding's contract is JSON rows, not opaque state values.
- **IV. Idiomatic, Pluggable YDB Integration**: registers via the SDK (`WithOutputBinding`) in the
  same process, no `daprd` rebuild; ships a binding `metadata.yaml`; uses the official YDB SDK;
  releases the driver on `Close`. ✔ Planned.
- **V. Observability & Operability**: structured logs on lifecycle/error paths; actionable `Init`
  errors (never panic on bad metadata); manifest-only config; DB/connection errors degrade
  predictably without taking down the sidecar. ✔ Planned. `Ping` provided for health checking.

**Result: PASS.** No violations; Complexity Tracking not required. Note (non-blocking): the
constitution predates a second component type; a future amendment could generalize Principles I–II
to "every advertised component contract," but adding a binding to the same pluggable process is
consistent with Principle IV today.

## Project Structure

### Documentation (this feature)

```text
specs/004-query-exec-binding/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   └── binding-operations.md   # query/exec request & response contract
├── checklists/
│   └── requirements.md  # spec quality checklist (from /speckit-specify)
└── tasks.md             # Phase 2 output (/speckit-tasks)
```

### Source Code (repository root)

```text
cmd/daprd-ydb/
└── main.go                  # MODIFIED: register state store + output binding on one socket

internal/ydbconfig/          # NEW: shared connection + auth configuration
├── config.go                # Config struct, Parse(props map[string]string), CredentialOptions(), Open()
└── config_test.go           # parse/validation table tests (moved from ydbstate)

internal/ydbstate/           # MODIFIED: delegate metadata parsing + credentialOptions to ydbconfig
├── store.go                 # Init uses ydbconfig.Open(); credentialOptions() removed/relocated
├── metadata.go              # thin wrapper over ydbconfig (or removed)
└── ...                      # Get/Set/Delete unchanged

internal/ydbbinding/         # NEW: the output binding
├── binding.go               # YDBBinding: Init / Invoke / Operations / Close / Ping; opens database/sql
├── params.go                # positional + named/typed parameter construction
├── result.go                # rows -> []map[string]any -> JSON serialization
└── binding_test.go          # unit tests: param building, result serialization, error mapping

bindings.metadata.yaml       # NEW: component manifest for type bindings.ydb (alongside metadata.yaml)

tests/
└── integration/             # NEW (or extend tests/conformance): binding query/exec/params vs real YDB
    └── binding_test.go
```

**Structure Decision**: Single Go project, multiple component types in one binary. The binding lives
in a sibling package `internal/ydbbinding` mirroring `internal/ydbstate`. Connection/auth code is
hoisted into `internal/ydbconfig` so both components parse the same manifest fields and share the
exact credential logic (including the Yandex Cloud auth methods from feature 003) — no duplicated,
security-sensitive auth code. `main.go` registers both on socket name `ydb`.

## Dependencies & Sequencing

- This branch descends from `003-yc-auth-methods`. The `ydbconfig` extraction relocates
  `credentialOptions()` (which feature 003 is wiring for `serviceAccountKey`/`metadata` + the
  `useInternalCA` flag). Sequence the extraction **after** 003's `credentialOptions` is in place so
  the moved code is the final version; the binding inherits whichever auth methods the shared
  package supports with no extra work.
- The `ydbconfig` extraction MUST be behavior-preserving for the state store: the state conformance
  suite passing post-refactor is the regression gate (constitution Principle II).

## Complexity Tracking

> No constitutional violations require justification. Section intentionally empty.
