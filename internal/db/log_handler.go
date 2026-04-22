package db

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

const logHandlerBufSize = 512

// LogHandler is a slog.Handler that persists records to the database via a
// buffered channel. If the channel is full the record is silently dropped —
// the handler must never block the request path. DB errors are also silently
// dropped after logging to stderr once.
type LogHandler struct {
	repo     *LogRepo
	ch       chan LogEntry
	preAttrs []slog.Attr
	level    slog.Level
}

// NewLogSlogHandler returns a non-blocking slog.Handler backed by repo.
// Call Start to begin draining; call Stop to flush and shut down.
func NewLogSlogHandler(repo *LogRepo, minLevel slog.Level) *LogHandler {
	h := &LogHandler{
		repo:  repo,
		ch:    make(chan LogEntry, logHandlerBufSize),
		level: minLevel,
	}
	go h.drain()
	return h
}

func (h *LogHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *LogHandler) Handle(_ context.Context, rec slog.Record) error {
	if rec.Level < h.level {
		return nil
	}

	component := ""
	fields := map[string]string{}

	for _, a := range h.preAttrs {
		if a.Key == "component" {
			component = fmt.Sprintf("%v", a.Value.Any())
		} else if a.Key != "" {
			fields[a.Key] = fmt.Sprintf("%v", a.Value.Any())
		}
	}

	rec.Attrs(func(a slog.Attr) bool {
		if a.Key == "component" {
			component = fmt.Sprintf("%v", a.Value.Any())
		} else if a.Key != "" {
			fields[a.Key] = fmt.Sprintf("%v", a.Value.Any())
		}
		return true
	})

	e := LogEntry{
		Ts:        rec.Time,
		Level:     rec.Level.String(),
		Component: component,
		Message:   rec.Message,
		Fields:    fields,
	}
	if e.Ts.IsZero() {
		e.Ts = time.Now()
	}

	// Non-blocking send — drop if full.
	select {
	case h.ch <- e:
	default:
	}
	return nil
}

func (h *LogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	combined := append(append([]slog.Attr{}, h.preAttrs...), attrs...)
	return &LogHandler{repo: h.repo, ch: h.ch, preAttrs: combined, level: h.level}
}

func (h *LogHandler) WithGroup(_ string) slog.Handler {
	return h
}

func (h *LogHandler) drain() {
	for e := range h.ch {
		_ = h.repo.Insert(context.Background(), e)
	}
}
