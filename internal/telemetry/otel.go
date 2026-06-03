package telemetry

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// OTELBackend implements TelemetryBackend using OpenTelemetry spans.
// Each Run becomes a span; parent-child relationships are propagated via context.
type OTELBackend struct {
	tracer trace.Tracer
	mu     sync.Mutex
	spans  map[uuid.UUID]trace.Span
}

// NewOTELBackend constructs an OTELBackend using the given tracer.
// Obtain a tracer via otel.Tracer("your-service-name") after configuring
// a TracerProvider (e.g. with go.opentelemetry.io/otel/sdk/trace).
// Returns an error if tracer is nil.
func NewOTELBackend(tracer trace.Tracer) (*OTELBackend, error) {
	if tracer == nil {
		return nil, errors.New("telemetry: tracer must not be nil")
	}
	return &OTELBackend{
		tracer: tracer,
		spans:  make(map[uuid.UUID]trace.Span),
	}, nil
}

func (b *OTELBackend) StartRun(ctx context.Context, run *Run) (context.Context, error) {
	if ctx.Err() != nil {
		return ctx, ctx.Err()
	}
	attrs := []attribute.KeyValue{
		attribute.String("bunshin.run.id", run.ID.String()),
		attribute.String("bunshin.run.type", string(run.RunType)),
	}
	if run.ParentID != nil {
		attrs = append(attrs, attribute.String("bunshin.run.parent_id", run.ParentID.String()))
	}
	ctx, span := b.tracer.Start(ctx, run.Name, trace.WithAttributes(attrs...))

	b.mu.Lock()
	if _, exists := b.spans[run.ID]; exists {
		b.mu.Unlock()
		span.End()
		return ctx, fmt.Errorf("telemetry: run %s already started", run.ID)
	}
	b.spans[run.ID] = span
	b.mu.Unlock()

	return WithRunID(ctx, run.ID), nil
}

func (b *OTELBackend) EndRun(_ context.Context, runID uuid.UUID, _ map[string]any, err error) error {
	b.mu.Lock()
	span, ok := b.spans[runID]
	delete(b.spans, runID)
	b.mu.Unlock()

	if !ok {
		return fmt.Errorf("run %s not found", runID)
	}
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)
	} else {
		span.SetStatus(codes.Ok, "")
	}
	span.End()
	return nil
}

// AddFeedback is a no-op for OTEL; feedback belongs in LangSmith.
func (b *OTELBackend) AddFeedback(_ context.Context, _ uuid.UUID, _ Feedback) error {
	return nil
}

// Flush is a no-op; flushing is the responsibility of the TracerProvider.
// Call TracerProvider.Shutdown(ctx) before process exit.
func (b *OTELBackend) Flush(_ context.Context) error {
	return nil
}
