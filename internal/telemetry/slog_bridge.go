package telemetry

import (
	"context"
	"log/slog"

	"github.com/rs/zerolog"
)

// SlogHandler implements slog.Handler backed by a zerolog.Logger.
// Install it as the global slog handler so any library using slog routes to zerolog:
//
//	slog.SetDefault(slog.New(telemetry.NewSlogHandler(zlog)))
type SlogHandler struct {
	logger zerolog.Logger
	attrs  []slog.Attr
	group  string
}

// NewSlogHandler returns a slog.Handler that forwards to logger.
func NewSlogHandler(logger zerolog.Logger) *SlogHandler {
	return &SlogHandler{logger: logger}
}

func (h *SlogHandler) Enabled(_ context.Context, level slog.Level) bool {
	return h.logger.GetLevel() <= slogToZerolog(level)
}

func (h *SlogHandler) Handle(_ context.Context, record slog.Record) error {
	e := h.logger.WithLevel(slogToZerolog(record.Level))
	if e == nil {
		return nil
	}
	for _, a := range h.attrs {
		e = appendAttr(e, h.group, a)
	}
	record.Attrs(func(a slog.Attr) bool {
		e = appendAttr(e, h.group, a)
		return true
	})
	e.Msg(record.Message)
	return nil
}

func (h *SlogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	merged := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(merged, h.attrs)
	copy(merged[len(h.attrs):], attrs)
	return &SlogHandler{logger: h.logger, attrs: merged, group: h.group}
}

func (h *SlogHandler) WithGroup(name string) slog.Handler {
	// Nest groups with "." to honour the slog.Handler contract for arbitrarily
	// deep group nesting.
	group := name
	if h.group != "" {
		group = h.group + "." + name
	}
	return &SlogHandler{logger: h.logger, attrs: h.attrs, group: group}
}

func slogToZerolog(level slog.Level) zerolog.Level {
	switch {
	case level >= slog.LevelError:
		return zerolog.ErrorLevel
	case level >= slog.LevelWarn:
		return zerolog.WarnLevel
	case level >= slog.LevelInfo:
		return zerolog.InfoLevel
	default:
		return zerolog.DebugLevel
	}
}

func appendAttr(e *zerolog.Event, group string, a slog.Attr) *zerolog.Event {
	// Resolve LogValuer before inspection so redaction/lazy-eval hooks run.
	a.Value = a.Value.Resolve()

	key := a.Key
	if group != "" {
		key = group + "." + key
	}
	switch a.Value.Kind() {
	case slog.KindString:
		return e.Str(key, a.Value.String())
	case slog.KindInt64:
		return e.Int64(key, a.Value.Int64())
	case slog.KindFloat64:
		return e.Float64(key, a.Value.Float64())
	case slog.KindBool:
		return e.Bool(key, a.Value.Bool())
	case slog.KindDuration:
		return e.Dur(key, a.Value.Duration())
	case slog.KindTime:
		return e.Time(key, a.Value.Time())
	case slog.KindGroup:
		// Recurse with extended group prefix so nested keys are "group.key".
		nestedGroup := key
		for _, ga := range a.Value.Group() {
			e = appendAttr(e, nestedGroup, ga)
		}
		return e
	default:
		return e.Interface(key, a.Value.Any())
	}
}
