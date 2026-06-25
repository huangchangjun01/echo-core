package handlers

import (
	"echo-core/models"
	"echo-core/remote"
	"echo-core/service"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestGetHistoryHandle 测试获取历史记录
func TestGetHistoryHandle(t *testing.T) {
	cleanup := setupTestDB()
	defer cleanup()

	gin.SetMode(gin.TestMode)
	svc := service.NewChatService(remote.NewPythonClient("http://localhost:8000"))
	handler := NewChatHandler(svc)

	tests := []struct {
		name       string
		query      string
		injectUID  bool
		uidValue   string
		wantCode   int
		wantInBody string
	}{
		{
			name:       "缺少参数",
			query:      "?session_id=test",
			injectUID:  false,
			wantCode:   http.StatusBadRequest,
			wantInBody: "session_id and user_id are required",
		},
		{
			name:       "正常获取（空历史）",
			query:      "?session_id=session1&user_id=1",
			injectUID:  false,
			wantCode:   http.StatusOK,
			wantInBody: `"code":200`,
		},
		{
			name:       "中间件注入 userId 覆盖",
			query:      "?session_id=session2&user_id=attacker",
			injectUID:  true,
			uidValue:   "realUser",
			wantCode:   http.StatusOK,
			wantInBody: `"code":200`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/chat/history"+tt.query, nil)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = req
			if tt.injectUID {
				c.Set("userId", tt.uidValue)
			}
			handler.GetHistoryHandle(c)

			if w.Code != tt.wantCode {
				t.Errorf("状态码 = %d, want %d, body=%s", w.Code, tt.wantCode, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), tt.wantInBody) {
				t.Errorf("响应体不包含 %q, body=%s", tt.wantInBody, w.Body.String())
			}
		})
	}
}

// TestGetHistoryWithData 测试有数据的历史查询
func TestGetHistoryWithData(t *testing.T) {
	cleanup := setupTestDB()
	defer cleanup()

	gin.SetMode(gin.TestMode)
	svc := service.NewChatService(remote.NewPythonClient("http://localhost:8000"))
	handler := NewChatHandler(svc)

	// 先插入一些测试数据
	msgs := []models.SessionMessage{
		{SessionID: "test-session", UserID: "1", Role: "user", Content: "你好"},
		{SessionID: "test-session", UserID: "1", Role: "assistant", Content: "你好！有什么可以帮助你的？"},
	}
	for _, m := range msgs {
		svc.GetHistory(m.SessionID, m.UserID, 0)
		svc.ChatStream(service.ChatRequest{
			UserID:    m.UserID,
			SessionID: m.SessionID,
			Message:   m.Content,
		}, func(service.StreamChunk) {})
	}

	req := httptest.NewRequest("GET", "/api/chat/history?session_id=test-session&user_id=1", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	handler.GetHistoryHandle(c)

	if w.Code != http.StatusOK {
		t.Errorf("状态码 = %d, want 200, body=%s", w.Code, w.Body.String())
	}
}

// TestClearSessionHandle 测试清理会话
func TestClearSessionHandle(t *testing.T) {
	cleanup := setupTestDB()
	defer cleanup()

	gin.SetMode(gin.TestMode)
	svc := service.NewChatService(remote.NewPythonClient("http://localhost:8000"))
	handler := NewChatHandler(svc)

	tests := []struct {
		name       string
		query      string
		injectUID  bool
		uidValue   string
		wantCode   int
		wantInBody string
	}{
		{
			name:       "缺少参数",
			query:      "?session_id=test",
			injectUID:  false,
			wantCode:   http.StatusBadRequest,
			wantInBody: "session_id and user_id are required",
		},
		{
			name:       "正常清理",
			query:      "?session_id=session1&user_id=1",
			injectUID:  false,
			wantCode:   http.StatusOK,
			wantInBody: "session cleared",
		},
		{
			name:       "中间件注入 userId 覆盖",
			query:      "?session_id=session2&user_id=attacker",
			injectUID:  true,
			uidValue:   "realUser",
			wantCode:   http.StatusOK,
			wantInBody: "session cleared",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("DELETE", "/api/chat/session"+tt.query, nil)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = req
			if tt.injectUID {
				c.Set("userId", tt.uidValue)
			}
			handler.ClearSessionHandle(c)

			if w.Code != tt.wantCode {
				t.Errorf("状态码 = %d, want %d, body=%s", w.Code, tt.wantCode, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), tt.wantInBody) {
				t.Errorf("响应体不包含 %q, body=%s", tt.wantInBody, w.Body.String())
			}
		})
	}
}