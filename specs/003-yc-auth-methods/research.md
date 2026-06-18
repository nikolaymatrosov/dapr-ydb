# Phase 0 Research: Yandex Cloud Production Authentication Methods

All unknowns below are resolved; none remain marked NEEDS CLARIFICATION. Decisions build on the
existing store (`001-project-scaffold`, `002-kv-get-set-delete`) and constitution v1.1.0.

## D1 — Which dependency supplies the YC credential options

**Decision**: Add a single module, `github.com/ydb-platform/ydb-go-yc`. Import it as
`yc "github.com/ydb-platform/ydb-go-yc"` and use:

- `yc.WithServiceAccountKeyFileCredentials(path string, opts ...) ydb.Option` — for `serviceAccountKey`
- `yc.WithMetadataCredentials(opts ...) ydb.Option` — for `metadata`
- `yc.WithInternalCA() ydb.Option` — for `useInternalCA`

**Rationale**: `ydb-go-yc` re-exports the metadata-service and internal-CA helpers (it depends on
`ydb-go-yc-metadata`), so one dependency covers all three code paths. Both packages were confirmed on
pkg.go.dev to expose exactly these signatures. Using the official helper module satisfies Principle IV
(prefer SDK-native capabilities over re-implementing IAM token exchange).

**Alternatives considered**:
- Add both `ydb-go-yc` and `ydb-go-yc-metadata` — unnecessary; `ydb-go-yc` already provides
  `WithMetadataCredentials`. Rejected to keep the dependency surface minimal.
- Hand-roll IAM JWT signing + token exchange against the IAM endpoint — large, security-sensitive,
  and exactly what the SDK module exists to do. Rejected (Principle IV).

## D2 — How `serviceAccountKey` maps to a driver option, and error timing

**Decision**: Map `authMethod=serviceAccountKey` to
`yc.WithServiceAccountKeyFileCredentials(s.md.ServiceAccountKeyPath)`. Before returning that option,
**pre-flight** the file with `os.ReadFile(path)`; on failure return a field-named error
(`metadata field 'serviceAccountKeyPath': cannot read key file %q: %w`).

**Rationale**: The YC credential option is evaluated lazily — the file is read and the key parsed only
on the first token request during `ydb.Open`/first RPC. Without a pre-flight, a missing or unreadable
file surfaces as a generic "failed to open YDB connection" wrap, which does not name the responsible
field (fails FR-005, SC-003). A cheap `os.ReadFile` pre-flight gives a clear, field-named error at
`Init` for the missing/unreadable case and is unit-testable without a live cloud. Malformed-JSON and
server-rejection errors still surface at connect time (see D3) — acceptable, because they are
distinguishable from a missing-config error.

**Alternatives considered**:
- Read the file ourselves and pass contents via `yc.WithServiceAccountKeyCredentials(string)` —
  duplicates the SDK's JSON parsing for no benefit; still lazy for the auth step. Rejected.
- No pre-flight, rely on the connection error — fails the "names the responsible field" requirement.
  Rejected.

## D3 — Distinguishing config errors from availability/auth errors

**Decision**: Two error tiers.
- **Config-time (Init, pre-connect)**: missing required field and unreadable key file return
  field-named errors from `parseAndValidateMetadata` / `credentialOptions` (already the pattern for
  `static`/`token`). These are returned *before* `ydb.Open`.
- **Connect-time**: a rejected key, an unreachable metadata service, or a malformed key JSON surface
  from `ydb.Open(...)` and are wrapped once as `failed to open YDB connection: %w`. The underlying YC
  SDK error text identifies the credential source.

**Rationale**: Satisfies FR-006 / SC-003 — a missing path is reported distinctly (and earlier) from a
source that is reachable-but-rejecting. The existing `Init` already wraps `ydb.Open` failures, so no new
wrapping site is needed for the connect-time tier.

**Note**: `metadata` has no config to validate (no secret, no path — FR-002), so its only failure mode
is connect-time (metadata service unreachable / unauthorized), reported via the `ydb.Open` wrap.

## D4 — Activating the latent `useInternalCA` flag

**Decision**: `useInternalCA` is parsed into `storeMetadata.UseInternalCA` today but **never applied**
to the driver. Honor it: when `m.UseInternalCA` is true, append `yc.WithInternalCA()` to the option
slice for **any** auth method (build the base credential option from the `switch`, then conditionally
append). This makes `useInternalCA` composable with every method (FR-007).

**Rationale**: Fixes a latent operability bug (a documented flag silently ignored) and is the
prerequisite for trusting managed-YDB endpoints whose certs chain to the YC internal CA. Keeping it
orthogonal to the auth `switch` matches the spec ("usable together with any auth method").

**Alternatives considered**: Apply internal-CA only for the two new methods — arbitrary; a user on
`token` auth against a managed endpoint needs it too. Rejected.

## D5 — Test & conformance strategy (Constitution Principle II)

**Decision**:
- **Unit tests** (`store_test.go`) assert `credentialOptions()` behavior without a network: each method
  returns a non-empty option slice and `nil` error (anonymous/static/token/metadata); `serviceAccountKey`
  with a missing path returns a field-named error and with a readable temp file returns options;
  `useInternalCA=true` yields exactly one more option than the same config with it false.
- **Existing conformance** (anonymous auth) is unchanged and still gates CRUD/ETag.
- **Live paths** (`serviceAccountKey`, `metadata` against real Yandex Cloud) are validated manually per
  `quickstart.md`; they cannot run in CI without cloud credentials and are not `state.Feature`s.

**Rationale**: Principle II governs *advertised features*. This feature advertises none (auth is
connection config), so the conformance contract is unaffected. The new paths get the strongest
automatable coverage (option-mapping + pre-flight unit tests) plus a documented real-cloud check. The
option count is a deliberate, stable proxy because the YC option values are opaque closures that cannot
be introspected by identity.

**Alternatives considered**: Stand up a YC emulator in CI — none exists for SA-key/metadata IAM flows.
Rejected as infeasible.

## D6 — Manifest (`metadata.yaml`) reconciliation

**Decision**: Remove the "not yet wired / later feature" caveats from the `authMethod` and
`serviceAccountKeyPath` descriptions; keep the `allowedValues` list intact. `useInternalCA` already has
an accurate description.

**Rationale**: FR-009 / SC-006 require published metadata to match runtime behavior. The manifest must
not advertise a method as unsupported once it works (and the constitution requires the manifest to track
reality in the same change).
