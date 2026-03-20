package logger

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("text format writes text output", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		log := New(Config{Level: "info", Format: "text", Output: &buf})
		log.Info("hello")
		if !strings.Contains(buf.String(), "hello") {
			t.Fatalf("expected output to contain 'hello', got %q", buf.String())
		}
	})

	t.Run("json format writes JSON output", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		log := New(Config{Level: "info", Format: "json", Output: &buf})
		log.Info("hello")
		out := buf.String()
		if !strings.Contains(out, `"msg":"hello"`) {
			t.Fatalf("expected JSON output with msg field, got %q", out)
		}
	})

	t.Run("nil output defaults to stdout", func(t *testing.T) {
		t.Parallel()
		log := New(Config{Level: "info", Format: "text", Output: nil})
		if log == nil {
			t.Fatal("expected non-nil logger")
		}
	})

	t.Run("unknown level defaults to info", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		log := New(Config{Level: "bogus", Format: "text", Output: &buf})
		log.Debug("should-be-hidden")
		log.Info("should-appear")
		out := buf.String()
		if strings.Contains(out, "should-be-hidden") {
			t.Fatal("debug message should not appear at default (info) level")
		}
		if !strings.Contains(out, "should-appear") {
			t.Fatalf("info message should appear, got %q", out)
		}
	})
}

func TestLevelFiltering(t *testing.T) {
	t.Parallel()

	levels := []struct {
		name    string
		level   string
		visible []string
		hidden  []string
	}{
		{
			name:    "debug level shows all",
			level:   "debug",
			visible: []string{"debug-msg", "info-msg", "warn-msg", "error-msg"},
			hidden:  nil,
		},
		{
			name:    "info level hides debug",
			level:   "info",
			visible: []string{"info-msg", "warn-msg", "error-msg"},
			hidden:  []string{"debug-msg"},
		},
		{
			name:    "warn level hides debug and info",
			level:   "warn",
			visible: []string{"warn-msg", "error-msg"},
			hidden:  []string{"debug-msg", "info-msg"},
		},
		{
			name:    "error level shows only error",
			level:   "error",
			visible: []string{"error-msg"},
			hidden:  []string{"debug-msg", "info-msg", "warn-msg"},
		},
	}

	for _, tc := range levels {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			log := New(Config{Level: tc.level, Format: "text", Output: &buf})

			log.Debug("debug-msg")
			log.Info("info-msg")
			log.Warn("warn-msg")
			log.Error("error-msg")

			out := buf.String()
			for _, msg := range tc.visible {
				if !strings.Contains(out, msg) {
					t.Errorf("at level %q: expected %q to be visible, got %q", tc.level, msg, out)
				}
			}
			for _, msg := range tc.hidden {
				if strings.Contains(out, msg) {
					t.Errorf("at level %q: expected %q to be hidden, got %q", tc.level, msg, out)
				}
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	if cfg.Level != "info" {
		t.Errorf("expected default level 'info', got %q", cfg.Level)
	}
	if cfg.Format != "text" {
		t.Errorf("expected default format 'text', got %q", cfg.Format)
	}
	if cfg.Output == nil {
		t.Error("expected non-nil default output")
	}
}

func TestDefault(t *testing.T) {
	t.Parallel()

	log := Default()
	if log == nil {
		t.Fatal("Default() should return non-nil logger")
	}
}

func TestSetGlobalAndGlobal(t *testing.T) {
	t.Run("SetGlobal replaces Global logger", func(t *testing.T) {
		original := Global()
		defer SetGlobal(original)

		var buf bytes.Buffer
		custom := New(Config{Level: "info", Format: "text", Output: &buf})
		SetGlobal(custom)

		got := Global()
		got.Info("via-global")
		if !strings.Contains(buf.String(), "via-global") {
			t.Fatalf("expected global logger to be the custom one, got %q", buf.String())
		}
	})

	t.Run("init sets a usable global logger", func(t *testing.T) {
		log := Global()
		if log == nil {
			t.Fatal("global logger should be set by init()")
		}
	})
}

func TestPackageLevelFunctions(t *testing.T) {
	original := Global()
	defer SetGlobal(original)

	var buf bytes.Buffer
	SetGlobal(New(Config{Level: "debug", Format: "text", Output: &buf}))

	Debug("pkg-debug")
	Info("pkg-info")
	Warn("pkg-warn")
	Error("pkg-error")

	out := buf.String()
	for _, msg := range []string{"pkg-debug", "pkg-info", "pkg-warn", "pkg-error"} {
		if !strings.Contains(out, msg) {
			t.Errorf("expected package-level output to contain %q, got %q", msg, out)
		}
	}
}

func TestPackageLevelWith(t *testing.T) {
	original := Global()
	defer SetGlobal(original)

	var buf bytes.Buffer
	SetGlobal(New(Config{Level: "info", Format: "text", Output: &buf}))

	child := With("component", "test")
	child.Info("with-msg")

	out := buf.String()
	if !strings.Contains(out, "with-msg") {
		t.Fatalf("expected output to contain 'with-msg', got %q", out)
	}
	if !strings.Contains(out, "component") {
		t.Fatalf("expected output to contain 'component' attr, got %q", out)
	}
}

func TestWith(t *testing.T) {
	t.Parallel()

	t.Run("adds key-value pairs to output", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		log := New(Config{Level: "info", Format: "text", Output: &buf})

		child := log.With("user_id", 42)
		child.Info("tagged")

		out := buf.String()
		if !strings.Contains(out, "user_id") {
			t.Errorf("expected 'user_id' in output, got %q", out)
		}
		if !strings.Contains(out, "42") {
			t.Errorf("expected '42' in output, got %q", out)
		}
	})

	t.Run("does not mutate parent logger", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		parent := New(Config{Level: "info", Format: "text", Output: &buf})
		_ = parent.With("extra", "field")

		parent.Info("parent-msg")
		out := buf.String()
		if strings.Contains(out, "extra") {
			t.Errorf("parent logger should not contain child's fields, got %q", out)
		}
	})

	t.Run("chaining preserves all fields", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		log := New(Config{Level: "info", Format: "text", Output: &buf})

		child := log.With("a", "1").With("b", "2").With("c", "3")
		child.Info("chained")

		out := buf.String()
		for _, key := range []string{"a", "b", "c"} {
			if !strings.Contains(out, key) {
				t.Errorf("expected %q in chained output, got %q", key, out)
			}
		}
	})
}

