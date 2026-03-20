package version

import (
	"runtime"
	"strings"
	"testing"
)

func TestGet(t *testing.T) {
	t.Parallel()

	info := Get()
	if info.GitCommit == "" {
		t.Fatalf("GitCommit should not be empty")
	}
	if info.GoVersion != runtime.Version() {
		t.Fatalf("expected GoVersion %q, got %q", runtime.Version(), info.GoVersion)
	}
}

func TestInfoString(t *testing.T) {
	t.Parallel()

	info := Info{
		GitCommit: "abc123def",
		GitDate:   "2026-01-01",
		GitBranch: "main",
		BuildDate: "2026-01-02",
		GoVersion: "go1.25.5",
		Platform:  "linux/amd64",
	}

	s := info.String()
	for _, want := range []string{"commit=abc123def", "date=2026-01-01", "branch=main", "built=2026-01-02", "go=go1.25.5", "platform=linux/amd64"} {
		if !strings.Contains(s, want) {
			t.Fatalf("String() = %q, missing %q", s, want)
		}
	}
}

func TestInfoShort(t *testing.T) {
	t.Parallel()

	t.Run("truncates long commit to 7 chars", func(t *testing.T) {
		t.Parallel()
		info := Info{GitCommit: "abc123def456", GitDate: "2026-01-01"}
		got := info.Short()
		if !strings.HasPrefix(got, "abc123d") {
			t.Fatalf("expected short commit prefix %q, got %q", "abc123d", got)
		}
	})

	t.Run("short commit unchanged", func(t *testing.T) {
		t.Parallel()
		info := Info{GitCommit: "abc", GitDate: "2026-01-01"}
		got := info.Short()
		if !strings.HasPrefix(got, "abc") {
			t.Fatalf("expected short commit %q, got %q", "abc", got)
		}
	})

	t.Run("exactly 7 chars unchanged", func(t *testing.T) {
		t.Parallel()
		info := Info{GitCommit: "1234567", GitDate: "2026-01-01"}
		got := info.Short()
		if !strings.HasPrefix(got, "1234567") {
			t.Fatalf("expected %q prefix, got %q", "1234567", got)
		}
	})
}

func TestInfoLogFields(t *testing.T) {
	t.Parallel()

	info := Info{
		GitCommit: "abc",
		GitDate:   "2026-01-01",
		GitBranch: "main",
		BuildDate: "2026-01-02",
		GoVersion: "go1.25.5",
		Platform:  "linux/amd64",
	}

	fields := info.LogFields()
	if len(fields) != 12 {
		t.Fatalf("expected 12 elements, got %d", len(fields))
	}

	expected := map[string]string{
		"git_commit": "abc",
		"git_date":   "2026-01-01",
		"git_branch": "main",
		"build_date": "2026-01-02",
		"go_version": "go1.25.5",
		"platform":   "linux/amd64",
	}

	for i := 0; i < len(fields); i += 2 {
		key, ok := fields[i].(string)
		if !ok {
			t.Fatalf("fields[%d] should be string, got %T", i, fields[i])
		}
		val, ok := fields[i+1].(string)
		if !ok {
			t.Fatalf("fields[%d] should be string, got %T", i+1, fields[i+1])
		}
		if want, exists := expected[key]; exists && val != want {
			t.Fatalf("field %q = %q, want %q", key, val, want)
		}
	}
}
