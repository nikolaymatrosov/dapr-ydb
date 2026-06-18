# Feature Specification: Yandex Cloud Production Authentication Methods

**Feature Branch**: `003-yc-auth-methods`

**Created**: 2026-06-18

**Status**: Draft

**Input**: User description: "Auth: only anonymous/static/token wired; serviceAccountKey/metadata (Yandex Cloud prod paths) still return 'not yet supported.' Support missing"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Connect using a service account key (Priority: P1)

An operator deploying the state store against managed YDB in Yandex Cloud configures the component to authenticate with a service account key file. When the component starts, it reads the key, obtains short-lived access tokens automatically, and serves state operations without any further manual token handling.

**Why this priority**: This is the primary, recommended way to run the component in production against managed YDB. Without it, the component cannot be used in its main target environment, so it delivers the largest share of the feature's value on its own.

**Independent Test**: Configure the component with `authMethod=serviceAccountKey` and a valid key file path, point it at a managed YDB endpoint, and confirm that state Get/Set/Delete operations succeed. Configure it with a missing or malformed key file and confirm the component fails to start with a clear, field-named error.

**Acceptance Scenarios**:

1. **Given** a valid service account key file at the configured path and a reachable managed YDB endpoint, **When** the component initializes, **Then** it connects successfully and state operations succeed.
2. **Given** `authMethod=serviceAccountKey` with no key file path configured, **When** the component initializes, **Then** startup fails with an error naming the missing required field.
3. **Given** a configured key file path that does not exist or cannot be parsed, **When** the component initializes, **Then** startup fails with an error that identifies the key file as the cause.
4. **Given** a valid key file but credentials that the server rejects, **When** the component initializes, **Then** startup fails with an error that distinguishes an authentication/authorization failure from a configuration error.

---

### User Story 2 - Connect using the instance metadata service (Priority: P2)

An operator running the component on a Yandex Cloud virtual machine (or other workload with an attached service account) configures it to authenticate via the instance metadata service. The component obtains and refreshes credentials from the local metadata endpoint with no secrets stored in the component configuration.

**Why this priority**: This is the secret-less deployment path for workloads running inside Yandex Cloud. It is highly valuable for production hardening but applies to a narrower set of deployments than the key-file path, and depends on the runtime environment providing the metadata service.

**Independent Test**: On a host where the metadata service is reachable and bound to a service account, configure the component with `authMethod=metadata` and confirm state operations succeed. On a host where the metadata service is not reachable, confirm the component reports a clear failure rather than the previous "not yet supported" message.

**Acceptance Scenarios**:

1. **Given** `authMethod=metadata` and a reachable metadata service bound to an authorized service account, **When** the component initializes, **Then** it connects successfully and state operations succeed.
2. **Given** `authMethod=metadata` and no reachable metadata service, **When** the component initializes, **Then** startup fails with an error indicating the metadata credential source was unavailable.
3. **Given** `authMethod=metadata`, **When** the operator configures the component, **Then** no service-account secret or key file is required in the configuration.

---

### User Story 3 - Trust the managed-endpoint certificate authority (Priority: P3)

An operator connecting to a managed YDB endpoint that presents a certificate issued by the cloud provider's internal certificate authority enables the internal-CA option so the secure connection is trusted without distributing custom CA bundles.

**Why this priority**: Managed YDB endpoints commonly use an internal CA, so this is frequently needed alongside the production auth methods. It is a supporting capability rather than an authentication method in its own right, hence the lower priority.

**Independent Test**: Enable the internal-CA option against an endpoint that uses the provider's internal CA and confirm the secure connection is established; disable it and confirm the connection is rejected as untrusted.

**Acceptance Scenarios**:

1. **Given** an endpoint whose certificate chains to the provider's internal CA and the internal-CA option enabled, **When** the component connects, **Then** the secure connection is trusted and established.
2. **Given** the internal-CA option enabled together with either production auth method, **When** the component connects, **Then** the option and the auth method work together without conflict.

---

### Edge Cases

