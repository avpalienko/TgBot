package auth

import (
	"sync"

	"github.com/user/tgbot/internal/logger"
)

// Whitelist manages user access control
type Whitelist struct {
	mu      sync.RWMutex
	allowed map[int64]bool
	log     logger.Logger
}

// NewWhitelist creates a new whitelist with the given user IDs
func NewWhitelist(userIDs []int64, log logger.Logger) *Whitelist {
	w := &Whitelist{
		allowed: make(map[int64]bool),
		log:     log,
	}
	for _, id := range userIDs {
		w.allowed[id] = true
	}
	w.log.Info("whitelist initialized", "users_count", len(userIDs))
	return w
}

// IsAllowed checks if the user ID is in the whitelist.
// Returns true if whitelist is empty (open access) or user is allowed.
func (w *Whitelist) IsAllowed(userID int64) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()

	// If whitelist is empty, allow everyone
	if len(w.allowed) == 0 {
		return true
	}

	allowed := w.allowed[userID]
	if !allowed {
		w.log.Warn("access denied", "user_id", userID)
	}
	return allowed
}

// Add adds a user to the whitelist
func (w *Whitelist) Add(userID int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.allowed[userID] = true
	w.log.Info("user added to whitelist", "user_id", userID)
}

// Remove removes a user from the whitelist
func (w *Whitelist) Remove(userID int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.allowed, userID)
	w.log.Info("user removed from whitelist", "user_id", userID)
}

// Count returns the number of users in the whitelist
func (w *Whitelist) Count() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return len(w.allowed)
}
