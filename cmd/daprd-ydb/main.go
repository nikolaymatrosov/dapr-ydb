// Command daprd-ydb runs the pluggable Dapr state store backed by YDB.
//
// It registers a state store under the socket name "ydb" (addressed by Dapr
// component manifests with spec.type: state.ydb) and serves it over a Unix
// Domain Socket in the Dapr components sockets folder (default
// /tmp/dapr-components-sockets, override via DAPR_COMPONENTS_SOCKETS_FOLDER).
// No rebuild of daprd is required.
package main

import (
	"os"

	dapr "github.com/dapr-sandbox/components-go-sdk"
	bindingsv1 "github.com/dapr-sandbox/components-go-sdk/bindings/v1"
	state "github.com/dapr-sandbox/components-go-sdk/state/v1"

	"github.com/nikolaymatrosov/dapr-ydb/internal/ydbbinding"
	"github.com/nikolaymatrosov/dapr-ydb/internal/ydbstate"
)

// Dapr sockets-folder environment variables. The components-go-sdk (v0.3.0)
// reads the singular "COMPONENT" form (with an even older single-SOCKET
// fallback), while modern daprd and the Dapr documentation use the plural
// "COMPONENTS" form. We bridge them so the documented variable is honored.
const (
	sdkSocketEnv     = "DAPR_COMPONENT_SOCKETS_FOLDER"  // what the SDK reads
	runtimeSocketEnv = "DAPR_COMPONENTS_SOCKETS_FOLDER" // what daprd/docs use
)

func main() {
	syncSocketFolderEnv()

	// One socket ("ydb") serves both component types: the state store
	// (state.ydb) and the output binding (bindings.ydb). The SDK appends each
	// onto the same gRPC server, so no extra binary is needed.
	dapr.Register("ydb",
		dapr.WithStateStore(func() state.Store {
			return ydbstate.New()
		}),
		dapr.WithOutputBinding(func() bindingsv1.OutputBinding {
			return ydbbinding.New()
		}),
	)
	dapr.MustRun()
}

// syncSocketFolderEnv mirrors the modern (plural) sockets-folder variable to the
// one the SDK reads when only the modern variable is set, so a custom sockets
// folder configured the documented way is actually honored by the component.
func syncSocketFolderEnv() {
	if v, ok := resolveSocketFolderEnv(os.Getenv(sdkSocketEnv), os.Getenv(runtimeSocketEnv)); ok {
		_ = os.Setenv(sdkSocketEnv, v)
	}
}

// resolveSocketFolderEnv returns the value the SDK variable should take given the
// current SDK (singular) and runtime (plural) values, and whether it must be set.
// The SDK variable wins when already set; otherwise the runtime variable is used.
func resolveSocketFolderEnv(sdkVal, runtimeVal string) (string, bool) {
	if sdkVal == "" && runtimeVal != "" {
		return runtimeVal, true
	}
	return sdkVal, false
}
