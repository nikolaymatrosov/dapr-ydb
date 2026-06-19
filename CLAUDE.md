<!-- SPECKIT START -->
For additional context about technologies to be used, project structure,
shell commands, and other important information, read the current plan:
`specs/004-query-exec-binding/plan.md`

Active feature: 004-query-exec-binding — add a second Dapr component, a YDB output
binding (bindings.ydb) exposing query/exec over raw YQL (like contrib bindings/postgres),
served by the SAME plugin binary as the existing state.ydb store via
dapr.Register("ydb", WithStateStore(...), WithOutputBinding(...)). Execute via the YDB
database/sql layer (sql.OpenDB + ydb.MustConnector with WithAutoDeclare + WithPositionalArgs)
so postgres-style positional `?` params work with auto type-declaration; named+typed params
are the escape hatch for exact YDB types. params metadata field is JSON: array=>positional,
object=>named+typed. Statement in metadata["sql"]; DDL uses queryMode=scheme. Extract shared
connection/auth into internal/ydbconfig (reused by state store + binding). New code:
internal/ydbbinding/, internal/ydbconfig/, bindings.metadata.yaml, main.go registration,
integration test (real YDB) as the gate. No new module dep. State store behavior unchanged.
See also research.md, data-model.md, contracts/binding-operations.md.
Prior: specs/003-yc-auth-methods/plan.md, specs/002-kv-get-set-delete/plan.md, specs/001-project-scaffold/plan.md.
<!-- SPECKIT END -->
