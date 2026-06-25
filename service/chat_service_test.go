package service

import (
	"echo-core/config"
	"echo-core/models"
	"echo-core/remote"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"log"
)

func setupChatServiceTestDB() func() {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.New(log.New(log.Writer(), "", 0), logger.Config{LogLevel: logger.Silent}),
	})
	if err != nil {
		panic("内存数据库初始化失败: " + err.Error())
	}
	if err := db.AutoMigrate(&models.SessionMessage{}); err != nil {
		panic("迁移失败: " + err.Error())
	}
	config.SetDBForTest(db)
	return func() {
		config.SetDBForTest(nil)
	}
}

// TestChatStreamInvalidParams 测试聊天流参数校验
func TestChatStreamInvalidParams(t *testing.T) {
	cleanup := setupChatServiceTestDB()
	defer cleanup()

	svc := NewChatService(remote.NewPythonClient("http://localhost:8000"))

	tests := []struct {
		name    string
		req     ChatRequest
		wantErr bool
	}{
		{"空 UserID", ChatRequest{UserID: "", SessionID: "s1", Message: "hi"}, true},
		{"空 SessionID", ChatRequest{UserID: "1", SessionID: "", Message: "hi"}, true},
		{"空 Message", ChatRequest{UserID: "1", SessionID: "s1", Message: ""}, true},
		{"正常请求", ChatRequest{UserID: "1", SessionID: "s1", Message: "hi"}, true}, // Python不可达，但参数校验通过
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			done := false
			var lastErr error
			svc.ChatStream(tt.req, func(chunk StreamChunk) {
				if chunk.Done {
					done = true
					lastErr = chunk.Err
				}
			})
			if !done {
				t.Error("ChatStream 应回调 Done")
			}
			if tt.wantErr && lastErr == nil {
				t.Error("应返回错误")
			}
		})
	}
}

// TestChatStreamSavesUserMessage 测试聊天流保存用户消息
func TestChatStreamSavesUserMessage(t *testing.T) {
	cleanup := setupChatServiceTestDB()
	defer cleanup()

	svc := NewChatService(remote.NewPythonClient("http://localhost:8000"))

	req := ChatRequest{
		UserID:    "user1",
		SessionID: "session-test",
		Message:   "你好世界",
	}

	done := false
	svc.ChatStream(req, func(chunk StreamChunk) {
		if chunk.Done {
			done = true
		}
	})
	if !done {
		t.Fatal("ChatStream 未完成")
	}

	// 验证用户消息已保存
	history, err := svc.GetHistory("session-test", "user1", 50)
	if err != nil {
		t.Fatalf("GetHistory error: %v", err)
	}
	if len(history) == 0 {
		t.Fatal("应至少有一条用户消息")
	}
	found := false
	for _, m := range history {
		if m.Role == "user" && m.Content == "你好世界" {
			found = true
			break
		}
	}
	if !found {
		t.Error("未找到用户消息")
	}
}

// TestGetHistory 测试获取历史
func TestGetHistory(t *testing.T) {
	cleanup := setupChatServiceTestDB()
	defer cleanup()

	svc := NewChatService(remote.NewPythonClient("http://localhost:8000"))

	// 插入多条消息
	msgs := []models.SessionMessage{
		{SessionID: "s1", UserID: "u1", Role: "user", Content: "msg1", CreatedAt: time.Now()},
		{SessionID: "s1", UserID: "u1", Role: "assistant", Content: "reply1", CreatedAt: time.Now().Add(time.Second)},
		{SessionID: "s1", UserID: "u1", Role: "user", Content: "msg2", CreatedAt: time.Now().Add(2 * time.Second)},
		{SessionID: "s2", UserID: "u1", Role: "user", Content: "other", CreatedAt: time.Now()},
	}
	for _, m := range msgs {
		svc.memRepo.SaveSessionMessage(&m)
	}

	tests := []struct {
		name      string
		sessionID string
		userID    string
		limit     int
		wantCount int
	}{
		{"获取 s1 历史", "s1", "u1", 50, 3},
		{"limit 限制", "s1", "u1", 2, 2},
		{"获取 s2 历史", "s2", "u1", 50, 1},
		{"空会话", "nonexist", "u1", 50, 0},
		{"默认 limit", "s1", "u1", 0, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			history, err := svc.GetHistory(tt.sessionID, tt.userID, tt.limit)
			if err != nil {
				t.Fatalf("GetHistory error: %v", err)
			}
			if len(history) != tt.wantCount {
				t.Errorf("len = %d, want %d", len(history), tt.wantCount)
			}
		})
	}
}

// TestClearSession 测试清理会话
func TestClearSession(t *testing.T) {
	cleanup := setupChatServiceTestDB()
	defer cleanup()

	svc := NewChatService(remote.NewPythonClient("http://localhost:8000"))

	// 插入消息
	msg := &models.SessionMessage{
		SessionID: "clear-me",
		UserID:    "u1",
		Role:      "user",
		Content:   "test",
		CreatedAt: time.Now(),
	}
	svc.memRepo.SaveSessionMessage(msg)

	// 确认存在
	history, _ := svc.GetHistory("clear-me", "u1", 50)
	if len(history) == 0 {
		t.Fatal("清理前应有消息")
	}

	// 清理
	if err := svc.ClearSession("clear-me", "u1"); err != nil {
		t.Fatalf("ClearSession error: %v", err)
	}

	// 确认已清理
	history, _ = svc.GetHistory("clear-me", "u1", 50)
	if len(history) != 0 {
		t.Errorf("清理后应有 0 条消息, 实际 %d", len(history))
	}
}

// TestChatStreamHistoryFormat 测试历史消息格式转换
func TestChatStreamHistoryFormat(t *testing.T) {
	cleanup := setupChatServiceTestDB()
	defer cleanup()

	svc := NewChatService(remote.NewPythonClient("http://localhost:8000"))

	// 插入历史
	msgs := []models.SessionMessage{
		{SessionID: "fmt-test", UserID: "u1", Role: "user", Content: "问题1", CreatedAt: time.Now()},
		{SessionID: "fmt-test", UserID: "u1", Role: "assistant", Content: "回答1", CreatedAt: time.Now().Add(time.Second)},
	}
	for _, m := range msgs {
		svc.memRepo.SaveSessionMessage(&m)
	}

	// 发起聊天（Python 不可达，但验证历史已加载）
	req := ChatRequest{
		UserID:    "u1",
		SessionID: "fmt-test",
		Message:   "新问题",
	}

	var reply strings.Builder
	svc.ChatStream(req, func(chunk StreamChunk) {
		if chunk.Delta != "" {
			reply.WriteString(chunk.Delta)
		}
	})

	// 至少应保存了用户消息
	history, _ := svc.GetHistory("fmt-test", "u1", 50)
	if len(history) < 3 {
		t.Errorf("应有至少 3 条消息（2 条历史 + 1 条新用户消息）, 实际 %d", len(history))
	}
}

// TestStreamChunk 测试 StreamChunk 结构
func TestStreamChunk(t *testing.T) {
	chunk := StreamChunk{
		Reply: "完整回复",
		Delta: "新",
		Done:  false,
		Err:   nil,
	}
	if chunk.Reply != "完整回复" {
		t.Error("Reply 字段错误")
	}
	if chunk.Delta != "新" {
		t.Error("Delta 字段错误")
	}
	if chunk.Done {
		t.Error("Done 应为 false")
	}

	doneChunk := StreamChunk{Done: true, Err: nil}
	if !doneChunk.Done {
		t.Error("Done 应为 true")
	}
}