<!-- SPECKIT START -->
For additional context about technologies to be used, project structure,
shell commands, and other important information, read the current plan:
`specs/002-kv-get-set-delete/plan.md`

Active feature: 002-kv-get-set-delete — real Get/Set/Delete against the documented KV
schema, with FeatureETag advertised only after the Dapr conformance eTag scenarios pass.
Key tech: Go 1.26.4+, dapr-sandbox/components-go-sdk (v0.3.0), dapr/components-contrib
state.Store, ydb-platform/ydb-go-sdk/v3 (query service + serializable DoTx for CAS).
ETag = opaque UUID per write; reads filter expired rows. See also research.md,
data-model.md, contracts/state-operations.md. Prior: specs/001-project-scaffold/plan.md.
<!-- SPECKIT END -->
