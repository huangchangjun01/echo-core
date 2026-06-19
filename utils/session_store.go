package utils

import (
	"log"
	"sync"
	"time"
)

// 全局 SessionStore 单例。
//
// 历史 Bug：早期 service/user_service.go 在 NewUserService() 内部
// 直接构造一个 *MemorySessionStore，导致每次 handler / 中间件
// 拿到的都是独立 store，跨请求的 session 一律失效。
//
// 修复：把 store 提到 utils 包作为进程级单例；user_service 和后续
// 鉴权中间件都从这里拿同一份实例。Redis 实现替换时只需改本文件。
var (
	sessionStoreOnce sync.Once
	sessionStore     *MemorySessionStore
	// sessionStoreTTL 全局默认 TTL；如需调整请在 main.go 启动早期
	// 通过 initSessionStore(ttl) 显式设置，或保留默认 24h。
	sessionStoreTTL = 24 * time.Hour
)

// InitSessionStore 显式初始化（可选；不调则用默认 24h）。
// 应在 main.go 的早期阶段调用一次。
func InitSessionStore(ttl time.Duration) {
	sessionStoreOnce.Do(func() {
		sessionStoreTTL = ttl
		sessionStore = NewMemorySessionStore(ttl)
		log.Printf("[SessionStore] 初始化全局 SessionStore | ttl: %v", ttl)
	})
}

// GetSessionStore 获取全局 SessionStore 单例。
// 若尚未 InitSessionStore，则懒加载默认 24h 实例。
func GetSessionStore() *MemorySessionStore {
	sessionStoreOnce.Do(func() {
		sessionStore = NewMemorySessionStore(sessionStoreTTL)
		log.Printf("[SessionStore] 懒加载全局 SessionStore | ttl: %v", sessionStoreTTL)
	})
	return sessionStore
}

// StopSessionStore 停止后台清理 goroutine，仅在进程退出前调用。
func StopSessionStore() {
	if sessionStore != nil {
		sessionStore.Stop()
	}
}

// SetSessionStoreForTest 测试钩子：替换为指定的 store（例如 mock）。
// 仅测试代码使用；生产代码请勿调用。
func SetSessionStoreForTest(s *MemorySessionStore) {
	sessionStoreOnce = sync.Once{}
	sessionStore = s
}