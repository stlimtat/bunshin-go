package telemetry

import (
	"log/slog"
	"os"

	"github.com/rs/zerolog"
)

// NewLogger creates a zerolog.Logger at the requested level and bridges the
// global slog default to it. Any third-party library that calls slog.Info() /
// slog.Error() etc. will automatically route through zerolog.
func NewLogger(level string) zerolog.Logger {
	lvl, err := zerolog.ParseLevel(level)
	if err != nil {
		lvl = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(lvl)
	zlog := zerolog.New(os.Stderr).With().Timestamp().Logger()
	// Bridge: route global slog calls through zerolog.
	slog.SetDefault(slog.New(NewSlogHandler(zlog)))
	return zlog
}
