package uuid

import "context"

// Use a private type to prevent name collisions with other packages
type key string

const uuidKey key = "UUID"

// NewContext creates a new context with the UUID set to the provided value
func NewContext(ctx context.Context, uuid string) context.Context {
	return context.WithValue(ctx, uuidKey, uuid)
}

// FromContext returns the UUID value stored in ctx, if any.
func FromContext(ctx context.Context) (string, bool) {
	uuid, ok := ctx.Value(uuidKey).(string)
	return uuid, ok
}
