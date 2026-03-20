package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"sync/atomic"
)

// Logger interface for application logging.
// Can be replaced with any implementation (zerolog, zap, logrus).
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)

	// With returns a new Logger with additional context fields
	With(args ...any) Logger
}

// Config holds logger configuration
type Config struct {
	Level  string // "debug", "info", "warn", "error"
	Format string // "text", "json"
	Output io.Writer
}

// DefaultConfig returns default logger configuration
func DefaultConfig() Config {
	return Config{
		Level:  "info",
		Format: "text",
		Output: os.Stdout,
	}
}

// slogLogger wraps slog.Logger to implement Logger interface
type slogLogger struct {
	log *slog.Logger
}

// New creates a new Logger with the given configuration
func New(cfg Config) Logger {
	var level slog.Level
	switch cfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	output := cfg.Output
	if output == nil {
		output = os.Stdout
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	var handler slog.Handler
	if cfg.Format == "json" {
		handler = slog.NewJSONHandler(output, opts)
	} else {
		handler = slog.NewTextHandler(output, opts)
	}

	return &slogLogger{
		log: slog.New(handler),
	}
}

// Default creates a logger with default configuration
func Default() Logger {
	return New(DefaultConfig())
}

func (l *slogLogger) Debug(msg string, args ...any) {
	l.log.Debug(msg, args...)
}

func (l *slogLogger) Info(msg string, args ...any) {
	l.log.Info(msg, args...)
}

func (l *slogLogger) Warn(msg string, args ...any) {
	l.log.Warn(msg, args...)
}

func (l *slogLogger) Error(msg string, args ...any) {
	l.log.Error(msg, args...)
}

func (l *slogLogger) With(args ...any) Logger {
	return &slogLogger{
		log: l.log.With(args...),
	}
}

var global atomic.Value

func init() {
	global.Store(Default())
}

// SetGlobal sets the global logger instance
func SetGlobal(l Logger) {
	global.Store(l)
}

// Global returns the global logger instance
func Global() Logger {
	return global.Load().(Logger)
}

// Package-level convenience functions using global logger

func Debug(msg string, args ...any) {
	Global().Debug(msg, args...)
}

func Info(msg string, args ...any) {
	Global().Info(msg, args...)
}

func Warn(msg string, args ...any) {
	Global().Warn(msg, args...)
}

func Error(msg string, args ...any) {
	Global().Error(msg, args...)
}

func With(args ...any) Logger {
	return Global().With(args...)
}

// ContextKey for storing logger in context
type contextKey struct{}

// WithContext returns a new context with the logger
func WithContext(ctx context.Context, l Logger) context.Context {
	return context.WithValue(ctx, contextKey{}, l)
}

// FromContext returns the logger from context, or global logger if not found
func FromContext(ctx context.Context) Logger {
	if l, ok := ctx.Value(contextKey{}).(Logger); ok {
		return l
	}
	return Global()
}
