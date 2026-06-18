# Quickstart: Pluggable YDB State Store (scaffold)

This validates the scaffold's three user stories: the component loads in Dapr (US1), config is
parsed/validated (US2), and the build/test/conformance toolchain runs (US3). Targets the
**scaffold** — persistence ops are stubbed.

## Prerequisites

- Go 1.24+ (verified with go1.26.1)
- Docker (for a local YDB instance and for `daprd`/Dapr CLI if used)
- Dapr CLI / `daprd` 1.11+ (for the load test in US1)
- `golangci-lint`

## 1. Build (US1 / FR-001, SC-001)

```bash
make build            # produces ./bin/daprd-ydb
# or: go build -o bin/daprd-ydb ./cmd/daprd-ydb
```

Expected: a binary is produced from a clean checkout with no manual steps.

## 2. Run the component & load it in Dapr (US1 / FR-002..FR-004, SC-002)

```bash
# Start a local YDB (for Init to connect to)
docker compose -f deploy/docker-compose.yml up -d ydb

# Run the pluggable component (creates ydb.sock in the sockets folder).
# daprd and the Dapr docs use DAPR_COMPONENTS_SOCKETS_FOLDER (plural "COMPONENTS").
# The component bridges this to the SDK's DAPR_COMPONENT_SOCKETS_FOLDER (singular)
# automatically, so setting the modern variable below is sufficient.
export DAPR_COMPONENTS_SOCKETS_FOLDER=/tmp/dapr-components-sockets
mkdir -p "$DAPR_COMPONENTS_SOCKETS_FOLDER"
make run            # runs ./bin/daprd-ydb (sets both variable names for you)

# In another shell: start daprd pointed at a components dir containing components/ydb.yaml
daprd --app-id demo --resources-path ./components --dapr-grpc-port 50001
```

Expected:
- `ydb.sock` appears in `$DAPR_COMPONENTS_SOCKETS_FOLDER`.
- `daprd` discovers the socket, loads `state.ydb`, and reports the component healthy with no
  errors in its startup log. (Liveness is served by the SDK framework.)

Sample `components/ydb.yaml`:

```yaml
apiVersion: dapr.io/v1alpha1
kind: Component
metadata:
  name: ydb-state
spec:
  type: state.ydb
  version: v1
  metadata:
    - name: connectionString
      value: "grpc://localhost:2136/local"
    - name: authMethod
      value: "anonymous"
```

## 3. Configuration validation (US2 / FR-005, FR-006, SC-004)

```bash
go test ./internal/ydbstate -run TestMetadata -v
```

Expected:
- Valid manifest → `Init` succeeds and applies declared fields.
- Missing `connectionString` → `Init` fails with an error naming `connectionString`.
- `authMethod=static` without `username` → error naming `username`.
- Unparseable `useInternalCA` → error naming `useInternalCA` and the expected form.
- No panic in any case; the host sidecar is unaffected.

## 4. Lint & unit tests (US3 / FR-009, SC-005)

```bash
make lint            # golangci-lint run ./...
make test            # go test ./... (excludes conformance, which needs YDB)
```

Expected: lint and unit tests execute and report results from a clean checkout.

## 5. Conformance harness (US3 / FR-010, SC-005)

```bash
docker compose -f deploy/docker-compose.yml up -d ydb
make conformance     # runs the Dapr state conformance suite against YDB
```

Expected: the harness **compiles** and runs the conformance suite for the features the
component advertises. Since the scaffold advertises **no** features, the suite establishes the
honest minimal baseline (it does not assert Get/Set/ETag/TTL behavior yet — those arrive with
later features).

## 6. Confirm the contract is satisfied at compile time

The package includes `var _ state.Store = (*YDBStore)(nil)`; a successful build proves the
component satisfies the Dapr `state.Store` contract.

---

**Done when**: steps 1–6 pass. This is the foundation; subsequent features fill in real
persistence (Get/Set/Delete → bulk → ETag → TTL → transactions → query), each advertised in
`Features()` only after it passes conformance.
