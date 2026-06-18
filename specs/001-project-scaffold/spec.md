# Feature Specification: Project Scaffold

**Feature Branch**: `001-project-scaffold`

**Created**: 2026-06-18

**Status**: Draft

**Input**: User description: "scaffold the project"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Loadable pluggable component skeleton (Priority: P1)

A platform engineer running Dapr wants to register the YDB state store as a pluggable
component and have the Dapr sidecar discover and connect to it over its Unix socket, so that
the integration is wired end-to-end before any persistence logic is filled in.

**Why this priority**: Nothing else can be demonstrated, tested, or deployed until the Dapr
runtime can actually load the component. This is the thinnest viable slice that proves the
pluggable plumbing works and unblocks every later feature.

**Independent Test**: Build the component, start it so it listens on a Unix Domain Socket in
the components socket directory, register it as `state.ydb`, and confirm the Dapr sidecar
discovers it and reports it healthy — without requiring real Get/Set persistence yet.

**Acceptance Scenarios**:

1. **Given** the component binary is built, **When** it is started, **Then** it exposes the
   state store service on a Unix Domain Socket in the expected socket directory and reports
   ready.
2. **Given** the component is running, **When** the Dapr sidecar starts with a component
   manifest naming `state.ydb`, **Then** the sidecar loads the component and its health/ping
   check succeeds without error.
3. **Given** the component is running, **When** a health/ping is issued, **Then** it responds
   successfully.

---

### User Story 2 - Declared, validated configuration (Priority: P2)

An operator wants to configure the store entirely through a declared component manifest
(connection settings and store options), and to be told immediately and clearly when a
configuration value is missing or invalid, so misconfiguration is caught at startup rather
than at first request.

**Why this priority**: A loadable skeleton is only usable once it can be pointed at a real YDB
target through declared configuration. Clear startup validation is the difference between a
diagnosable component and a silent failure.

**Independent Test**: Provide a component manifest with the documented fields and confirm
initialization succeeds; provide a manifest with a missing/invalid required field and confirm
initialization fails with an actionable error message that names the offending field.

**Acceptance Scenarios**:

1. **Given** a manifest with all required configuration fields, **When** the component
   initializes, **Then** initialization succeeds and the declared fields are applied.
2. **Given** a manifest missing a required field, **When** the component initializes, **Then**
   initialization fails with a clear message identifying the missing field, and the component
   does not crash the sidecar.
3. **Given** a manifest with an invalid field value, **When** the component initializes,
   **Then** initialization fails with a message explaining what was expected.
4. **Given** the published manifest, **When** an operator reads it, **Then** every field the
   component actually reads is documented there.

---

### User Story 3 - Repeatable build, test, and conformance harness (Priority: P3)

A contributor wants a one-command way to build, lint, and test the component, plus a wired-up
harness for running Dapr's state conformance suite, so that correctness can be verified
consistently from the very first commit and the project's quality gate is enforceable.

**Why this priority**: The constitution makes conformance the non-negotiable correctness gate.
Establishing the harness during scaffolding ensures every later feature can be validated the
moment it is written, rather than retrofitting test infrastructure later.

**Independent Test**: Run the project's build, lint, and test commands from a clean checkout
and confirm they succeed; confirm the conformance harness compiles and can be pointed at a YDB
target, even if only stub operations are exercised initially.

**Acceptance Scenarios**:

1. **Given** a clean checkout, **When** the contributor runs the documented build command,
   **Then** the component builds successfully.
2. **Given** a clean checkout, **When** the contributor runs the documented lint and test
   commands, **Then** they execute and report results.
3. **Given** the conformance harness, **When** it is invoked against a reachable YDB target,
   **Then** it compiles and runs the suite for the features the component advertises.

---

### Edge Cases

- What happens when the configured socket directory is missing or not writable? The component
  must fail to start with a clear error rather than appearing healthy.
- What happens when two component instances attempt to use the same socket? Startup must fail
  predictably rather than silently shadowing one another.
