package logger

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"gopkg.in/natefinch/lumberjack.v2"
)

const DefaultLogFilePath = "./logs/server.log"

type multiHandler struct {
	handlers []slog.Handler
}

func (m *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (m *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	var errs []error
	for _, h := range m.handlers {
		if h.Enabled(ctx, r.Level) {
			if err := h.Handle(ctx, r.Clone()); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func (m *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		next[i] = h.WithAttrs(attrs)
	}
	return &multiHandler{handlers: next}
}

func (m *multiHandler) WithGroup(name string) slog.Handler {
	next := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		next[i] = h.WithGroup(name)
	}
	return &multiHandler{handlers: next}
}

// New creates a structured slog.Logger writing human-readable text to consoleOut
// and rotating JSON logs to logPath (defaults to ./logs/server.log).
func New(nodeEnv string, logPath string, consoleOut io.Writer) (*slog.Logger, func() error, error) {
	if logPath == "" {
		logPath = DefaultLogFilePath
	}
	if consoleOut == nil {
		consoleOut = os.Stdout
	}

	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return nil, nil, err
	}

	level := slog.LevelDebug
	if nodeEnv == "production" {
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: level}

	fileWriter := &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    10, // MB
		MaxBackups: 5,
	}

	consoleHandler := slog.NewTextHandler(consoleOut, opts)
	fileHandler := slog.NewJSONHandler(fileWriter, opts)

	l := slog.New(&multiHandler{
		handlers: []slog.Handler{consoleHandler, fileHandler},
	}).With("service", "DashboardServer")

	return l, fileWriter.Close, nil
}
