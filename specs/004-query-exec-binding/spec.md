# Feature Specification: YDB Query/Exec Output Binding

**Feature Branch**: `004-query-exec-binding`

**Created**: 2026-06-18

**Status**: Draft

**Input**: User description: "Implement query and exec operations for YDB, like the dapr/components-contrib postgres output binding (bindings/postgres) which defines `query` and `exec` operations over raw SQL."

## Overview

Today this project ships a single Dapr component: a `state.ydb` state store (Get/Set/Delete).
Application developers who want to run arbitrary YQL against the same YDB database — to read
rows, run analytics, or perform DDL/writes that don't fit the key/value model — have no
supported path.

This feature adds a **second component**: a YDB output binding (`bindings.ydb`) that exposes
`query` and `exec` operations, mirroring the contract of the reference `bindings/postgres`
output binding. A developer addresses the binding from any Dapr-supported language via the
standard binding `Invoke` API; the component runs the supplied YQL statement against YDB and
returns rows (for `query`) or an execution summary (for `exec`). Both components are served by
the same plugin binary over the existing pluggable-component socket — no new binary, and no
change to the existing state store.

## Clarifications

### Session 2026-06-18

- Q: Which parameter model should `query`/`exec` expose — postgres-compatible positional, named+typed, or both? → A: Both. A positional `params` JSON array (bound via the YDB `database/sql` driver's positional-args + auto-declare support) covers the common case for drop-in postgres familiarity; an optional named+typed form lets callers force exact YDB types (e.g. `Int64`/`Uint64`/`Timestamp`) that JSON value inference cannot disambiguate.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Run a read query and get rows back (Priority: P1)

An application developer needs to read data from YDB that does not fit the key/value access
pattern of the state store — e.g., selecting a set of rows by a non-key column, or running a
small aggregate. They send a `query` operation to the binding with a YQL `SELECT` statement and
receive the result rows as structured JSON they can parse in their own code.

**Why this priority**: Reading data is the most common and lowest-risk use of a SQL binding and
delivers immediate value on its own. A binding that can only run queries is already a viable MVP.

**Independent Test**: Issue a `query` operation with a `SELECT` over a known seeded table and
confirm the returned data is a JSON array whose rows and columns match the table contents.

**Acceptance Scenarios**:

1. **Given** a table containing known rows, **When** the developer invokes the binding with a
   `query` operation and a valid `SELECT` statement, **Then** the response data is a JSON array
   of row objects (one object per row, keyed by column name) reflecting the matching rows.
2. **Given** a `SELECT` that matches no rows, **When** the developer invokes a `query` operation,
   **Then** the response data is an empty JSON array and the operation reports success.
3. **Given** a `query` whose statement is not valid YQL, **When** invoked, **Then** the operation
   fails with an error message that conveys the database's reason and does not crash the component.

---

### User Story 2 - Execute a write or DDL statement (Priority: P2)

An application developer needs to mutate data or change schema in YDB outside the key/value model
— e.g., `INSERT`/`UPDATE`/`DELETE` over a non-state table, or `CREATE TABLE`/`DROP TABLE` during
setup. They send an `exec` operation with the YQL statement and receive confirmation that it ran,
including an execution summary.

**Why this priority**: Writes and DDL are valuable but higher-risk than reads and depend on the
same machinery proven by P1. Sequencing it second keeps the MVP small.

**Independent Test**: Issue an `exec` with an `INSERT`, then issue a `query` confirming the row is
present; issue an `exec` with `CREATE TABLE` / `DROP TABLE` and confirm success metadata is
returned.

**Acceptance Scenarios**:

1. **Given** a valid write statement, **When** the developer invokes an `exec` operation, **Then**
   the change is durably applied to YDB and the operation reports success.
2. **Given** a valid DDL statement (e.g., create/drop table), **When** invoked as `exec`, **Then**
   the schema change is applied and the operation reports success.
3. **Given** an `exec` statement that the database rejects, **When** invoked, **Then** the
   operation fails with a descriptive error and no partial state is left behind for that statement.

---

### User Story 3 - Pass parameters safely (Priority: P3)

An application developer needs to run a statement whose values come from untrusted or dynamic
input (e.g., a user-supplied id). They supply the values as named parameters alongside the
statement rather than concatenating them into the YQL text, so the binding binds them safely and
avoids injection.

**Why this priority**: Parameterization is important for security and correctness but builds
directly on P1/P2; the binding is demonstrable without it, so it is sequenced last.

**Independent Test**: Issue a parameterized `query` (and `exec`) supplying parameter values
separately from the statement text, and confirm the bound values produce the same result as the
equivalent literal statement, while a value containing YQL syntax is treated as data, not code.

**Acceptance Scenarios**:

1. **Given** a statement that references named parameters and a set of supplied parameter values,
   **When** invoked, **Then** the values are bound to the statement and the result matches the
   equivalent literal statement.
2. **Given** a parameter value that contains characters resembling YQL syntax, **When** invoked,
   **Then** the value is treated strictly as data and cannot alter the statement's structure.
3. **Given** a statement that references a parameter for which no value (or no resolvable type) was
   supplied, **When** invoked, **Then** the operation fails with an error naming the offending
   parameter rather than executing an ill-formed statement.

---

### Edge Cases

- **Unknown operation**: An `Invoke` with an operation other than the advertised ones (e.g.,
  `get`) fails with a clear "operation not supported" error listing the supported operations.
- **Missing statement**: An `Invoke` with no statement supplied fails with a descriptive error and
  does not reach the database.
- **Empty result set**: A `query` matching zero rows returns an empty array, not an error or null.
- **Large result set**: A `query` returning many rows returns them as a single JSON payload; the
  spec assumes result sizes that fit a single binding response (very large/streaming exports are
  out of scope — see Assumptions).
- **NULL and non-string column values**: Rows containing NULLs and non-string types (numbers,
  booleans, timestamps, binary) are represented in JSON in a documented, lossless-as-possible way.
- **Connection/availability failure**: If YDB is unreachable when an operation runs, the operation
  fails with a descriptive error and the component remains usable for later requests.
- **Statement that returns rows sent as `exec`** (or a non-returning statement sent as `query`):
  the operation behaves predictably and documents which result fields are populated.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST provide a YDB output binding component, served by the existing
  plugin binary alongside the current state store, addressable from Dapr component manifests.
- **FR-002**: The binding MUST advertise its supported operations such that the runtime reports
  `query` and `exec` as available operations.
- **FR-003**: For a `query` operation, the binding MUST execute the supplied YQL statement against
  the configured YDB database and return the result rows as a JSON array of objects keyed by column
  name.
- **FR-004**: For an `exec` operation, the binding MUST execute the supplied YQL statement
  (including data-modification and DDL statements) against the configured YDB database and report
  success or a descriptive failure.
- **FR-005**: The binding MUST accept the statement text via the operation's request metadata using
  a documented field name.
- **FR-006**: The binding MUST support binding caller-supplied parameter values to the statement
  separately from the statement text, so that values are never interpreted as statement syntax. It
  MUST accept parameters in **two** forms: (a) a **positional** list bound to placeholders in the
  statement, with types inferred from the supplied values, for postgres-binding familiarity; and
  (b) an optional **named, explicitly-typed** form that lets the caller force an exact YDB type for
  a parameter. A single invocation uses one form or the other, not both.
- **FR-006a**: When the positional form is used, the binding MUST infer each parameter's YDB type
  from its supplied value, and MUST document the inference rules (including that all JSON numbers
  infer to a single numeric type) so callers know when to switch to the named+typed form.
- **FR-007**: When a parameter value cannot be bound (missing value or undeterminable type for a
  referenced parameter), the binding MUST fail with an error identifying the parameter and MUST NOT
  execute the statement.
- **FR-008**: On any database or input error, the binding MUST return a descriptive error and MUST
  NOT crash, panic, or take down the host plugin process or the co-resident state store.
- **FR-009**: For each operation the binding MUST return response metadata describing the outcome,
  including at minimum the operation performed and the statement executed; `query` responses carry
  the result rows and `exec` responses carry an execution summary.
- **FR-010**: A `query` matching zero rows MUST return an empty result set reported as success, not
  an error.
- **FR-011**: The binding MUST reject any operation it does not support with a clear error that
  enumerates the supported operations, and MUST reject requests missing a required statement.
- **FR-012**: The binding MUST reuse the project's existing YDB connection configuration and
  authentication options (the same manifest fields the state store accepts), so operators configure
  credentials the same way for both components.
- **FR-013**: The binding's documented behavior (operations, request fields, parameter model,
  response shape) MUST be captured in the component's metadata/documentation so operators and app
  developers can use it without reading source.
