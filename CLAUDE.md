<!-- SPECKIT START -->
For additional context about technologies to be used, project structure,
shell commands, and other important information, read the current plan:
`specs/003-yc-auth-methods/plan.md`

Active feature: 003-yc-auth-methods — wire the two Yandex Cloud production auth paths
(serviceAccountKey, metadata) that currently dead-end in credentialOptions() with
"not yet supported", and honor the latent useInternalCA flag. Single new dep:
github.com/ydb-platform/ydb-go-yc (provides WithServiceAccountKeyFileCredentials,
WithMetadataCredentials, WithInternalCA). No new state.Feature advertised; change is
confined to internal/ydbstate/store.go credentialOptions() + metadata.yaml docs.
serviceAccountKey pre-flights the key file (os.ReadFile) for a field-named Init error.
See also research.md, data-model.md, contracts/auth-methods.md.
Prior: specs/002-kv-get-set-delete/plan.md, specs/001-project-scaffold/plan.md.
<!-- SPECKIT END -->
