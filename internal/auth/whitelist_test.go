package auth

import (
    "io"
    "sync"
    "testing"

    "github.com/user/tgbot/internal/logger"
)

func testLogger() logger.Logger {
    return logger.New(logger.Config{Level: "error", Output: io.Discard})
}

func TestNewWhitelist(t *testing.T) {
    t.Parallel()

    t.Run("empty user list", func(t *testing.T) {
        t.Parallel()
        w := NewWhitelist(nil, testLogger())
        if w.Count() != 0 {
            t.Fatalf("expected 0 users, got %d", w.Count())
        }
    })

    t.Run("non-empty user list", func(t *testing.T) {
        t.Parallel()
        w := NewWhitelist([]int64{100, 200, 300}, testLogger())
        if w.Count() != 3 {
            t.Fatalf("expected 3 users, got %d", w.Count())
        }
    })
}

func TestIsAllowed(t *testing.T) {
    t.Parallel()

    t.Run("allowed user", func(t *testing.T) {
        t.Parallel()
        w := NewWhitelist([]int64{42, 99}, testLogger())
        if !w.IsAllowed(42) {
            t.Fatalf("user 42 should be allowed")
        }
    })

    t.Run("denied user", func(t *testing.T) {
        t.Parallel()
        w := NewWhitelist([]int64{42, 99}, testLogger())
        if w.IsAllowed(1) {
            t.Fatalf("user 1 should be denied")
        }
    })

    t.Run("empty whitelist allows everyone", func(t *testing.T) {
        t.Parallel()
        w := NewWhitelist(nil, testLogger())
        if !w.IsAllowed(12345) {
            t.Fatalf("empty whitelist should allow everyone")
        }
    })
}

func TestAddRemove(t *testing.T) {
    t.Parallel()

    w := NewWhitelist(nil, testLogger())

    w.Add(10)
    if !w.IsAllowed(10) {
        t.Fatalf("user 10 should be allowed after Add")
    }

    w.Remove(10)
    // After removing the only user the whitelist is empty again -> open access.
    // Add a different user so the whitelist is non-empty, then verify 10 is denied.
    w.Add(20)
    if w.IsAllowed(10) {
        t.Fatalf("user 10 should be denied after Remove")
    }
}

func TestCount(t *testing.T) {
    t.Parallel()

    w := NewWhitelist([]int64{1, 2}, testLogger())
    if w.Count() != 2 {
        t.Fatalf("expected 2, got %d", w.Count())
    }

    w.Add(3)
    if w.Count() != 3 {
        t.Fatalf("expected 3 after Add, got %d", w.Count())
    }

    w.Remove(1)
    if w.Count() != 2 {
        t.Fatalf("expected 2 after Remove, got %d", w.Count())
    }
}

func TestWhitelistConcurrency(t *testing.T) {
    t.Parallel()

    w := NewWhitelist([]int64{1, 2, 3}, testLogger())

    var wg sync.WaitGroup
    for i := 0; i < 100; i++ {
        wg.Add(3)
        id := int64(i)
        go func() {
            defer wg.Done()
            w.IsAllowed(id)
        }()
        go func() {
            defer wg.Done()
            w.Add(id + 1000)
        }()
        go func() {
            defer wg.Done()
            w.Remove(id + 1000)
        }()
    }
    wg.Wait()
}