- **FR-014**: The binding's advertised behavior MUST be verified by an automated test that
  exercises `query`, `exec`, and parameterized statements against a real YDB instance before the
  feature is considered done.

### Key Entities *(include if feature involves data)*

- **Binding invocation request**: The unit of work a caller sends — carries the operation kind
  (`query` or `exec`), the YQL statement text, and an optional set of named parameter values.
- **Parameter set**: The caller-supplied values bound into the statement — either an ordered
  positional list (types inferred per value) or a named map where each entry carries an explicit
  YDB type and value.
- **Query result**: The rows returned by a `query`, represented as an ordered collection of row
  objects keyed by column name, serialized as JSON for the response payload.
- **Execution summary**: The outcome metadata returned for an `exec` (and alongside `query`),
  describing the operation performed and the statement executed.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A developer can retrieve rows from YDB through the binding using only a statement and
  the standard binding invocation API, with zero changes to the existing state store configuration.
- **SC-002**: 100% of the advertised operations (`query`, `exec`) and the parameterized-statement
  path are exercised by an automated test against a real YDB instance and pass before release.
- **SC-003**: A query returning a result set is returned to the caller as parseable JSON in which
  every row and column from the matching data is present and correctly labeled by column name.
- **SC-004**: Every failure mode in the Edge Cases section (unknown operation, missing statement,
  database error, unreachable database) returns a descriptive error and leaves the plugin process —
  including the co-resident state store — fully operational for subsequent requests.
