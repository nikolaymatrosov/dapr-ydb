# Contract: `state.Store` interface (the component's external contract with Dapr)

The pluggable component's contract is the Dapr `state.Store` interface, reached through
`components-go-sdk/state/v1.Store` (a thin embed of `github.com/dapr/components-contrib/state.Store`).
Signatures below are pinned to the SDK v0.3.0 / Dapr 1.11-era contrib snapshot.

## Required methods (scaffold implements all; persistence stubbed)

```go
// BaseStore
Init(ctx context.Context, metadata state.Metadata) error
Features() []state.Feature
Get(ctx context.Context, req *state.GetRequest) (*state.GetResponse, error)
Set(ctx context.Context, req *state.SetRequest) error
Delete(ctx context.Context, req *state.DeleteRequest) error
GetComponentMetadata() map[string]string   // REQUIRED at this version

// BulkStore (satisfied by embedding state.NewDefaultBulkStore(self))
BulkGet(ctx context.Context, req []state.GetRequest, opts state.BulkGetOpts) ([]state.BulkGetResponse, error)
BulkSet(ctx context.Context, req []state.SetRequest, opts state.BulkStoreOpts) error
BulkDelete(ctx context.Context, req []state.DeleteRequest, opts state.BulkStoreOpts) error
```

## Optional interfaces (NOT implemented in scaffold; features NOT advertised)

```go
// state.TransactionalStore — required only for actorStateStore: true
Multi(ctx context.Context, request *state.TransactionalStateRequest) error
// state.Querier
Query(ctx context.Context, req *state.QueryRequest) (*state.QueryResponse, error)
```

## Scaffold behavior contract (this feature)

| Method | Scaffold behavior | Verified by |
|--------|-------------------|-------------|
| `Init` | Parse + validate metadata; open YDB driver; return field-named error on bad config; never panic | metadata unit tests; US2 acceptance |
| `Features` | Return **empty** slice (no unimplemented capability advertised) | store unit test; constitution Principle I |
| `Get` | Return a typed "not implemented" error (no real read yet) | store unit test |
| `Set` | Return a typed "not implemented" error | store unit test |
| `Delete` | Return a typed "not implemented" error | store unit test |
| `GetComponentMetadata` | Return the metadata schema map (may be empty) | compiles; interface satisfied |
| `BulkGet/Set/Delete` | Delegated to single-key ops via `DefaultBulkStore` | compiles; interface satisfied |
| construction | `YDBStore` must satisfy `state.Store`; assert with `var _ state.Store = (*YDBStore)(nil)` | compile-time assertion |

> Stubbed ops MUST return a clear, typed error (e.g. a sentinel `errNotImplemented`) — never a
> success with empty data, which would violate honest-contract expectations.

## Bootstrap contract (`cmd/daprd-ydb/main.go`)

```go
package main

import (
    dapr "github.com/dapr-sandbox/components-go-sdk"
    "github.com/dapr-sandbox/components-go-sdk/state/v1"
    "github.com/nikolaymatrosov/dapr-ydb/internal/ydbstate"
)

func main() {
    dapr.Register("ydb", dapr.WithStateStore(func() state.Store {
        return ydbstate.New()
    }))
    dapr.MustRun()
}
```

- `dapr.Register("ydb", ...)` → socket `ydb.sock` in the components sockets folder
  (default `/tmp/dapr-components-sockets`, override `DAPR_COMPONENTS_SOCKETS_FOLDER`).
- Addressed by component manifests with `spec.type: state.ydb`.
- Health/liveness (Ping) is served by the SDK framework — no method implemented by us.

## ETag error contract (for later features; documented now)

```go
return state.NewETagError(state.ETagMismatch, underlyingErr) // on optimistic-concurrency conflict
return state.NewETagError(state.ETagInvalid, underlyingErr)  // on malformed caller ETag
```
