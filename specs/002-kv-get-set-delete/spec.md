# Feature Specification: KV Get/Set/Delete with conformance-gated ETag

**Feature Branch**: `002-kv-get-set-delete`

**Created**: 2026-06-18

**Status**: Draft

**Input**: User description: "a new feature for Get/Set/Delete against the documented KV schema (data-model.md), then advertise FeatureETag only after it passes conformance."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Basic key/value round-trip (Priority: P1)

As an application using the YDB-backed state store, I can save a value under a key, read the exact
same value back later, and delete it when no longer needed — so my app has durable, working state.

**Why this priority**: This is the core value of a state store. Without a working
save → read → delete round-trip there is no usable persistence; every other capability builds on it.
This is the MVP slice that turns the scaffold into a functioning store.

**Independent Test**: Configure the store against a YDB instance, save a value under a fresh key,
read it back and confirm it is byte-identical, then delete it and confirm a subsequent read returns
"not found". Fully exercisable through the standard state-store interface without any other story.

**Acceptance Scenarios**:

1. **Given** an initialized store and a key that does not exist, **When** the app saves a value under
   that key, **Then** the operation succeeds and the value is durably stored.
2. **Given** a key that holds a value, **When** the app reads that key, **Then** it receives the exact
   bytes that were saved, unmodified.
3. **Given** a key that holds a value, **When** the app reads it and the stored bytes are arbitrary
   binary (not valid text/JSON), **Then** the value is returned unchanged and is never parsed or
   transformed.
4. **Given** a key that holds a value, **When** the app deletes that key, **Then** the operation
   succeeds and a subsequent read returns an empty/"not found" result rather than an error.
5. **Given** a key that does not exist, **When** the app reads it, **Then** it receives an empty/"not
   found" result (no error) and no value.
6. **Given** a key that does not exist, **When** the app deletes it (with no concurrency token),
   **Then** the operation succeeds (delete is idempotent).
7. **Given** a key that already holds a value, **When** the app saves a new value under the same key
   with no concurrency token, **Then** the previous value is overwritten with the new one.

---

### User Story 2 - Optimistic concurrency via ETag, advertised only after conformance (Priority: P2)

As an application that needs safe concurrent updates, I can pass the version token I last read when
saving or deleting, so that my write only applies if no one else changed the key in the meantime — and
the store only advertises this capability once it has been proven against the official conformance
suite.

**Why this priority**: Optimistic concurrency prevents lost updates, but correctness here is subtle and
must be proven before the store claims the capability. Advertising the capability prematurely would let
the runtime route concurrency-sensitive workloads to an unverified implementation. P2 because the store
is already useful (P1) without it, and this story explicitly gates the advertised feature on passing
conformance.

**Independent Test**: With ETag support implemented but before flipping the advertised capability,
run the conformance suite's ETag scenarios; confirm they pass; then confirm the store reports the ETag
capability. Independently verify that a save/delete carrying a stale token is rejected while one
carrying the current token succeeds.

**Acceptance Scenarios**:

1. **Given** a key holding a value with version token T1, **When** the app saves a new value carrying
   token T1, **Then** the write succeeds and the key now reports a new, different version token T2.
2. **Given** a key whose current version token is T2, **When** the app saves a value carrying the stale
   token T1, **Then** the write is rejected with a concurrency-mismatch error and the stored value is
   unchanged.
3. **Given** a key holding a value with version token T2, **When** the app deletes it carrying token
   T2, **Then** the delete succeeds; **When** instead it carries stale token T1, **Then** the delete is
   rejected with a concurrency-mismatch error.
4. **Given** a save or delete that carries a malformed/uninterpretable version token, **When** it is
   submitted, **Then** it is rejected with a concurrency-mismatch error and the stored value is unchanged.
   (Revised during implementation: the conformance suite treats a bad token as a mismatch — see FR-008.)
5. **Given** the ETag implementation has NOT yet passed the conformance suite, **When** a caller asks
   the store which capabilities it supports, **Then** the ETag capability is NOT listed.
6. **Given** the ETag implementation HAS passed the conformance suite, **When** a caller asks the store
   which capabilities it supports, **Then** the ETag capability IS listed.

---

### Edge Cases

- **Logically-expired rows**: a row whose expiry timestamp is in the past MUST be treated as absent on
  read (return "not found") even if background purge has not yet removed it.
- **Empty value**: saving an empty (zero-length) value is distinct from the key being absent; reading it
  returns an empty value with a valid version token, not "not found".
- **First write generates a token**: a successful save against a previously-absent key returns a valid
  version token so the caller can do conditional updates afterward.
- **Concurrent conflicting writes**: when two writers race on the same key each carrying the same prior
  token, at most one succeeds; the other receives a concurrency-mismatch error.
- **Context cancellation / timeout**: if the caller's deadline expires or the call is cancelled mid-
  operation, the operation aborts and surfaces the cancellation rather than hanging or panicking.
- **Backend unavailable**: if the underlying database is unreachable, operations return an error and the
  process never crashes.
- **Missing table on first use**: the persistence table is created idempotently; concurrent initializers
  do not fail because the table already exists.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The store MUST ensure the documented KV table (columns: key, value, version token,
  expiry) exists before serving operations, creating it idempotently if absent, matching the schema in
  `data-model.md`.
- **FR-002**: The store MUST support a Set operation that durably stores a value under a key, creating
  the key if absent or overwriting it if present (when no concurrency token is supplied).
