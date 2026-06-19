# Contract: `bindings.ydb` Output Binding Operations

Component type `bindings.ydb`, served on socket `ydb` alongside `state.ydb`. Implements
`bindings.OutputBinding`: `Init`, `Invoke`, `Operations`, `Close` (+ optional `Ping`).

`Operations()` MUST return exactly: `query`, `exec` (constitution Principle I analogue — advertise
only what is implemented).

---

## Operation: `query`

Run a row-returning YQL statement and return the rows as JSON.

**Request**

| Field | Location | Required | Description |
|-------|----------|----------|-------------|
| `operation` | request | yes | `query` |
| `sql` | `metadata` | yes | YQL statement (typically `SELECT`). |
| `params` | `metadata` | no | JSON array (positional) or object (named+typed). See data-model. |

**Response**

- `data`: JSON array of row objects keyed by column name; `[]` when no rows match.
- `contentType`: `application/json`.
- `metadata`: `operation=query`, `sql`, `start-time`, `end-time`, `duration`.

**Behavior**

- Zero rows → `[]` + success (FR-010).
- Invalid YQL → error conveying YDB's reason; component stays up (FR-008).
- NULL/typed columns serialized per data-model mapping (FR-003, SC-003).

**Example**

```jsonc
// request
{ "operation": "query",
  "metadata": { "sql": "SELECT id, name FROM users WHERE id = ?", "params": "[1]" } }
// response.data
[ { "id": 1, "name": "alice" } ]
```

---

## Operation: `exec`

Run a non-row-returning YQL statement (DML or DDL).

**Request**

| Field | Location | Required | Description |
|-------|----------|----------|-------------|
| `operation` | request | yes | `exec` |
| `sql` | `metadata` | yes | YQL statement (`INSERT`/`UPDATE`/`DELETE`/`CREATE`/`DROP`/…). |
| `params` | `metadata` | no | JSON array (positional) or object (named+typed). |
| `queryMode` | `metadata` | no | `data` (default) \| `scheme` (DDL) \| `scripting`. |

**Response**

- `data`: empty.
- `metadata`: `operation=exec`, `sql`, `start-time`, `end-time`, `duration`, and `rows-affected`
  (best-effort; may be `unknown`).

**Behavior**

- Valid DML durably applied; valid DDL applies the schema change (FR-004).
- Statement rejected by DB → descriptive error, no partial state for that statement (FR-008).

**Example**

```jsonc
{ "operation": "exec",
  "metadata": { "sql": "INSERT INTO users (id, name) VALUES (?, ?)", "params": "[2, \"bob\"]" } }
```

---

## Errors (all operations)

| Condition | Result |
|-----------|--------|
| Unsupported `operation` | Error: "operation not supported", lists `query`, `exec` (FR-011). |
| Missing/empty `sql` | Error: required statement missing; no DB call (FR-011). |
| Malformed `params` JSON | Error describing the parse failure; no DB call. |
| Unknown type name (named form) | Error naming the parameter (FR-007). |
| Missing value / uncoercible value for referenced param | Error naming the parameter; statement not executed (FR-007). |
| YDB rejects statement | Error conveying YDB's reason (FR-008). |
| YDB unreachable | Error; component remains usable for later requests, state store unaffected (FR-008, SC-004). |

All errors are returned as the `Invoke` error; none crash or panic the process (FR-008).

## Parameter forms

`params` is type-discriminated by JSON kind:
- **Array** → positional, bound to `?` placeholders, types inferred (lossy; see data-model).
- **Object** → named+typed, bound to `$name` placeholders with explicit YDB types.

One form per invocation. Supplying neither is valid for parameterless statements.

## Configuration

The binding reads the **same** connection/auth manifest fields as `state.ydb`
(`connectionString`, `authMethod`, `username`/`password`, `accessToken`,
`serviceAccountKeyPath`, `useInternalCA`) via the shared `internal/ydbconfig` package (FR-012).
Documented in `bindings.metadata.yaml`.
