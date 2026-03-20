package session

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
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
}

// Manager handles conversation sessions for users
type Manager struct {
	mu         sync.RWMutex
	sessions   map[int64]*Session
	maxHistory int
}

// NewManager creates a new session manager
func NewManager(maxHistory int) *Manager {
	if maxHistory <= 0 {
		maxHistory = 20
	}
	return &Manager{
		sessions:   make(map[int64]*Session),
		maxHistory: maxHistory,
	}
}

// generateSessionID creates a unique session identifier
func generateSessionID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// getOrCreateSession returns existing session or creates a new one
func (m *Manager) getOrCreateSession(userID int64) *Session {
	if session, exists := m.sessions[userID]; exists {
		return session
	}
	session := &Session{
		ID:       generateSessionID(),
		Messages: []Message{},
	}
	m.sessions[userID] = session
	return session
}

// GetSessionID returns the session ID for a user (creates if not exists)
func (m *Manager) GetSessionID(userID int64) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.getOrCreateSession(userID).ID
}

// Get returns the conversation history for a user
func (m *Manager) Get(userID int64) []Message {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, exists := m.sessions[userID]
	if !exists || session == nil {
		return []Message{}
	}

	// Return a copy to avoid race conditions
	result := make([]Message, len(session.Messages))
	copy(result, session.Messages)
	return result
}

// Add appends messages to user's conversation history
func (m *Manager) Add(userID int64, messages ...Message) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session := m.getOrCreateSession(userID)
	session.Messages = append(session.Messages, messages...)

	// Trim to max history (keep last N messages)
	if len(session.Messages) > m.maxHistory {
		session.Messages = session.Messages[len(session.Messages)-m.maxHistory:]
	}
}

// AddWithResponseID appends messages and updates the response ID in a single lock acquisition.
func (m *Manager) AddWithResponseID(userID int64, responseID string, messages ...Message) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session := m.getOrCreateSession(userID)
	session.Messages = append(session.Messages, messages...)

	if len(session.Messages) > m.maxHistory {
		session.Messages = session.Messages[len(session.Messages)-m.maxHistory:]
	}

	session.PreviousResponseID = responseID
}

// GetPreviousResponseID returns the latest stored Responses API response ID.
func (m *Manager) GetPreviousResponseID(userID int64) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, exists := m.sessions[userID]
	if !exists || session == nil {
		return ""
	}

	return session.PreviousResponseID
}

// SetPreviousResponseID updates the latest stored Responses API response ID.
func (m *Manager) SetPreviousResponseID(userID int64, responseID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session := m.getOrCreateSession(userID)
	session.PreviousResponseID = responseID
}

// GetLatestImage returns the most recent image stored in the session.
func (m *Manager) GetLatestImage(userID int64) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, exists := m.sessions[userID]
	if !exists || session == nil {
		return ""
	}

	for i := len(session.Messages) - 1; i >= 0; i-- {
		if session.Messages[i].ImageData != "" {
			return session.Messages[i].ImageData
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
	}
	return newSessionID
}

// Count returns the number of messages in user's history
func (m *Manager) Count(userID int64) int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, exists := m.sessions[userID]
	if !exists || session == nil {
		return 0
	}
	return len(session.Messages)
}
