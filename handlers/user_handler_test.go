package handlers

import (
	"echo-core/utils"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestRegisterHandler 测试用户注册
func TestRegisterHandler(t *testing.T) {
	cleanup := setupTestDB()
	defer cleanup()

	gin.SetMode(gin.TestMode)
	handler := NewUserHandler()

	tests := []struct {
		name       string
		body       string
		wantCode   int
		wantInBody string
	}{
		{
			name:       "注册成功",
			body:       `{"username":"newuser","password":"Test1234!","email":"new@test.com"}`,
			wantCode:   http.StatusOK,
			wantInBody: "注册成功",
		},
		{
			name:       "缺少用户名",
			body:       `{"password":"Test1234!"}`,
			wantCode:   http.StatusBadRequest,
			wantInBody: "参数错误",
		},
		{
			name:       "密码太短",
			body:       `{"username":"user2","password":"ab"}`,
			wantCode:   http.StatusBadRequest,
			wantInBody: "参数错误",
		},
		{
			name:       "重复注册",
			body:       `{"username":"newuser","password":"Test1234!","email":"dup@test.com"}`,
			wantCode:   http.StatusConflict,
			wantInBody: "账号已存在",
		},
		{
			name:       "带昵称注册",
			body:       `{"username":"user3","password":"Test1234!","nickname":"昵称","email":"nick@test.com"}`,
			wantCode:   http.StatusOK,
			wantInBody: "注册成功",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/auth/register", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = req
			handler.Register(c)

			if w.Code != tt.wantCode {
				t.Errorf("状态码 = %d, want %d, body=%s", w.Code, tt.wantCode, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), tt.wantInBody) {
				t.Errorf("响应体不包含 %q, body=%s", tt.wantInBody, w.Body.String())
			}
		})
	}
}

// TestLoginHandler 测试用户登录
func TestLoginHandler(t *testing.T) {
	cleanup := setupTestDB()
	defer cleanup()

	gin.SetMode(gin.TestMode)
	handler := NewUserHandler()

	// 先注册
	regReq := httptest.NewRequest("POST", "/api/auth/register",
		strings.NewReader(`{"username":"loginuser","password":"Test1234!"}`))
	regReq.Header.Set("Content-Type", "application/json")
	regW := httptest.NewRecorder()
	regC, _ := gin.CreateTestContext(regW)
	regC.Request = regReq
	handler.Register(regC)

	tests := []struct {
		name       string
		body       string
		wantCode   int
		wantInBody string
	}{
		{
			name:       "登录成功",
			body:       `{"username":"loginuser","password":"Test1234!"}`,
			wantCode:   http.StatusOK,
			wantInBody: "登录成功",
		},
		{
			name:       "密码错误",
			body:       `{"username":"loginuser","password":"WrongPass"}`,
			wantCode:   http.StatusUnauthorized,
			wantInBody: "账号或密码错误",
		},
		{
			name:       "账号不存在",
			body:       `{"username":"noexist","password":"Test1234!"}`,
			wantCode:   http.StatusUnauthorized,
			wantInBody: "账号或密码错误",
		},
		{
			name:       "缺少参数",
			body:       `{"username":"loginuser"}`,
			wantCode:   http.StatusBadRequest,
			wantInBody: "参数错误",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = req
			handler.Login(c)

			if w.Code != tt.wantCode {
				t.Errorf("状态码 = %d, want %d, body=%s", w.Code, tt.wantCode, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), tt.wantInBody) {
				t.Errorf("响应体不包含 %q, body=%s", tt.wantInBody, w.Body.String())
			}

			// 登录成功时验证返回 sessionId
			if tt.wantCode == http.StatusOK {
				var resp map[string]interface{}
				json.Unmarshal(w.Body.Bytes(), &resp)
				data, ok := resp["data"].(map[string]interface{})
				if !ok || data["sessionId"] == nil || data["sessionId"].(string) == "" {
					t.Error("登录成功应返回 sessionId")
				}
			}
		})
	}
}

// TestCheckAccountHandler 测试账号校验
func TestCheckAccountHandler(t *testing.T) {
	cleanup := setupTestDB()
	defer cleanup()

	gin.SetMode(gin.TestMode)
	handler := NewUserHandler()

	// 先注册
	regReq := httptest.NewRequest("POST", "/api/auth/register",
		strings.NewReader(`{"username":"checkuser","password":"Test1234!"}`))
	regReq.Header.Set("Content-Type", "application/json")
	regW := httptest.NewRecorder()
	regC, _ := gin.CreateTestContext(regW)
	regC.Request = regReq
	handler.Register(regC)

	tests := []struct {
		name       string
		body       string
		wantCode   int
		wantInBody string
	}{
		{
			name:       "账号已被占用",
			body:       `{"username":"checkuser"}`,
			wantCode:   http.StatusOK,
			wantInBody: "账号已被占用",
		},
		{
			name:       "账号可用",
			body:       `{"username":"newone"}`,
			wantCode:   http.StatusOK,
			wantInBody: "账号可用",
		},
		{
			name:       "缺少参数",
			body:       `{}`,
			wantCode:   http.StatusBadRequest,
			wantInBody: "参数错误",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/auth/checkAccount", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = req
			handler.CheckAccount(c)

			if w.Code != tt.wantCode {
				t.Errorf("状态码 = %d, want %d, body=%s", w.Code, tt.wantCode, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), tt.wantInBody) {
				t.Errorf("响应体不包含 %q, body=%s", tt.wantInBody, w.Body.String())
			}
		})
	}
}

