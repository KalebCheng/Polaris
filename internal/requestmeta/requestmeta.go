// Package requestmeta carries request metadata that services need for audit.
package requestmeta

import "context"

// Info is immutable request metadata attached to a request context.
type Info struct {
	ClientIP   string
	Method     string
	RequestURI string
}

type contextKey struct{}

// With attaches request metadata to ctx.
func With(ctx context.Context, info Info) context.Context {
	return context.WithValue(ctx, contextKey{}, info)
}

// From returns request metadata from ctx, or the zero value outside HTTP.
func From(ctx context.Context) Info {
	info, _ := ctx.Value(contextKey{}).(Info)
	return info
}
