# Quickstart: YDB Query/Exec Output Binding

This binding adds raw-YQL `query`/`exec` to the existing YDB plugin. The plugin binary now serves
**two** components on the same socket: the `state.ydb` state store (unchanged) and the new
`bindings.ydb` output binding.

## 1. Component manifest

Create a binding component alongside your existing state-store manifest. It reuses the same
connection/auth fields:

```yaml
apiVersion: dapr.io/v1alpha1
kind: Component
metadata:
  name: ydb-sql
spec:
  type: bindings.ydb
  version: v1
  metadata:
    - name: connectionString
      value: "grpcs://ydb.serverless.yandexcloud.net:2135/ru-central1/b1g.../etn..."
    - name: authMethod
      value: "serviceAccountKey"
    - name: serviceAccountKeyPath
      value: "/secrets/sa-key.json"
```

The pluggable socket is discovered exactly as for the state store — no extra binary, no daprd
rebuild.

## 2. Run a query

Invoke the binding with operation `query` and the statement in metadata:

```bash
curl -X POST http://localhost:3500/v1.0/bindings/ydb-sql \
  -H 'Content-Type: application/json' \
  -d '{
        "operation": "query",
        "metadata": { "sql": "SELECT id, name FROM users WHERE id = ?", "params": "[1]" }
      }'
# => [ { "id": 1, "name": "alice" } ]
```

Zero matches return `[]`.

## 3. Execute a write or DDL

```bash
# insert
curl -X POST http://localhost:3500/v1.0/bindings/ydb-sql \
  -d '{ "operation": "exec",
        "metadata": { "sql": "INSERT INTO users (id, name) VALUES (?, ?)", "params": "[2, \"bob\"]" } }'

# create a table (DDL needs scheme mode)
curl -X POST http://localhost:3500/v1.0/bindings/ydb-sql \
  -d '{ "operation": "exec",
        "metadata": { "sql": "CREATE TABLE users (id Uint64, name Utf8, PRIMARY KEY (id))",
                      "queryMode": "scheme" } }'
```

## 4. Parameters

**Positional** (postgres-style, `?`, types inferred from JSON):

```json
{ "sql": "SELECT * FROM t WHERE id = ? AND active = ?", "params": "[42, true]" }
```

**Named + typed** (force exact YDB types — use when an integral JSON number must be `Uint64`,
`Timestamp`, etc.):

```json
{ "sql": "SELECT * FROM t WHERE id = $id AND created >= $since",
  "params": "{\"$id\": {\"type\":\"Uint64\",\"value\":42}, \"$since\": {\"type\":\"Timestamp\",\"value\":\"2026-06-18T00:00:00Z\"}}" }
```

A value containing YQL-like text is always bound as data — it cannot alter the statement.

## 5. Verify locally

```bash
# fast unit tests (no DB): param building, result serialization, error mapping
go test ./internal/ydbbinding/...

# integration test against a real/containerized YDB (authoritative gate)
go test ./tests/integration/... -run Binding   # requires a reachable YDB (see Makefile)
```

The integration test covers `query`, `exec` (DML + DDL), positional and named+typed params, an
injection-style value treated as data, and the unknown-operation / missing-statement errors.
