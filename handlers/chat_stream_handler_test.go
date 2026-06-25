package handlers

import (
	"echo-core/remote"
	"echo-core/service"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestChatHandleSSEParamValidation 测试 SSE 聊天参数校验
func TestChatHandleSSEParamValidation(t *testing.T) {
	cleanup := setupTestDB()
	defer cleanup()

	gin.SetMode(gin.TestMode)
	svc := service.NewChatService(remote.NewPythonClient("http://localhost:8000"))
	handler := NewChatStreamHandler(svc)

	tests := []struct {
		name       string
		body       string
		injectUID  bool
		uidValue   string
		wantCode   int
		wantInBody string
	}{
		{
			name:       "缺少 userId",
			body:       `{"sessionId":"s1","message":"hello"}`,
			wantCode:   http.StatusBadRequest,
			wantInBody: "userId, sessionId and message are required",
		},
		{
			name:       "缺少 sessionId",
			body:       `{"userId":"1","message":"hello"}`,
			wantCode:   http.StatusBadRequest,
			wantInBody: "userId, sessionId and message are required",
		},
		{
			name:       "缺少 message",
			body:       `{"userId":"1","sessionId":"s1"}`,
			wantCode:   http.StatusBadRequest,
			wantInBody: "userId, sessionId and message are required",
		},
		{
			name:       "无效 JSON",
			body:       `{invalid`,
			wantCode:   http.StatusBadRequest,
			wantInBody: "error",
		},
		{
			name:       "中间件注入 userId 覆盖",
			body:       `{"userId":"attacker","sessionId":"s1","message":"hello"}`,
			injectUID:  true,
			uidValue:   "realUser",
			wantCode:   http.StatusOK,
			wantInBody: "event: start",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/chat", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = req
			if tt.injectUID {
				c.Set("userId", tt.uidValue)
			}
			handler.ChatHandleSSE(c)

			if w.Code != tt.wantCode {
				t.Errorf("状态码 = %d, want %d, body=%s", w.Code, tt.wantCode, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), tt.wantInBody) {
				t.Errorf("响应体不包含 %q, body=%s", tt.wantInBody, w.Body.String())
			}
		})
	}
}

// TestChatHandleSSEHeaders 测试 SSE 响应头
func TestChatHandleSSEHeaders(t *testing.T) {
	cleanup := setupTestDB()
	defer cleanup()

	gin.SetMode(gin.TestMode)
	svc := service.NewChatService(remote.NewPythonClient("http://localhost:8000"))
	handler := NewChatStreamHandler(svc)

	req := httptest.NewRequest("POST", "/api/chat",
		strings.NewReader(`{"userId":"1","sessionId":"s1","message":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	handler.ChatHandleSSE(c)

	// 验证 SSE 响应头
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") {
		t.Errorf("Content-Type = %s, 应包含 text/event-stream", ct)
	}
	if w.Header().Get("Cache-Control") != "no-cache" {
		t.Error("Cache-Control 应为 no-cache")
	}
	if w.Header().Get("Connection") != "keep-alive" {
		t.Error("Connection 应为 keep-alive")
	}
}

// TestChatHandleSSEStartEvent 测试 SSE start 事件
func TestChatHandleSSEStartEvent(t *testing.T) {
	cleanup := setupTestDB()
	defer cleanup()

	gin.SetMode(gin.TestMode)
	svc := service.NewChatService(remote.NewPythonClient("http://localhost:8000"))
	handler := NewChatStreamHandler(svc)

	req := httptest.NewRequest("POST", "/api/chat",
		strings.NewReader(`{"userId":"1","sessionId":"test-session","message":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	handler.ChatHandleSSE(c)

	body := w.Body.String()
	if !strings.Contains(body, "event: start") {
		t.Error("SSE 响应应包含 start 事件")
	}
	if !strings.Contains(body, "test-session") {
		t.Error("start 事件应包含 sessionId")
	}
}

// TestChatHandleSSEErrorEvent 测试 SSE error 事件（Python 不可达）
func TestChatHandleSSEErrorEvent(t *testing.T) {
	cleanup := setupTestDB()
	defer cleanup()

	gin.SetMode(gin.TestMode)
	svc := service.NewChatService(remote.NewPythonClient("http://localhost:8000"))
	handler := NewChatStreamHandler(svc)

	req := httptest.NewRequest("POST", "/api/chat",
		strings.NewReader(`{"userId":"1","sessionId":"s1","message":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	handler.ChatHandleSSE(c)

	body := w.Body.String()
	// Python 服务不可达，应收到 error 事件
	if !strings.Contains(body, "event: error") {
		t.Errorf("Python 不可达时应返回 error 事件, body=%s", body)
	}
}

// TestChatHandleSSEFinishEvent 测试 SSE finish 事件
func TestChatHandleSSEFinishEvent(t *testing.T) {
	cleanup := setupTestDB()
	defer cleanup()

	gin.SetMode(gin.TestMode)
	svc := service.NewChatService(remote.NewPythonClient("http://localhost:8000"))
	handler := NewChatStreamHandler(svc)

	req := httptest.NewRequest("POST", "/api/chat",
		strings.NewReader(`{"userId":"1","sessionId":"s1","message":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	handler.ChatHandleSSE(c)

	body := w.Body.String()
	// 流结束时应收到 finish 或 error 事件
	hasFinish := strings.Contains(body, "event: finish")
	hasError := strings.Contains(body, "event: error")
	if !hasFinish && !hasError {
		t.Errorf("SSE 响应应包含 finish 或 error 事件, body=%s", body)
	}
}