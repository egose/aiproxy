package observability

import (
	"io"
	"log/slog"
	"os"

	"github.com/google/uuid"
)

func NewLogger(w io.Writer) *slog.Logger {
	if w == nil {
		w = os.Stdout
	}
	return slog.New(slog.NewJSONHandler(w, nil))
}

func RequestID(existing string) string {
	if existing != "" {
		return existing
	}
	return uuid.NewString()
}
