package session

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/user/tgbot/internal/logger"
)

func TestNewManager(t *testing.T) {
	t.Parallel()

	t.Run("positive maxHistory", func(t *testing.T) {
		t.Parallel()
		m := NewManager(10, 0)
		if m.maxHistory != 10 {
			t.Fatalf("expected maxHistory=10, got %d", m.maxHistory)
		}
	})

	t.Run("zero defaults to 20", func(t *testing.T) {
		t.Parallel()
		m := NewManager(0, 0)
		if m.maxHistory != 20 {
			t.Fatalf("expected maxHistory=20, got %d", m.maxHistory)
		}
	})

	t.Run("negative defaults to 20", func(t *testing.T) {
		t.Parallel()
		m := NewManager(-5, 0)
		if m.maxHistory != 20 {
			t.Fatalf("expected maxHistory=20, got %d", m.maxHistory)
		}
	})

	t.Run("stores sessionTTL", func(t *testing.T) {
		t.Parallel()
		m := NewManager(10, 5*time.Hour)
		if m.sessionTTL != 5*time.Hour {
			t.Fatalf("expected sessionTTL=5h, got %v", m.sessionTTL)
		}
	})
}

func TestGetSessionID(t *testing.T) {
	t.Parallel()

	m := NewManager(10, 0)

	id1 := m.GetSessionID(1)
	if id1 == "" {
		t.Fatalf("session ID should not be empty")
	}

	id2 := m.GetSessionID(1)
	if id1 != id2 {
		t.Fatalf("same user should get same session ID, got %q and %q", id1, id2)
	}

	id3 := m.GetSessionID(2)
	if id1 == id3 {
		t.Fatalf("different users should get different session IDs")
	}
}

func TestGet(t *testing.T) {
	t.Parallel()

	t.Run("empty history for new user", func(t *testing.T) {
		t.Parallel()
		m := NewManager(10, 0)
		msgs := m.Get(99)
		if len(msgs) != 0 {
			t.Fatalf("expected empty history, got %d messages", len(msgs))
		}
	})

	t.Run("returns copy not pointer to internal slice", func(t *testing.T) {
		t.Parallel()
		m := NewManager(10, 0)
		m.AddWithResponseID(1, "", Message{Role: "user", Content: "hello"})

		msgs := m.Get(1)
		msgs[0].Content = "tampered"

		original := m.Get(1)
		if original[0].Content == "tampered" {
			t.Fatalf("Get should return a copy, not a reference to internal data")
		}
	})
}

func TestAddWithResponseID(t *testing.T) {
	t.Parallel()

	t.Run("adds messages", func(t *testing.T) {
		t.Parallel()
		m := NewManager(10, 0)
		m.AddWithResponseID(1, "resp-1",
			Message{Role: "user", Content: "hi"},
			Message{Role: "assistant", Content: "hello"},
		)
		msgs := m.Get(1)
		if len(msgs) != 2 {
			t.Fatalf("expected 2 messages, got %d", len(msgs))
		}
		if msgs[0].Content != "hi" || msgs[1].Content != "hello" {
			t.Fatalf("unexpected message content: %+v", msgs)
		}
	})

	t.Run("trims to maxHistory", func(t *testing.T) {
		t.Parallel()
		m := NewManager(3, 0)
		for i := 0; i < 5; i++ {
			m.AddWithResponseID(1, "", Message{Role: "user", Content: "msg"})
		}
		msgs := m.Get(1)
		if len(msgs) != 3 {
			t.Fatalf("expected 3 messages after trim, got %d", len(msgs))
		}
	})

	t.Run("stores response ID", func(t *testing.T) {
		t.Parallel()
		m := NewManager(10, 0)
		m.AddWithResponseID(1, "resp-abc", Message{Role: "user", Content: "hi"})
		if got := m.GetPreviousResponseID(1); got != "resp-abc" {
			t.Fatalf("expected response ID %q, got %q", "resp-abc", got)
		}
	})
}