- What happens when the service account key file exists but is empty, truncated, or contains a key for a different account than the server expects?
- How does the component behave when the metadata service is reachable but returns an error or an expired/short-lived response?
- What happens when credentials expire during a long-running session — are tokens refreshed automatically, or does the operator have to restart the component?
- How does the component handle the case where the configured auth method requires a secure connection but the connection string requests an insecure one?
- What is reported when both a key file path and metadata are plausibly available but the configured method's source fails?
- How does the component behave if the key file path points to a file the process has no permission to read?

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The component MUST authenticate to YDB using a service account key when `authMethod=serviceAccountKey` is configured, obtaining and refreshing access credentials automatically for the lifetime of the component.
- **FR-002**: The component MUST authenticate to YDB using the instance metadata service when `authMethod=metadata` is configured, without requiring any secret or key material in its configuration.
- **FR-003**: The component MUST NOT return a "not yet supported" error for either `serviceAccountKey` or `metadata` once this feature is delivered; these methods MUST be fully functional connection paths.
- **FR-004**: The component MUST continue to support the existing `anonymous`, `static`, and `token` auth methods with unchanged behavior.
- **FR-005**: When `authMethod=serviceAccountKey` is configured, the component MUST fail startup with a clear, field-named error if the key file path is missing, unreadable, or cannot be parsed.
- **FR-006**: When the configured credential source rejects the credentials or is unreachable, the component MUST fail startup with an error that distinguishes an authentication/availability failure from a missing-configuration error.
- **FR-007**: The component MUST allow the connection to a managed endpoint to trust the cloud provider's internal certificate authority when the internal-CA option is enabled, and this option MUST be usable together with any auth method.
- **FR-008**: All authentication configuration MUST be supplied exclusively through the declared component manifest (no environment-specific side channels), consistent with the project's manifest-only configuration principle.
- **FR-009**: The component MUST document each supported auth method, its required configuration fields, and its intended deployment context, and MUST advertise the now-supported methods accurately in its component metadata.
- **FR-010**: Configuration validation MUST reject an unknown `authMethod` value and report the full set of accepted values, including the two newly supported methods.

### Key Entities *(include if feature involves data)*

- **Authentication Method**: The selected mode by which the component proves its identity to YDB. One of: anonymous, static (username/password), token (static access token), service-account key (key file → auto-refreshed credentials), or instance metadata (metadata service → auto-refreshed credentials). Determines which configuration fields are required.
- **Service Account Key**: A credential file identifying a service account, referenced by a configured file path. Source material from which the component derives short-lived access credentials. Sensitive; never logged.
- **Instance Metadata Source**: The local, environment-provided credential endpoint that supplies and refreshes credentials for a workload's attached service account, requiring no stored secrets.
- **Connection Trust Settings**: The certificate-authority trust configuration (including the internal-CA option) governing whether the secure connection to a managed endpoint is accepted.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator can configure the component to authenticate against managed YDB with a service account key and successfully perform state Get/Set/Delete operations, with zero occurrences of the "not yet supported" message for that method.
- **SC-002**: An operator running inside the cloud environment can configure the component to authenticate via the instance metadata service and successfully perform state operations without placing any secret in the configuration.
- **SC-003**: 100% of misconfiguration cases for the new methods (missing path, unreadable/malformed key, unreachable credential source) produce a clear, actionable startup error that names the responsible field or cause, with no silent failures and no stack-trace-only output.
- **SC-004**: All four previously existing behaviors (anonymous, static, token, and validation of unknown methods) remain unchanged, verified by the existing test suite passing without modification to their expectations.
- **SC-005**: Credentials for both new methods are refreshed automatically so that a session running longer than a single credential's lifetime continues to operate without operator intervention or restart.
- **SC-006**: The component's published metadata and documentation list service-account-key and metadata authentication as supported, matching the actual runtime behavior with no discrepancy.

## Assumptions

- The target deployment environment is Yandex Cloud managed YDB; the service-account-key and metadata paths are the cloud provider's standard production authentication mechanisms.
- The metadata-service path is only expected to work where the runtime provides a reachable metadata endpoint bound to a service account (e.g., a cloud VM or comparable workload); local developer machines are not expected to support it.
- Credential acquisition and automatic refresh are handled by the provider's standard credential mechanisms; this feature wires those mechanisms in rather than reimplementing token exchange.
- The internal-CA option already present in configuration is intended primarily for managed endpoints using the provider's internal certificate authority and is in scope to validate alongside the new auth methods.
- Sensitive material (key file contents, derived tokens) is never written to logs or error messages.
- Existing state operation behavior (Get/Set/Delete, expiry, ETag) is unchanged; this feature only affects how the connection is authenticated.
- The configuration field names already accepted by the component (`authMethod`, `serviceAccountKeyPath`, `useInternalCA`, `connectionString`) remain the configuration surface; no new authentication method names beyond the five already enumerated are introduced.
