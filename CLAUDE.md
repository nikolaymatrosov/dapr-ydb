<!-- SPECKIT START -->
For additional context about technologies to be used, project structure,
shell commands, and other important information, read the current plan:
`specs/001-project-scaffold/plan.md`

Active feature: 001-project-scaffold — pluggable Dapr state store backed by YDB.
Key tech: Go 1.24+, dapr-sandbox/components-go-sdk (v0.3.0), dapr/components-contrib
state.Store, ydb-platform/ydb-go-sdk/v3. Pluggable component over a Unix Domain Socket
(register "ydb" → spec.type: state.ydb). See also research.md, data-model.md, contracts/.
<!-- SPECKIT END -->
