package telemetry

import (
	"context"

	"github.com/stlimtat/bunshin-go/pkg/core"
	"github.com/stlimtat/bunshin-go/pkg/middleware"
)

// WithLangSmith returns a Middleware that creates a root RunContext and stores
// it in the request context. Downstream nodes call FromContext to read it and
// Child() to derive their own child spans.
//
// Use once at the outermost Runnable in a workflow chain.
func WithLangSmith(projectName string) middleware.Middleware {
	return func(next core.Runnable) core.Runnable {
		return core.NewRunnableFuncWithStream(
			next.Name(),
			func(ctx context.Context, input any) (any, error) {
				rc := NewRunContext(projectName)
				ctx = WithContext(ctx, rc)
				return next.Invoke(ctx, input)
			},
			func(ctx context.Context, input any) (<-chan core.StreamChunk, error) {
				rc := NewRunContext(projectName)
				ctx = WithContext(ctx, rc)
				return next.Stream(ctx, input)
			},
		)
	}
}
