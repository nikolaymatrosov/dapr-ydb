package ydbstate

import "errors"

// errNotImplemented is returned by persistence operations that the scaffold does
// not yet implement. Stubbed operations MUST return a clear, typed error rather
// than a misleading success (constitution Principle I: honest contract).
var errNotImplemented = errors.New("ydbstate: operation not implemented in this build")