func TestGetPreviousResponseID(t *testing.T) {
	t.Parallel()

	t.Run("empty for unknown user", func(t *testing.T) {
		t.Parallel()
		m := NewManager(10, 0)
		if got := m.GetPreviousResponseID(999); got != "" {
			t.Fatalf("expected empty response ID, got %q", got)
		}
	})

	t.Run("returns stored ID", func(t *testing.T) {
		t.Parallel()
		m := NewManager(10, 0)
		m.AddWithResponseID(1, "resp-xyz", Message{Role: "user", Content: "hi"})
		if got := m.GetPreviousResponseID(1); got != "resp-xyz" {
			t.Fatalf("expected %q, got %q", "resp-xyz", got)
		}
	})
}

func TestGetLatestImage(t *testing.T) {
	t.Parallel()

	t.Run("no image returns empty", func(t *testing.T) {
		t.Parallel()
		m := NewManager(10, 0)
		m.AddWithResponseID(1, "", Message{Role: "user", Content: "hi"})
		if got := m.GetLatestImage(1); got != "" {
			t.Fatalf("expected empty, got %q", got)
		}
	})

	t.Run("unknown user returns empty", func(t *testing.T) {
		t.Parallel()
		m := NewManager(10, 0)
		if got := m.GetLatestImage(999); got != "" {
			t.Fatalf("expected empty, got %q", got)
		}
	})

	t.Run("finds most recent image", func(t *testing.T) {
		t.Parallel()
		m := NewManager(10, 0)
		m.AddWithResponseID(1, "",
			Message{Role: "user", Content: "first", ImageData: "data:image/png;base64,AAA"},
			Message{Role: "user", Content: "second"},
			Message{Role: "assistant", Content: "third", ImageData: "data:image/png;base64,BBB"},
		)
		if got := m.GetLatestImage(1); got != "data:image/png;base64,BBB" {
			t.Fatalf("expected latest image BBB, got %q", got)
		}
	})
}

func TestClear(t *testing.T) {
	t.Parallel()

	m := NewManager(10, 0)
	m.AddWithResponseID(1, "resp-old",
		Message{Role: "user", Content: "hello"},
		Message{Role: "assistant", Content: "hi"},
	)

	oldID := m.GetSessionID(1)
	newID := m.Clear(1)

	if oldID == newID {
		t.Fatalf("Clear should generate a new session ID")
	}
	if msgs := m.Get(1); len(msgs) != 0 {
		t.Fatalf("expected empty messages after Clear, got %d", len(msgs))
	}
	if got := m.GetPreviousResponseID(1); got != "" {
		t.Fatalf("expected empty response ID after Clear, got %q", got)
	}
	if got := m.GetSessionID(1); got != newID {
		t.Fatalf("GetSessionID after Clear should return new ID %q, got %q", newID, got)
	}
}

func TestSessionConcurrency(t *testing.T) {
	t.Parallel()

	m := NewManager(50, 0)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		uid := int64(i % 5)
		wg.Add(4)
		go func() {
			defer wg.Done()
			m.Get(uid)
		}()
		go func() {
			defer wg.Done()
			m.AddWithResponseID(uid, "resp", Message{Role: "user", Content: "msg"})
		}()
		go func() {
			defer wg.Done()
			m.GetPreviousResponseID(uid)
		}()
		go func() {
			defer wg.Done()
			m.GetLatestImage(uid)
		}()
	}
	wg.Wait()
}

func testLogger() logger.Logger {
	return logger.New(logger.Config{Level: "debug", Format: "text"})
}

func TestCleanupEvictsExpired(t *testing.T) {
	t.Parallel()

	ttl := 100 * time.Millisecond
	m := NewManager(10, ttl)

	m.AddWithResponseID(1, "", Message{Role: "user", Content: "a"})
	m.AddWithResponseID(2, "", Message{Role: "user", Content: "b"})

	if m.SessionCount() != 2 {
		t.Fatalf("expected 2 sessions, got %d", m.SessionCount())
	}

	time.Sleep(ttl + 50*time.Millisecond)

	m.evictExpired(testLogger())

	if m.SessionCount() != 0 {
		t.Fatalf("expected 0 sessions after eviction, got %d", m.SessionCount())
	}
}