- What happens when the manifest declares a feature the skeleton does not yet honor? The
  advertised feature set must reflect only what is actually implemented (per constitution
  Principle I), so the skeleton advertises no unimplemented features.
- What happens when the YDB target is unreachable at startup? Behavior must be predictable and
  diagnosable (clear error or documented retry behavior), never a sidecar crash.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The project MUST be a buildable component that produces a runnable artifact from
  a clean checkout using a single documented command.
- **FR-002**: The component MUST expose the Dapr state store service over a Unix Domain Socket
  in the components socket directory so the Dapr sidecar can discover it as a pluggable
  component without rebuilding the sidecar.
- **FR-003**: The component MUST register itself under the name `state.ydb`.
- **FR-004**: The component MUST respond successfully to a health/ping check while running.
- **FR-005**: The component MUST accept configuration exclusively through a declared component
  manifest and MUST NOT depend on hidden/undeclared configuration sources.
- **FR-006**: The component MUST validate configuration at initialization, failing with an
  actionable, field-specific error message on missing or invalid required values, without
  crashing the host sidecar.
- **FR-007**: The project MUST publish a configuration manifest that documents every field the
  component reads, kept in sync with what initialization actually parses.
- **FR-008**: The component MUST advertise only the capabilities it actually implements; the
  initial scaffold MUST NOT advertise any feature it does not yet honor.
- **FR-009**: The project MUST provide documented commands to build, lint, and run tests.
- **FR-010**: The project MUST include a harness wired to Dapr's state conformance suite that
  compiles and can be run against a YDB target.
- **FR-011**: The project MUST include a top-level README documenting prerequisites, how to
  build and run the component, how to configure it, and how to run the test/conformance
  harness.
- **FR-012**: The project structure MUST cleanly separate the component implementation,
  configuration manifest, and tests so later features can be added without restructuring.
- **FR-013**: The component MUST release its resources cleanly on shutdown (no orphaned
  sockets or background work) so it can be restarted reliably.

### Key Entities *(include if feature involves data)*

- **Pluggable Component Process**: The standalone process that hosts the state store service,
  owns the Unix socket lifecycle, and is loaded by the Dapr sidecar.
- **Configuration Manifest**: The declared description of the component's name, type, and
  supported configuration fields; the single source of truth for what operators may set.
- **Store Configuration**: The set of values (connection target, credentials/endpoint
  settings, store options) parsed and validated at initialization.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A contributor can go from a clean checkout to a built, running component using
  documented commands in under 15 minutes.
- **SC-002**: The Dapr sidecar successfully loads the component and passes its health check on
  the first attempt with a correct manifest, with zero manual post-build steps beyond the
  documented ones.
- **SC-003**: 100% of configuration fields the component reads are documented in the published
  manifest (no undocumented fields).
- **SC-004**: Every required-field misconfiguration produces an error message that names the
  offending field, verified for each required field.
- **SC-005**: The build, lint, and test commands succeed from a clean checkout on a supported
  environment, and the conformance harness compiles, in 100% of runs on a clean environment.
- **SC-006**: A new contributor can identify where to add a new state operation (e.g., a new
  store behavior) from the project structure and README without external guidance.

## Assumptions

- The component is delivered as a **pluggable** Dapr component (gRPC over a Unix Domain
  Socket), per the project constitution — not a built-in component requiring a `daprd` rebuild.
- The implementation language and core dependencies follow the constitution (Go, the Dapr
  pluggable component SDK, the YDB client SDK, and Dapr's state contract types).
- This scaffold establishes the foundation and plumbing only; full persistence semantics
  (Get/Set/Delete, bulk, ETags, TTL, transactions, query) are delivered by subsequent features
  and are intentionally out of scope here beyond stubs sufficient to load and health-check.
- The Go module path / repository identity will follow the project's repository location;
  the exact module path is a configuration detail that can be set during planning.
- A YDB instance (containerized or remote) is available to contributors for running the
  conformance/integration harness; provisioning that instance is out of scope for this feature.
- The component is registered as `state.ydb`; the human-facing component name in manifests may
  differ and is set per deployment.
