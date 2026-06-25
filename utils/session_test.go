package utils

import (
	"testing"
	"time"
)

func setupTestStore() *MemorySessionStore {
	store := NewMemorySessionStore(24 * time.Hour)
	return store
}

// TestSessionCreate 测试创建会话
func TestSessionCreate(t *testing.T) {
	store := setupTestStore()
	defer store.Stop()

	sess, err := store.Create(1, "testuser", time.Hour)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if sess.SessionID == "" {
		t.Error("Create() SessionID 为空")
	}
	if sess.UserID != 1 {
		t.Errorf("Create() UserID = %d, want 1", sess.UserID)
	}
	if sess.Username != "testuser" {
		t.Errorf("Create() Username = %s, want testuser", sess.Username)
	}
	if sess.IsExpired() {
		t.Error("Create() 新创建的会话不应过期")
	}
}

// TestSessionGet 测试获取会话
func TestSessionGet(t *testing.T) {
	store := setupTestStore()
	defer store.Stop()

	sess, _ := store.Create(2, "user2", time.Hour)

	// 获取存在的会话
	got, err := store.Get(sess.SessionID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.SessionID != sess.SessionID {
		t.Errorf("Get() SessionID = %s, want %s", got.SessionID, sess.SessionID)
	}

	// 获取不存在的会话
	_, err = store.Get("nonexistent")
	if err != ErrSessionNotFound {
		t.Errorf("Get() 不存在的会话应返回 ErrSessionNotFound, got %v", err)
	}

	// 空 sessionID
	_, err = store.Get("")
	if err != ErrSessionNotFound {
		t.Errorf("Get() 空 sessionID 应返回 ErrSessionNotFound, got %v", err)
	}
}

// TestSessionExpired 测试过期会话
func TestSessionExpired(t *testing.T) {
	store := NewMemorySessionStore(1 * time.Millisecond)
	defer store.Stop()

	sess, _ := store.Create(3, "user3", 1*time.Millisecond)
	time.Sleep(10 * time.Millisecond)

	if !sess.IsExpired() {
		t.Error("过期会话 IsExpired() 应为 true")
	}

	_, err := store.Get(sess.SessionID)
	if err != ErrSessionNotFound {
		t.Errorf("Get() 过期会话应返回 ErrSessionNotFound, got %v", err)
	}
}

// TestSessionTouch 测试刷新会话
func TestSessionTouch(t *testing.T) {
	store := NewMemorySessionStore(1 * time.Hour)
	defer store.Stop()

	sess, _ := store.Create(4, "user4", 100*time.Millisecond)
	time.Sleep(50 * time.Millisecond)

	// 刷新
	if err := store.Touch(sess.SessionID); err != nil {
		t.Fatalf("Touch() error = %v", err)
	}

	// 刷新后应仍有效
	got, err := store.Get(sess.SessionID)
	if err != nil {
		t.Fatalf("Get() after Touch error = %v", err)
	}
	if got.IsExpired() {
		t.Error("Touch() 后会话不应过期")
	}

	// 刷新不存在的会话
	err = store.Touch("nonexistent")
	if err != ErrSessionNotFound {
		t.Errorf("Touch() 不存在的会话应返回 ErrSessionNotFound, got %v", err)
	}
}

// TestSessionDelete 测试删除会话
func TestSessionDelete(t *testing.T) {
	store := setupTestStore()
	defer store.Stop()

	sess, _ := store.Create(5, "user5", time.Hour)

	// 删除
	if err := store.Delete(sess.SessionID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// 删除后获取应失败
	_, err := store.Get(sess.SessionID)
	if err != ErrSessionNotFound {
		t.Errorf("Get() 删除后应返回 ErrSessionNotFound, got %v", err)
	}

	// 删除不存在的会话
	err = store.Delete("nonexistent")
	if err != ErrSessionNotFound {
		t.Errorf("Delete() 不存在的会话应返回 ErrSessionNotFound, got %v", err)
	}
}

// TestSessionMultipleUsers 测试多用户会话隔离
func TestSessionMultipleUsers(t *testing.T) {
	store := setupTestStore()
	defer store.Stop()

	sess1, _ := store.Create(10, "userA", time.Hour)
	sess2, _ := store.Create(20, "userB", time.Hour)

	if sess1.SessionID == sess2.SessionID {
		t.Error("不同用户的 SessionID 应不同")
	}

	got1, _ := store.Get(sess1.SessionID)
	got2, _ := store.Get(sess2.SessionID)

	if got1.UserID != 10 || got1.Username != "userA" {
		t.Error("用户A会话数据错误")
	}
	if got2.UserID != 20 || got2.Username != "userB" {
		t.Error("用户B会话数据错误")
	}
}

// TestSessionDefaultTTL 测试默认 TTL
func TestSessionDefaultTTL(t *testing.T) {
	store := NewMemorySessionStore(0) // 0 应默认 24h
	defer store.Stop()

	sess, _ := store.Create(6, "user6", 0)
	if sess.IsExpired() {
		t.Error("默认 TTL 下新会话不应过期")
	}
}

// TestSessionIsExpired 测试 IsExpired 方法
func TestSessionIsExpired(t *testing.T) {
	future := time.Now().Add(time.Hour)
	past := time.Now().Add(-time.Hour)

	sessFuture := &Session{ExpireAt: future}
	sessPast := &Session{ExpireAt: past}

	if sessFuture.IsExpired() {
		t.Error("未来过期的会话 IsExpired() 应为 false")
	}
	if !sessPast.IsExpired() {
		t.Error("已过期的会话 IsExpired() 应为 true")
	}
}