func TestCleanupPreservesActive(t *testing.T) {
	t.Parallel()

	ttl := 150 * time.Millisecond
	m := NewManager(10, ttl)

	m.AddWithResponseID(1, "", Message{Role: "user", Content: "active"})
	m.AddWithResponseID(2, "", Message{Role: "user", Content: "stale"})

	time.Sleep(100 * time.Millisecond)

	// Touch user 1 to keep it alive
	m.Get(1)

	time.Sleep(80 * time.Millisecond)

	m.evictExpired(testLogger())

	if m.SessionCount() != 1 {
		t.Fatalf("expected 1 session (active one preserved), got %d", m.SessionCount())
	}

	msgs := m.Get(1)
	if len(msgs) != 1 || msgs[0].Content != "active" {
		t.Fatalf("expected active user's session to survive, got %+v", msgs)
	}

	if msgs := m.Get(2); len(msgs) != 0 {
		t.Fatalf("expected stale user's session to be evicted")
	}
}

func TestLastActivityUpdatedOnAccess(t *testing.T) {
	t.Parallel()

	m := NewManager(10, time.Hour)
	m.AddWithResponseID(1, "", Message{Role: "user", Content: "hi", ImageData: "data:image/png;base64,X"})

	getActivity := func() time.Time {
		m.mu.RLock()
		defer m.mu.RUnlock()
		return m.sessions[1].LastActivity
	}

	methods := []struct {
		name string
		fn   func()
	}{
		{"GetSessionID", func() { m.GetSessionID(1) }},
		{"Get", func() { m.Get(1) }},
		{"AddWithResponseID", func() { m.AddWithResponseID(1, "", Message{Role: "user", Content: "x"}) }},
		{"GetPreviousResponseID", func() { m.GetPreviousResponseID(1) }},
		{"GetLatestImage", func() { m.GetLatestImage(1) }},
	}

	for _, mt := range methods {
		before := getActivity()
		time.Sleep(2 * time.Millisecond)
		mt.fn()
		after := getActivity()
		if !after.After(before) {
			t.Errorf("%s did not update LastActivity", mt.name)
		}
	}
}

func TestStartCleanupStopsOnCancel(t *testing.T) {
	t.Parallel()

	m := NewManager(10, 50*time.Millisecond)
	m.AddWithResponseID(1, "", Message{Role: "user", Content: "hi"})

	ctx, cancel := context.WithCancel(context.Background())
	m.StartCleanup(ctx, testLogger())

	// Let the ticker fire at least once
	time.Sleep(100 * time.Millisecond)

	if m.SessionCount() != 0 {
		t.Fatalf("expected session to be evicted, got %d", m.SessionCount())
	}

	cancel()
	// Give goroutine time to exit
	time.Sleep(20 * time.Millisecond)

	// After cancel, adding a new session should not be evicted
	m.AddWithResponseID(2, "", Message{Role: "user", Content: "new"})
	time.Sleep(100 * time.Millisecond)

	if m.SessionCount() != 1 {
		t.Fatalf("cleanup should have stopped; expected 1 session, got %d", m.SessionCount())
	}
}

func TestStartCleanupNoopWhenTTLZero(t *testing.T) {
	t.Parallel()

	m := NewManager(10, 0)
	m.AddWithResponseID(1, "", Message{Role: "user", Content: "hi"})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m.StartCleanup(ctx, testLogger())

	time.Sleep(50 * time.Millisecond)

	if m.SessionCount() != 1 {
		t.Fatalf("TTL=0 should disable cleanup; expected 1 session, got %d", m.SessionCount())
	}
}