// TestCheckSessionHandler 测试会话校验
func TestCheckSessionHandler(t *testing.T) {
	cleanup := setupTestDB()
	defer cleanup()

	gin.SetMode(gin.TestMode)
	handler := NewUserHandler()

	sid := createTestSession(1, "testuser")

	tests := []struct {
		name       string
		body       string
		wantCode   int
		wantInBody string
	}{
		{
			name:       "会话有效",
			body:       `{"sessionId":"` + sid + `"}`,
			wantCode:   http.StatusOK,
			wantInBody: "会话有效",
		},
		{
			name:       "会话无效",
			body:       `{"sessionId":"invalid-session"}`,
			wantCode:   http.StatusOK,
			wantInBody: "会话无效",
		},
		{
			name:       "缺少参数",
			body:       `{}`,
			wantCode:   http.StatusBadRequest,
			wantInBody: "参数错误",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/auth/check", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = req
			handler.Check(c)

			if w.Code != tt.wantCode {
				t.Errorf("状态码 = %d, want %d, body=%s", w.Code, tt.wantCode, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), tt.wantInBody) {
				t.Errorf("响应体不包含 %q, body=%s", tt.wantInBody, w.Body.String())
			}
		})
	}
}

// TestLogoutHandler 测试注销
func TestLogoutHandler(t *testing.T) {
	cleanup := setupTestDB()
	defer cleanup()

	gin.SetMode(gin.TestMode)
	handler := NewUserHandler()

	sid := createTestSession(1, "testuser")

	tests := []struct {
		name       string
		body       string
		wantCode   int
		wantInBody string
	}{
		{
			name:       "注销成功",
			body:       `{"sessionId":"` + sid + `"}`,
			wantCode:   http.StatusOK,
			wantInBody: "退出成功",
		},
		{
			name:       "缺少参数",
			body:       `{}`,
			wantCode:   http.StatusBadRequest,
			wantInBody: "参数错误",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/auth/logout", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = req
			handler.Logout(c)

			if w.Code != tt.wantCode {
				t.Errorf("状态码 = %d, want %d, body=%s", w.Code, tt.wantCode, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), tt.wantInBody) {
				t.Errorf("响应体不包含 %q, body=%s", tt.wantInBody, w.Body.String())
			}
		})
	}

	// 注销后会话应不可用
	store := utils.GetSessionStore()
	_, err := store.Get(sid)
	if err == nil {
		t.Error("注销后会话应被删除")
	}
}

// TestLoginLogoutFlow 测试完整登录注销流程
func TestLoginLogoutFlow(t *testing.T) {
	cleanup := setupTestDB()
	defer cleanup()

	gin.SetMode(gin.TestMode)
	handler := NewUserHandler()

	// 1. 注册
	regReq := httptest.NewRequest("POST", "/api/auth/register",
		strings.NewReader(`{"username":"flowuser","password":"Test1234!","email":"flow@test.com"}`))
	regReq.Header.Set("Content-Type", "application/json")
	regW := httptest.NewRecorder()
	regC, _ := gin.CreateTestContext(regW)
	regC.Request = regReq
	handler.Register(regC)

	if regW.Code != http.StatusOK {
		t.Fatalf("注册失败: %s", regW.Body.String())
	}

	// 2. 登录
	loginReq := httptest.NewRequest("POST", "/api/auth/login",
		strings.NewReader(`{"username":"flowuser","password":"Test1234!"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginW := httptest.NewRecorder()
	loginC, _ := gin.CreateTestContext(loginW)
	loginC.Request = loginReq
	handler.Login(loginC)

	if loginW.Code != http.StatusOK {
		t.Fatalf("登录失败: %s", loginW.Body.String())
	}

	var loginResp map[string]interface{}
	json.Unmarshal(loginW.Body.Bytes(), &loginResp)
	data := loginResp["data"].(map[string]interface{})
	sid := data["sessionId"].(string)

	// 3. 校验会话
	checkReq := httptest.NewRequest("POST", "/api/auth/check",
		strings.NewReader(`{"sessionId":"`+sid+`"}`))
	checkReq.Header.Set("Content-Type", "application/json")
	checkW := httptest.NewRecorder()
	checkC, _ := gin.CreateTestContext(checkW)
	checkC.Request = checkReq
	handler.Check(checkC)

	if checkW.Code != http.StatusOK {
		t.Fatalf("会话校验失败: %s", checkW.Body.String())
	}
	if !strings.Contains(checkW.Body.String(), "会话有效") {
		t.Error("应返回会话有效")
	}

	// 4. 注销
	logoutReq := httptest.NewRequest("POST", "/api/auth/logout",
		strings.NewReader(`{"sessionId":"`+sid+`"}`))
	logoutReq.Header.Set("Content-Type", "application/json")
	logoutW := httptest.NewRecorder()
	logoutC, _ := gin.CreateTestContext(logoutW)
	logoutC.Request = logoutReq
	handler.Logout(logoutC)

	if logoutW.Code != http.StatusOK {
		t.Fatalf("注销失败: %s", logoutW.Body.String())
	}

	// 5. 注销后校验
	checkReq2 := httptest.NewRequest("POST", "/api/auth/check",
		strings.NewReader(`{"sessionId":"`+sid+`"}`))
	checkReq2.Header.Set("Content-Type", "application/json")
	checkW2 := httptest.NewRecorder()
	checkC2, _ := gin.CreateTestContext(checkW2)
	checkC2.Request = checkReq2
	handler.Check(checkC2)

	if !strings.Contains(checkW2.Body.String(), "会话无效") {
		t.Error("注销后应返回会话无效")
	}
}