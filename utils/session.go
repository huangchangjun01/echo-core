package utils

import (
	"errors"
	"log"
	"sync"
	"time"
)

// Session 内存中的会话信息
type Session struct {
	SessionID string                 // 会话标识
	UserID    uint                   // 关联用户ID
	Username  string                 // 关联用户账号
	Data      map[string]interface{} // 自定义业务数据，预留给业务扩展
	CreatedAt time.Time              // 创建时间
	UpdatedAt time.Time              // 最近活跃时间
	ExpireAt  time.Time              // 过期时间
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

// Stats 调试用：返回当前会话总数（gc 也会调，但读锁安全）
func (m *MemorySessionStore) Stats() (total int) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}

// Create 创建会话并返回 Session 实例
func (m *MemorySessionStore) Create(userID uint, username string, ttl time.Duration) (*Session, error) {
	if ttl <= 0 {
		ttl = m.defaultTTL
	}
	token, err := GenerateSalt() // 32 字节随机串，足够作为 sessionID
	if err != nil {
		log.Printf("[SessionStore.Create] 生成 token 失败 | userID=%d | err=%v", userID, err)
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
	total := len(m.sessions)
	m.mu.Unlock()
	log.Printf("[SessionStore.Create] ok | userID=%d username=%s ttl=%v total=%d", userID, username, ttl, total)
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
		log.Printf("[SessionStore.Get] miss | sid=%s", truncateSID(sessionID))
		return nil, ErrSessionNotFound
	}
	if sess.IsExpired() {
		m.Delete(sessionID)
		log.Printf("[SessionStore.Get] expired | sid=%s userID=%d", truncateSID(sessionID), sess.UserID)
		return nil, ErrSessionNotFound
	}
	log.Printf("[SessionStore.Get] hit | sid=%s userID=%d username=%s expireIn=%v", truncateSID(sessionID), sess.UserID, sess.Username, time.Until(sess.ExpireAt))
	return sess, nil
}

// Touch 刷新会话活跃时间与过期时间
func (m *MemorySessionStore) Touch(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	sess, ok := m.sessions[sessionID]
	if !ok {
		log.Printf("[SessionStore.Touch] miss | sid=%s", truncateSID(sessionID))
		return ErrSessionNotFound
	}
	now := time.Now()
	sess.UpdatedAt = now
	sess.ExpireAt = now.Add(m.defaultTTL)
	log.Printf("[SessionStore.Touch] ok | sid=%s userID=%d newExpireIn=%v", truncateSID(sessionID), sess.UserID, m.defaultTTL)
	return nil
}

// Delete 删除会话
func (m *MemorySessionStore) Delete(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	sess, existed := m.sessions[sessionID]
	if !existed {
		log.Printf("[SessionStore.Delete] miss | sid=%s", truncateSID(sessionID))
		return ErrSessionNotFound
	}
	delete(m.sessions, sessionID)
	total := len(m.sessions)
	log.Printf("[SessionStore.Delete] ok | sid=%s userID=%d total=%d", truncateSID(sessionID), sess.UserID, total)
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
			removed := 0
			for id, s := range m.sessions {
				if now.After(s.ExpireAt) {
					delete(m.sessions, id)
					removed++
				}
			}
			remaining := len(m.sessions)
			m.mu.Unlock()
			if removed > 0 {
				log.Printf("[SessionStore.gc] 清理过期会话 | removed=%d remaining=%d", removed, remaining)
			}
		}
	}
}

// truncateSID 日志里避免完整 sid（64 字符 hex）打满：截前 8 字符 + 长度。
func truncateSID(sid string) string {
	const prefix = 8
	if len(sid) <= prefix {
		return sid
	}
	return sid[:prefix] + "...(len=" + itoa(len(sid)) + ")"
}

// itoa 避免引入 strconv 的小依赖
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
