package telemetry

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/stlimtat/bunshin-go/pkg/core"
	"github.com/stlimtat/bunshin-go/pkg/middleware"
)

const tracerName = "bunshin-go"

// WithOTEL returns a Middleware that creates an OTEL span for each Invoke call.
// The span name is the Runnable's name. Errors are recorded on the span.
//
// Pairs with WithLangSmith: apply both at the workflow boundary:
//
//	middleware.Chain(agent,
//	    telemetry.WithLangSmith("my-project"),
//	    telemetry.WithOTEL(),
//	)
//
// OTEL context propagation is handled by the tracer provider configured at
// process startup via otel.SetTracerProvider.
func WithOTEL() middleware.Middleware {
	return func(next core.Runnable) core.Runnable {
		tracer := otel.Tracer(tracerName)
		name := next.Name()
		return core.NewRunnableFuncWithStream(
			name,
			func(ctx context.Context, input any) (any, error) {
				ctx, span := tracer.Start(ctx, name,
					oteltrace.WithSpanKind(oteltrace.SpanKindInternal),
					oteltrace.WithAttributes(attribute.String("bunshin.runnable", name)),
				)
				defer span.End()

				out, err := next.Invoke(ctx, input)
				if err != nil {
					span.RecordError(err)
					span.SetStatus(codes.Error, err.Error())
				} else {
					span.SetStatus(codes.Ok, "")
				}
				return out, err
			},
			func(ctx context.Context, input any) (<-chan core.StreamChunk, error) {
				ctx, span := tracer.Start(ctx, name+"/stream",
					oteltrace.WithSpanKind(oteltrace.SpanKindInternal),
					oteltrace.WithAttributes(attribute.String("bunshin.runnable", name)),
				)
				ch, err := next.Stream(ctx, input)
				if err != nil {
					span.RecordError(err)
					span.SetStatus(codes.Error, err.Error())
					span.End()
					return nil, err
				}
				// Wrap the channel to close the span when streaming completes.
				wrapped := make(chan core.StreamChunk)
				go func() {
					defer span.End()
					defer close(wrapped)
					for chunk := range ch {
						select {
						case wrapped <- chunk:
						case <-ctx.Done():
							return
						}
					}
					span.SetStatus(codes.Ok, "")
				}()
				return wrapped, nil
			},
		)
	}
}
