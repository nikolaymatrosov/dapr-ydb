# Contract: Authentication-Method → Driver-Option Mapping

This is the external configuration contract the component exposes to operators via the component
manifest. It is the authoritative reference for which `authMethod` values are supported, what each
requires, and how each maps to a YDB driver option. It MUST stay in sync with `metadata.yaml`,
[internal/ydbstate/metadata.go](../../../internal/ydbstate/metadata.go), and the `credentialOptions()`
mapping in [internal/ydbstate/store.go](../../../internal/ydbstate/store.go).

## Manifest fields (auth-relevant)

| Field | Required when | Sensitive | Notes |
|-------|---------------|-----------|-------|
| `authMethod` | optional (default `anonymous`) | no | One of the five values below. |
| `username` | `authMethod=static` | no | — |
| `password` | `authMethod=static` | yes | — |
| `accessToken` | `authMethod=token` | yes | Static IAM/access token (not auto-refreshed). |
| `serviceAccountKeyPath` | `authMethod=serviceAccountKey` | no (path) | File at the path is sensitive; never logged. |
| `useInternalCA` | optional (default `false`) | no | Composable with **every** auth method. |

## Supported `authMethod` values

| Value | Requires | Driver option | Credential refresh | Typical context |
|-------|----------|---------------|--------------------|-----------------|
| `anonymous` | — | `ydb.WithAnonymousCredentials()` | n/a | Local YDB / tests |
| `static` | username, password | `ydb.WithStaticCredentials(u, p)` | n/a | Self-hosted YDB |
| `token` | accessToken | `ydb.WithAccessTokenCredentials(t)` | none (caller-managed) | Short-lived/manual |
| `serviceAccountKey` | serviceAccountKeyPath | `yc.WithServiceAccountKeyFileCredentials(path)` | **automatic** | Yandex Cloud prod (off-cloud or on-cloud) |
| `metadata` | — | `yc.WithMetadataCredentials()` | **automatic** | Yandex Cloud prod (on-cloud workload) |

Plus, for any of the above: if `useInternalCA=true`, append `yc.WithInternalCA()`.

## Behavioral guarantees

1. **No "not yet supported".** `serviceAccountKey` and `metadata` MUST produce a working connection
   when correctly configured; neither returns a "not yet supported" / "later feature" error. (FR-003)
2. **Field-named config errors at Init.** Missing `serviceAccountKeyPath`, or a path that cannot be
   read, MUST fail `Init` with an error naming `serviceAccountKeyPath` — before any network call.
   (FR-005)
3. **Distinguishable failure tiers.** A missing/unreadable key (config) is reported distinctly and
   earlier than a reachable-but-rejecting credential source or an unreachable metadata service
   (connect-time `failed to open YDB connection: …`). (FR-006)
4. **Secret-less metadata path.** `authMethod=metadata` MUST require no secret or key material in the
   manifest. (FR-002)
5. **Automatic refresh.** For `serviceAccountKey` and `metadata`, credentials MUST refresh
   automatically; a session outliving a single token's lifetime keeps working without restart. (FR-001,
   FR-002, SC-005)
6. **Internal-CA composability.** `useInternalCA=true` MUST work with any `authMethod` and MUST be
   honored (it was previously parsed but ignored). (FR-007)
7. **Unchanged legacy methods.** `anonymous`, `static`, `token`, and unknown-method rejection behave
   exactly as before. (FR-004, FR-010)
8. **No secrets in logs.** Key file contents and derived tokens MUST never appear in logs or errors.
9. **No new advertised feature.** `Features()` is unchanged; auth is connection config, not a
   `state.Feature`.

## Error-message contract (illustrative)

| Condition | Tier | Message shape |
|-----------|------|---------------|
| `serviceAccountKey` + empty path | Init (existing) | `metadata field 'serviceAccountKeyPath' is required when authMethod=serviceAccountKey` |
| `serviceAccountKey` + unreadable file | Init (new pre-flight) | `metadata field 'serviceAccountKeyPath': cannot read key file %q: %w` |
| `serviceAccountKey` + malformed/rejected key | Connect | `failed to open YDB connection: %w` |
| `metadata` + metadata service unreachable | Connect | `failed to open YDB connection: %w` |
| unknown `authMethod` | Init (existing) | `invalid metadata field 'authMethod': %q (expected one of: anonymous, static, token, serviceAccountKey, metadata)` |
