// Command daprd-ydb runs the pluggable Dapr state store backed by YDB.
//
// It registers a state store under the socket name "ydb" (addressed by Dapr
// component manifests with spec.type: state.ydb) and serves it over a Unix
// Domain Socket in the Dapr components sockets folder (default
// /tmp/dapr-components-sockets, override via DAPR_COMPONENTS_SOCKETS_FOLDER).
// No rebuild of daprd is required.
package main

import (
	dapr "github.com/dapr-sandbox/components-go-sdk"
	state "github.com/dapr-sandbox/components-go-sdk/state/v1"

	"github.com/nikolaymatrosov/dapr-ydb/internal/ydbstate"
)

func main() {
	dapr.Register("ydb", dapr.WithStateStore(func() state.Store {
		return ydbstate.New()
	}))
	dapr.MustRun()
}
