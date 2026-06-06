package utils

import (
	"errors"
	"sync"
	"time"
)

// Session 内存中的会话信息
type Session struct {
	SessionID  string                 // 会话标识
	UserID     uint                   // 关联用户ID
	Username   string                 // 关联用户账号
	Data       map[string]interface{} // 自定义业务数据，预留给业务扩展
	CreatedAt  time.Time              // 创建时间
	UpdatedAt  time.Time              // 最近活跃时间
	ExpireAt   time.Time              // 过期时间
}

// IsExpired 判断会话是否已过期
func (s *Session) IsExpired() bool {
	return time.Now().After(s.ExpireAt)
}

// SessionStore 会话存储接口
// 当前使用内存实现，后续可接入 Redis 等分布式存储
type SessionStore interface {
	Create(userID uint, username string, ttl time.Duration) (*Session, error)
	Get(sessionID string) (*Session, error)
	Touch(sessionID string) error // 刷新活跃时间
	Delete(sessionID string) error
}

// ErrSessionNotFound 会话不存在
var ErrSessionNotFound = errors.New("session not found")

// MemorySessionStore 基于内存的会话存储实现（单实例）
// 预留 Redis 接口：后续只需新增 RedisSessionStore 实现 SessionStore
type MemorySessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	stop     chan struct{}
	// defaultTTL 默认会话有效期
	defaultTTL time.Duration
}

// NewMemorySessionStore 创建内存会话存储
func NewMemorySessionStore(defaultTTL time.Duration) *MemorySessionStore {
	if defaultTTL <= 0 {
		defaultTTL = 24 * time.Hour
	}
	store := &MemorySessionStore{
		sessions:   make(map[string]*Session),
		stop:       make(chan struct{}),
		defaultTTL: defaultTTL,
	}
	// 启动后台清理 goroutine
	go store.gcLoop()
	return store
}

// Create 创建会话并返回 Session 实例
func (m *MemorySessionStore) Create(userID uint, username string, ttl time.Duration) (*Session, error) {
	if ttl <= 0 {
		ttl = m.defaultTTL
	}
	token, err := GenerateSalt() // 32 字节随机串，足够作为 sessionID
	if err != nil {
		return nil, err
	}
	now := time.Now()
	sess := &Session{
		SessionID: token,
		UserID:    userID,
		Username:  username,
		Data:      make(map[string]interface{}),
		CreatedAt: now,
		UpdatedAt: now,
		ExpireAt:  now.Add(ttl),
	}
	m.mu.Lock()
	m.sessions[token] = sess
	m.mu.Unlock()
	return sess, nil
}

// Get 获取会话，不存在或已过期则返回 ErrSessionNotFound
func (m *MemorySessionStore) Get(sessionID string) (*Session, error) {
	if sessionID == "" {
		return nil, ErrSessionNotFound
	}
	m.mu.RLock()
	sess, ok := m.sessions[sessionID]
	m.mu.RUnlock()
	if !ok {
		return nil, ErrSessionNotFound
	}
	if sess.IsExpired() {
		m.Delete(sessionID)
		return nil, ErrSessionNotFound
	}
	return sess, nil
}

// Touch 刷新会话活跃时间与过期时间
func (m *MemorySessionStore) Touch(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	sess, ok := m.sessions[sessionID]
	if !ok {
		return ErrSessionNotFound
	}
	now := time.Now()
	sess.UpdatedAt = now
	sess.ExpireAt = now.Add(m.defaultTTL)
	return nil
}

// Delete 删除会话
func (m *MemorySessionStore) Delete(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sessions[sessionID]; !ok {
		return ErrSessionNotFound
	}
	delete(m.sessions, sessionID)
	return nil
}

// Stop 停止后台清理 goroutine
func (m *MemorySessionStore) Stop() {
	close(m.stop)
}

// gcLoop 定期清理已过期会话
func (m *MemorySessionStore) gcLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-m.stop:
			return
		case <-ticker.C:
			now := time.Now()
			m.mu.Lock()
			for id, s := range m.sessions {
				if now.After(s.ExpireAt) {
					delete(m.sessions, id)
				}
			}
			m.mu.Unlock()
		}
	}
}
