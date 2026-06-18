# Quickstart: Yandex Cloud Production Authentication

How to configure and verify each authentication method for the YDB pluggable state store. The two
production paths (`serviceAccountKey`, `metadata`) require a real Yandex Cloud environment; the others
work against any YDB.

## Prerequisites

- The `state.ydb` pluggable component built and registered with `daprd` (see project README).
- For managed YDB: a database endpoint like
  `grpcs://ydb.serverless.yandexcloud.net:2135/ru-central1/<folder>/<db>`.

## 1. Service account key (recommended for production)

Create a service account, grant it the YDB role(s), and download a key file:

```bash
yc iam service-account create --name dapr-ydb
yc resource-manager folder add-access-binding <folder-id> \
  --role ydb.editor --subject serviceAccount:<sa-id>
yc iam key create --service-account-id <sa-id> --output /var/run/secrets/ydb/sa-key.json
```

Component manifest:

```yaml
apiVersion: dapr.io/v1alpha1
kind: Component
metadata:
  name: ydbstate
spec:
  type: state.ydb
  version: v1
  metadata:
    - name: connectionString
      value: "grpcs://ydb.serverless.yandexcloud.net:2135/ru-central1/<folder>/<db>"
    - name: authMethod
      value: "serviceAccountKey"
    - name: serviceAccountKeyPath
      value: "/var/run/secrets/ydb/sa-key.json"
    - name: useInternalCA
      value: "true"
```

**Verify**: start the sidecar and perform a state round-trip:

```bash
dapr run --app-id demo --resources-path ./components -- sleep 5 &
curl -s -X POST localhost:3500/v1.0/state/ydbstate \
  -d '[{"key":"k1","value":"hello"}]' -H 'Content-Type: application/json'
curl -s localhost:3500/v1.0/state/ydbstate/k1   # → "hello"
```

**Expected failure modes (field-named, at Init):**

```bash
# Missing path
authMethod=serviceAccountKey, no serviceAccountKeyPath
#   → "metadata field 'serviceAccountKeyPath' is required when authMethod=serviceAccountKey"

# Unreadable / missing file
serviceAccountKeyPath=/nope/sa.json
#   → "metadata field 'serviceAccountKeyPath': cannot read key file ...: ..."
```

## 2. Instance metadata (secret-less, on-cloud only)

Run the workload on a Yandex Cloud VM (or function) with an attached service account; no key file is
needed.

```yaml
spec:
  type: state.ydb
  version: v1
  metadata:
    - name: connectionString
      value: "grpcs://ydb.serverless.yandexcloud.net:2135/ru-central1/<folder>/<db>"
    - name: authMethod
      value: "metadata"
    - name: useInternalCA
      value: "true"
```

**Verify**: same state round-trip as above. If the metadata service is unreachable (e.g. running
off-cloud), Init fails with `failed to open YDB connection: ...` — this is expected off-cloud.

## 3. Existing methods (unchanged)

```yaml
# anonymous (local YDB)
- name: authMethod
  value: "anonymous"

# static
- name: authMethod
  value: "static"
- name: username
  value: "root"
- name: password
  value: "1234"

# token
- name: authMethod
  value: "token"
- name: accessToken
  value: "<iam-token>"
```

## 4. Run the automated checks

```bash
go test ./internal/ydbstate/...      # unit: credentialOptions mapping + SA-key pre-flight
make conformance                     # CRUD/ETag conformance (anonymous auth) — unchanged
```

The live `serviceAccountKey` / `metadata` paths are not exercised by CI (no cloud credentials); use
sections 1–2 against a real environment to validate them.
