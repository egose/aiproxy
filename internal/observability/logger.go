package observability

import (
	"io"
	"log/slog"
	"os"

	"github.com/google/uuid"
)

type LoggerOptions struct {
	Level slog.Leveler
}

func NewLogger(w io.Writer, opts LoggerOptions) *slog.Logger {
	if w == nil {
		w = os.Stdout
	}
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: opts.Level}))
}

func ParseLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func RequestID(existing string) string {
	if existing != "" {
		return existing
	}
	return uuid.NewString()
}
