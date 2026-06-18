<!--
SYNC IMPACT REPORT
==================
Version change: (template) → 1.0.0
Bump rationale: Initial ratification — first concrete constitution replacing the
  unfilled template. Establishes the full set of governing principles.

Modified principles: N/A (initial adoption)
  - [PRINCIPLE_1_NAME] → I. State Store Contract Fidelity
  - [PRINCIPLE_2_NAME] → II. Conformance-Verified (NON-NEGOTIABLE)
  - [PRINCIPLE_3_NAME] → III. Correct Concurrency, Consistency & TTL Semantics
  - [PRINCIPLE_4_NAME] → IV. Idiomatic, Pluggable YDB Integration
  - [PRINCIPLE_5_NAME] → V. Observability & Operability

Added sections:
  - Technology & Architecture Constraints (was [SECTION_2_NAME])
  - Development Workflow & Quality Gates (was [SECTION_3_NAME])

Removed sections: None

Templates requiring updates:
  - ✅ .specify/templates/plan-template.md (Constitution Check is constitution-driven;
       no hard-coded gates needed — no change required)
  - ✅ .specify/templates/spec-template.md (no constitution-coupled sections — no change)
  - ✅ .specify/templates/tasks-template.md (task categories remain compatible — no change)

Follow-up TODOs: None — all placeholders resolved.
-->

# Dapr Pluggable State Store for YDB Constitution

## Core Principles

### I. State Store Contract Fidelity

