package session

import (
	"sync"
)

// Message represents a chat message
type Message struct {
	Role    string // "user" or "assistant"
	Content string
}

// Manager handles conversation sessions for users
type Manager struct {
	mu         sync.RWMutex
	sessions   map[int64][]Message
	maxHistory int
}

// NewManager creates a new session manager
func NewManager(maxHistory int) *Manager {
	if maxHistory <= 0 {
		maxHistory = 20
	}
	return &Manager{
		sessions:   make(map[int64][]Message),
		maxHistory: maxHistory,
	}
}

// Get returns the conversation history for a user
func (m *Manager) Get(userID int64) []Message {
	m.mu.RLock()
	defer m.mu.RUnlock()

	messages := m.sessions[userID]
	if messages == nil {
		return []Message{}
	}

	// Return a copy to avoid race conditions
	result := make([]Message, len(messages))
	copy(result, messages)
	return result
}

// Add appends messages to user's conversation history
func (m *Manager) Add(userID int64, messages ...Message) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.sessions[userID] = append(m.sessions[userID], messages...)

	// Trim to max history (keep last N messages)
	if len(m.sessions[userID]) > m.maxHistory {
		m.sessions[userID] = m.sessions[userID][len(m.sessions[userID])-m.maxHistory:]
	}
}

// Clear removes all messages for a user
func (m *Manager) Clear(userID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, userID)
}

// Count returns the number of messages in user's history
func (m *Manager) Count(userID int64) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions[userID])
}
