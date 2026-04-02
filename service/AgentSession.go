package service

import (
	"sync"

	"github.com/sashabaranov/go-openai"
)

// Session 结构用于保存用户的多轮对话记录
type Session struct {
	SessionID string
	Messages  []openai.ChatCompletionMessage
}

// SessionManager 用于管理所有活跃的会话。利用内存做简单的Session保持。
type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*Session),
	}
}

func (m *SessionManager) GetSession(sessionID string) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()

	if session, ok := m.sessions[sessionID]; ok {
		return session
	}

	session := &Session{
		SessionID: sessionID,
		Messages:  make([]openai.ChatCompletionMessage, 0),
	}
	m.sessions[sessionID] = session
	return session
}

func (m *SessionManager) AddMessage(sessionID string, msg openai.ChatCompletionMessage) {
	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.sessions[sessionID]
	if !ok {
		return
	}
	session.Messages = append(session.Messages, msg)
}

func (m *SessionManager) ClearSession(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, sessionID)
}