The component MUST implement Dapr's `state.Store` interface exactly as defined by
`dapr/components-contrib`, exposed through the `dapr-sandbox/components-go-sdk` pluggable
gRPC contract. `Init`, `Features`, `Get`, `Set`, and `Delete` are mandatory; `BulkGet`,
`BulkSet`, and `BulkDelete` MUST be provided (delegating to the single-key operations via
the SDK's bulk helper unless YDB offers a more efficient batch path). `Features()` MUST
advertise ONLY capabilities that are fully implemented and verified — advertising a feature
the code does not honor (e.g., `FeatureETag`, `FeatureTransactional`, `FeatureTTL`) is a
contract violation because the Dapr runtime gates behavior on this list. Optional interfaces
(`TransactionalStore`/`Multi`, `Querier`/`Query`) MUST be implemented when, and only when,
their corresponding feature is advertised.

**Rationale**: The runtime trusts `Features()` to decide what operations to route. A
faithful, honest contract is the difference between a store that works and one that silently
corrupts actor state or drops data.

### II. Conformance-Verified (NON-NEGOTIABLE)

Dapr's state conformance test suite (`tests/conformance/state`) is the authoritative
definition of correctness. The component MUST pass the conformance suite for every feature
it advertises before any release or merge to the main branch. New behavior MUST be
demonstrated against the suite (or an integration test exercising the same contract) BEFORE
it is considered done. A failing or skipped conformance assertion blocks release; it is never
waived by assertion of "works on my machine."

**Rationale**: Hand-written unit tests cannot capture the full matrix of ETag, TTL, bulk,
and transactional edge cases the runtime depends on. Conformance is the only objective gate.

### III. Correct Concurrency, Consistency & TTL Semantics

The component MUST honor the semantics the `state.Store` contract requires:
- **ETags / optimistic concurrency**: `Set` and `Delete` carry an optional ETag. A mismatch
  MUST return `state.NewETagError(state.ETagMismatch, ...)`; a malformed ETag MUST return
  `state.NewETagError(state.ETagInvalid, ...)`. ETags MUST be generated as opaque values
  (e.g., random UUIDs) and never reused across writes.
- **Transactions**: when `actorStateStore: true` / `FeatureTransactional` is advertised,
  `Multi` MUST execute all operations atomically under a single YDB serializable transaction —
  all-or-nothing, with ETag checks enforced inside the transaction.
- **TTL**: expired records MUST NOT be returned by reads and MUST be reclaimed. Native YDB TTL
  is preferred; if an expiry column with background GC is used instead, reads MUST filter
  expired rows regardless of GC timing.
- **Value as bytes**: values MUST be persisted and returned as raw `[]byte`; the component
  MUST NOT assume, parse, or transform the payload as JSON.

**Rationale**: Actor and workflow stores rely on atomicity and ETags for correctness; silent
deviations here cause data loss that is invisible until production.

### IV. Idiomatic, Pluggable YDB Integration

The component runs as a standalone pluggable process exposing a Unix Domain Socket — it MUST
NOT require forking or rebuilding `daprd`. It MUST register via the SDK
(`dapr.Register("state.ydb", dapr.WithStateStore(...))`) and ship a `metadata.yaml` manifest
describing every supported metadata field. Persistence MUST use the official YDB Go SDK with a
connection/session pool opened in `Init` from parsed metadata, and SHOULD prefer YDB-native
capabilities (serializable transactions, native row TTL, the query service) over emulating
them in application code. Resources (pools, background GC goroutines) MUST be released cleanly
on `Close`/`Ping` lifecycle.

**Rationale**: The pluggable model keeps the component private and independently deployable;
leaning on YDB's native guarantees is simpler and more correct than re-implementing them.

### V. Observability & Operability

The component MUST emit structured logs for lifecycle and error paths, MUST surface
configuration errors at `Init` time with actionable messages (never panic on bad metadata),
and MUST implement `Ping` for health checking. Configuration MUST come exclusively from the
declared `metadata.yaml` fields — no hidden environment-variable coupling. Failures MUST
degrade predictably: a connection loss returns errors, it does not crash the sidecar.

**Rationale**: A pluggable component is an operational dependency of every app that binds to
it; it must be diagnosable and must fail safe.

## Technology & Architecture Constraints

- **Language**: Go (version pinned in `go.mod`; MUST track a version supported by the current
  `components-go-sdk` and `components-contrib` releases).
- **Core dependencies**: `dapr-sandbox/components-go-sdk` (pluggable host), the YDB Go SDK
  (`ydb-platform/ydb-go-sdk`) for persistence, and `dapr/components-contrib` `state` package
  types for the interface contract.
- **Transport**: gRPC over a Unix Domain Socket in the Dapr components socket directory.
- **Manifest**: a `metadata.yaml` component manifest is REQUIRED and MUST stay in sync with the
  metadata fields actually parsed in `Init`.
- **Schema management**: YDB table creation/migration MUST be explicit and idempotent; the
  component MUST NOT silently depend on a pre-existing schema without documenting it.

## Development Workflow & Quality Gates

- **Definition of done**: a change is done only when the relevant conformance/integration tests
  pass against a real (or containerized) YDB instance and the `metadata.yaml` reflects reality.
- **Review**: every change MUST be reviewed against this constitution; PRs MUST state which
  features are affected and confirm `Features()` still matches behavior.
- **Testing gate**: conformance for advertised features (Principle II) is a merge blocker.
  Integration tests MUST run against a YDB instance, not a mock, for persistence paths.
- **Documentation**: supported metadata fields, advertised features, and required YDB setup
  MUST be documented and updated in the same change that alters them.

## Governance

This constitution supersedes ad-hoc practices for this project. Amendments MUST be made by
editing this file, MUST include an updated Sync Impact Report, and MUST bump the version per
semantic versioning:
- **MAJOR**: removal or backward-incompatible redefinition of a principle or governance rule.
- **MINOR**: a new principle/section or materially expanded mandatory guidance.
- **PATCH**: clarifications and wording fixes that do not change obligations.

All PRs and reviews MUST verify compliance with the principles above. Any deviation MUST be
justified in the plan's Complexity Tracking section with the simpler alternative that was
rejected and why. When this constitution and a downstream template disagree, this constitution
wins and the template MUST be corrected.

**Version**: 1.0.0 | **Ratified**: 2026-06-18 | **Last Amended**: 2026-06-18
