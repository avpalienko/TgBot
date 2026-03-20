package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/user/tgbot/internal/logger"
)

// Message represents a chat message
type Message struct {
	Role      string // "user" or "assistant"
	Content   string
	ImageData string // optional: base64 data URI for images
}

// Session holds conversation data for a user
type Session struct {
	ID                 string    // Unique session ID for log tracing
	Messages           []Message // Conversation history
	PreviousResponseID string
	LastActivity       time.Time
}

// Manager handles conversation sessions for users
type Manager struct {
	mu         sync.RWMutex
	sessions   map[int64]*Session
	maxHistory int
	sessionTTL time.Duration
}

// NewManager creates a new session manager.
// A sessionTTL <= 0 disables automatic eviction.
func NewManager(maxHistory int, sessionTTL time.Duration) *Manager {
	if maxHistory <= 0 {
		maxHistory = 20
	}
	return &Manager{
		sessions:   make(map[int64]*Session),
		maxHistory: maxHistory,
		sessionTTL: sessionTTL,
	}
}

// generateSessionID creates a unique session identifier.
// Falls back to a timestamp-based ID if the crypto/rand source fails.
func generateSessionID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func (m *Manager) touch(s *Session) {
	s.LastActivity = time.Now()
}

// getOrCreateSession returns existing session or creates a new one
func (m *Manager) getOrCreateSession(userID int64) *Session {
	if s, exists := m.sessions[userID]; exists {
		return s
	}
	s := &Session{
		ID:           generateSessionID(),
		Messages:     []Message{},
		LastActivity: time.Now(),
	}
	m.sessions[userID] = s
	return s
}

// GetSessionID returns the session ID for a user (creates if not exists)
func (m *Manager) GetSessionID(userID int64) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.getOrCreateSession(userID)
	m.touch(s)
	return s.ID
}

// Get returns the conversation history for a user
func (m *Manager) Get(userID int64) []Message {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, exists := m.sessions[userID]
	if !exists || s == nil {
		return []Message{}
	}

	m.touch(s)

	result := make([]Message, len(s.Messages))
	copy(result, s.Messages)
	return result
}

// AddWithResponseID appends messages and updates the response ID in a single lock acquisition.
func (m *Manager) AddWithResponseID(userID int64, responseID string, messages ...Message) {
	m.mu.Lock()
	defer m.mu.Unlock()

	s := m.getOrCreateSession(userID)
	s.Messages = append(s.Messages, messages...)

	if len(s.Messages) > m.maxHistory {
		s.Messages = s.Messages[len(s.Messages)-m.maxHistory:]
	}

	s.PreviousResponseID = responseID
	m.touch(s)
}

// GetPreviousResponseID returns the latest stored Responses API response ID.
func (m *Manager) GetPreviousResponseID(userID int64) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, exists := m.sessions[userID]
	if !exists || s == nil {
		return ""
	}

	m.touch(s)
	return s.PreviousResponseID
}

// GetLatestImage returns the most recent image stored in the session.
func (m *Manager) GetLatestImage(userID int64) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, exists := m.sessions[userID]
	if !exists || s == nil {
		return ""
	}

	m.touch(s)

	for i := len(s.Messages) - 1; i >= 0; i-- {
		if s.Messages[i].ImageData != "" {
			return s.Messages[i].ImageData
		}
	}

	return ""
}

// Clear removes all messages for a user and generates new session ID
func (m *Manager) Clear(userID int64) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	newSessionID := generateSessionID()
	m.sessions[userID] = &Session{
		ID:                 newSessionID,
		Messages:           []Message{},
		PreviousResponseID: "",
		LastActivity:       time.Now(),
	}
	return newSessionID
}

// SessionCount returns the number of active sessions (for testing/monitoring).
func (m *Manager) SessionCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}

// StartCleanup launches a background goroutine that periodically evicts
// sessions whose LastActivity is older than sessionTTL.
// Does nothing if sessionTTL <= 0. Stops when ctx is cancelled.
func (m *Manager) StartCleanup(ctx context.Context, log logger.Logger) {
	if m.sessionTTL <= 0 {
		return
	}

	interval := m.sessionTTL / 2
	if interval > time.Hour {
		interval = time.Hour
	}

	log.Info("session cleanup started", "ttl", m.sessionTTL, "interval", interval)

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				log.Debug("session cleanup stopped")
				return
			case <-ticker.C:
				m.evictExpired(log)
			}
		}
	}()
}

func (m *Manager) evictExpired(log logger.Logger) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	evicted := 0
	for uid, s := range m.sessions {
		if now.Sub(s.LastActivity) > m.sessionTTL {
			delete(m.sessions, uid)
			evicted++
		}
	}

	if evicted > 0 {
		log.Debug("expired sessions evicted", "count", evicted, "remaining", len(m.sessions))
	}
}