- **SC-005**: A parameter value containing YQL-like syntax cannot alter the structure of an executed
  statement (verified by a test that would only pass if the value is treated as data).
- **SC-006**: An operator can configure and use the binding using only the published component
  metadata/documentation, reusing the same credential fields as the state store.

## Assumptions

- **Component type**: The right vehicle for raw-YQL `query`/`exec` is a Dapr **output binding**
  (matching the referenced postgres binding), not an extension of the `state.Store`. The existing
  state store is unchanged; the binding is added as a second component in the same plugin binary.
- **Statement language**: Statements are YQL (YDB's SQL dialect). The binding does not translate or
  validate SQL beyond passing it to YDB; invalid statements surface the database's own error.
- **Parameter model** (resolved in Clarifications 2026-06-18): the binding exposes **both** a
  postgres-compatible **positional** `params` array (types inferred from the supplied values) and
  an optional **named+typed** form for exact YDB types. The underlying YDB `database/sql` driver
  natively supports positional placeholders with automatic type declaration, so the positional form
  is genuinely postgres-like; the named+typed form is the escape hatch when JSON value inference
  cannot pick the intended numeric/temporal type. The exact request encoding for each form is a
  design detail for the plan.
- **Execution-summary fidelity**: An exact "rows-affected" count comparable to the postgres binding
  is not guaranteed, because YDB does not surface affected-row counts the same way. The `exec`
  response reports success and the statement executed; any affected-row count is best-effort and
  documented as such.
- **Result size**: Result sets are assumed to fit within a single binding response payload.
  Streaming or paginating very large exports is out of scope for this feature.
- **Operation set**: Only `query` and `exec` are in scope. The postgres binding's `close` operation
  is out of scope; connection lifecycle is managed by the component, not by callers.
- **Configuration reuse**: The binding consumes the same connection-string and authentication
  manifest fields the state store already defines (including the Yandex Cloud auth methods), so no
  new credential model is introduced.
- **Verification path**: Dapr's binding conformance suite and/or an integration test against a real
  YDB instance is the authoritative correctness gate for this component, consistent with the
  project's conformance-first principle for the state store.
- **No state-store impact**: This feature does not change the state store's behavior, advertised
  features, or schema.
