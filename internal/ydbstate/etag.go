package ydbstate

import "github.com/google/uuid"

// newETag returns a fresh, opaque optimistic-concurrency token. A random UUIDv4
// is generated for every successful write and is never reused (constitution
// Principle III); callers treat it as opaque.
func newETag() string {
	return uuid.NewString()
}