func TestWithContext(t *testing.T) {
	t.Parallel()

	t.Run("stores logger in context", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		log := New(Config{Level: "info", Format: "text", Output: &buf})

		ctx := WithContext(context.Background(), log)
		retrieved := FromContext(ctx)
		retrieved.Info("from-ctx")

		if !strings.Contains(buf.String(), "from-ctx") {
			t.Fatalf("expected logger from context to write to buffer, got %q", buf.String())
		}
	})

	t.Run("preserves With fields through context", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		log := New(Config{Level: "info", Format: "text", Output: &buf})
		tagged := log.With("request_id", "abc-123")

		ctx := WithContext(context.Background(), tagged)
		FromContext(ctx).Info("ctx-tagged")

		out := buf.String()
		if !strings.Contains(out, "request_id") {
			t.Errorf("expected 'request_id' in output, got %q", out)
		}
		if !strings.Contains(out, "abc-123") {
			t.Errorf("expected 'abc-123' in output, got %q", out)
		}
	})
}

func TestFromContext(t *testing.T) {
	t.Parallel()

	t.Run("returns global when context has no logger", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		log := FromContext(ctx)
		if log == nil {
			t.Fatal("FromContext should never return nil")
		}
	})

	t.Run("returns global for context with wrong value type", func(t *testing.T) {
		t.Parallel()
		ctx := context.WithValue(context.Background(), contextKey{}, "not-a-logger")
		log := FromContext(ctx)
		if log == nil {
			t.Fatal("FromContext should fall back to global, not nil")
		}
	})
}