- **FR-003**: The store MUST support a Get operation that returns the stored value for a key, or an
  empty/"not found" result (without error) when the key is absent.
- **FR-004**: The store MUST support a Delete operation that removes a key; deleting an absent key with
  no concurrency token MUST succeed (idempotent).
- **FR-005**: Values MUST be stored and returned as opaque raw bytes — never parsed, validated, or
  transformed as JSON or any other format.
- **FR-006**: Every successful Set MUST generate a new opaque version token (a fresh unique value per
  write) and return it to the caller; Get MUST return the current version token alongside the value.
- **FR-007**: When a Set or Delete carries a concurrency token, the operation MUST apply only if the
  token matches the key's current token; a mismatch MUST be rejected with a concurrency-mismatch error
  and leave stored data unchanged.
- **FR-008** (revised during implementation): When a Set or Delete carries a concurrency token that does
  not match the stored token — **including a malformed/uninterpretable token** — the operation MUST be
  rejected with a concurrency-mismatch error and leave stored data unchanged. *Rationale*: the Dapr
  state conformance suite supplies a bad token (`"bad-etag"`) and asserts a **mismatch** result, and the
  reference contrib Postgres v2 component (also UUID etags) returns mismatch on parse failure. Because
  conformance is the authoritative gate (constitution Principle II and the feature's own directive), a
  distinct "invalid token" class is not produced. See
  [research.md](research.md) D4. Constitution Principle III was amended to v1.1.0 to match this behavior
  (malformed/non-matching ETag → mismatch), so spec, constitution, and conformance are now consistent.
- **FR-009**: Get MUST NOT return rows whose expiry timestamp has passed; such rows MUST be reported as
  "not found" regardless of whether background purge has removed them yet.
- **FR-010**: The store MUST NOT advertise the ETag (optimistic concurrency) capability until the ETag
  behavior has passed the official Dapr state-store conformance suite.
- **FR-011**: Once the ETag behavior passes conformance, the store MUST advertise the ETag capability in
  its reported feature set; no other capability (transactions, TTL, query) is advertised by this feature.
- **FR-012**: All operations MUST fail-fast with descriptive errors and MUST NOT panic; they MUST honor
  caller context cancellation and deadlines.
- **FR-013**: The store MUST behave correctly under concurrent conflicting writes to the same key such
  that at most one write carrying a given prior token succeeds.

### Key Entities *(include if feature involves data)*

- **Key/Value Record**: a single stored entry identified by its key. Attributes: the key (unique
  identifier), the value (opaque bytes), a version token (opaque optimistic-concurrency marker that
  changes on every successful write), and an expiry anchor (optional; a past value means the record is
  logically absent). Defined in `data-model.md`.
- **Concurrency Token (ETag)**: an opaque marker a caller reads with a value and may present on a later
  write to assert "only apply if unchanged since I read it." Not interpreted by the caller.
- **Advertised Capability Set**: the list of capabilities the store reports to the runtime; ETag is
  added to this list only after conformance passes.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A value saved via Set is read back byte-identical via Get in 100% of round-trip attempts,
  including arbitrary binary payloads.
- **SC-002**: The Dapr state-store conformance suite passes for all basic CRUD scenarios (save, get,
  delete, get-absent) with the ETag scenarios excluded while the ETag capability is unadvertised.
- **SC-003**: After the ETag capability is advertised, the conformance suite's ETag/optimistic-
  concurrency scenarios pass with zero failures.
- **SC-004**: In a concurrent test where N writers race on one key each carrying the same prior token,
  exactly one succeeds and the remaining N−1 receive a concurrency-mismatch error, across repeated runs.
- **SC-005**: A read of a logically-expired key returns "not found" in 100% of attempts, even before
  background purge runs.
- **SC-006**: No operation panics or leaks resources under error, cancellation, or backend-unavailable
  conditions, verified across the test suite.
- **SC-007**: The store reports the ETag capability if and only if the conformance ETag scenarios have
  passed — verifiable by inspecting the reported capability set before and after the gate.

## Assumptions

- **TTL/expiry writes are out of scope**: this feature reads and respects an existing expiry value
  (treating past-expiry rows as absent) but does NOT implement caller-supplied time-to-live on Set, and
  does NOT advertise the TTL capability. Expiry is honored defensively for correctness of Get.
- **Transactions and bulk are out of scope**: multi-item transactional writes and the query API are not
  implemented here; bulk operations, if exercised, rely on the framework's default per-item fan-out over
  the single-item Get/Set/Delete defined here. No transactional or query capability is advertised.
- **Empty concurrency token means unconditional**: a Set/Delete with no concurrency token performs an
  unconditional upsert/delete, consistent with standard state-store semantics.
- **Version tokens are opaque and unique-per-write**: tokens are generated server-side as fresh unique
  values (e.g., random UUIDs per the documented schema) and carry no caller-visible meaning.
- **Single configured table**: all keys for this store live in the one configured table from the
  scaffold's store configuration; key namespacing/composition is handled by the runtime upstream.
- **Conformance is the gate of record**: "passes conformance" means the official Dapr state-store
  conformance suite for the relevant scenarios reports zero failures; that result is the authoritative
  trigger for advertising the ETag capability.

## Dependencies

- Builds on the `001-project-scaffold` store configuration and connection lifecycle (Init/Close).
- Persists against the KV schema documented in `specs/001-project-scaffold/data-model.md`.
- Verification depends on the official Dapr state-store conformance suite.
