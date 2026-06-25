package middleware

import (
	"echo-core/utils"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func setupMiddlewareTest() *gin.Engine {
	gin.SetMode(gin.TestMode)
	// 重置 session store 为测试专用
	store := utils.NewMemorySessionStore(24 * time.Hour)
	utils.SetSessionStoreForTest(store)

	r := gin.New()
	return r
}

// TestExtractSessionIDFromHeader 测试从 Header 提取 sessionId
func TestExtractSessionIDFromHeader(t *testing.T) {
	r := setupMiddlewareTest()
	r.Use(RequireSession())
	r.GET("/test", func(c *gin.Context) {
		uid, _ := MustUserID(c)
		c.JSON(http.StatusOK, gin.H{"userId": uid})
	})

	// 先在 store 中创建会话
	store := utils.GetSessionStore()
	sess, _ := store.Create(100, "testuser", time.Hour)

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Session-Id", sess.SessionID)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("期望 200, 得到 %d, body=%s", w.Code, w.Body.String())
	}
}

// TestExtractSessionIDFromCookie 测试从 Cookie 提取 sessionId
func TestExtractSessionIDFromCookie(t *testing.T) {
	r := setupMiddlewareTest()
	r.Use(RequireSession())
	r.GET("/test", func(c *gin.Context) {
		uid, _ := MustUserID(c)
		c.JSON(http.StatusOK, gin.H{"userId": uid})
	})

	store := utils.GetSessionStore()
	sess, _ := store.Create(101, "cookieuser", time.Hour)

	req := httptest.NewRequest("GET", "/test", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: sess.SessionID})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("期望 200, 得到 %d, body=%s", w.Code, w.Body.String())
	}
}

// TestExtractSessionIDFromBody 测试从 Body 提取 sessionId
func TestExtractSessionIDFromBody(t *testing.T) {
	r := setupMiddlewareTest()
	r.Use(RequireSession())
	r.POST("/test", func(c *gin.Context) {
		uid, _ := MustUserID(c)
		c.JSON(http.StatusOK, gin.H{"userId": uid})
	})

	store := utils.GetSessionStore()
	sess, _ := store.Create(102, "bodyuser", time.Hour)

	body := `{"sessionId":"` + sess.SessionID + `","message":"hello"}`
	req := httptest.NewRequest("POST", "/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("期望 200, 得到 %d, body=%s", w.Code, w.Body.String())
	}
}

// TestRequireSessionMissing 测试缺少 sessionId 返回 401
func TestRequireSessionMissing(t *testing.T) {
	r := setupMiddlewareTest()
	r.Use(RequireSession())
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("期望 401, 得到 %d", w.Code)
	}
}

// TestRequireSessionInvalid 测试无效 sessionId 返回 401
func TestRequireSessionInvalid(t *testing.T) {
	r := setupMiddlewareTest()
	r.Use(RequireSession())
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Session-Id", "invalid-session-id-that-does-not-exist")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("期望 401, 得到 %d", w.Code)
	}
}

// TestExtractSessionIDPriority 测试提取优先级：Header > Cookie > Body
func TestExtractSessionIDPriority(t *testing.T) {
	r := setupMiddlewareTest()
	r.Use(RequireSession())
	r.POST("/test", func(c *gin.Context) {
		uid, _ := MustUserID(c)
		c.JSON(http.StatusOK, gin.H{"userId": uid})
	})

	store := utils.GetSessionStore()
	headerSess, _ := store.Create(200, "headerUser", time.Hour)
	cookieSess, _ := store.Create(201, "cookieUser", time.Hour)
	bodySess, _ := store.Create(202, "bodyUser", time.Hour)

	body := `{"sessionId":"` + bodySess.SessionID + `"}`
	req := httptest.NewRequest("POST", "/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Session-Id", headerSess.SessionID) // Header 优先级最高
	req.AddCookie(&http.Cookie{Name: "session_id", Value: cookieSess.SessionID})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("期望 200, 得到 %d", w.Code)
	}
	if w.Body.String() != `{"userId":"200"}` {
		t.Errorf("应使用 Header 中的 session, 得到 %s", w.Body.String())
	}
}

// TestMustUserID 测试 MustUserID 辅助函数
func TestMustUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// 未注入 userId
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	_, ok := MustUserID(c)
	if ok {
		t.Error("未注入 userId 时 MustUserID 应返回 false")
	}

	// 注入 userId
	c.Set(CtxKeyUserID, "123")
	uid, ok := MustUserID(c)
	if !ok {
		t.Error("已注入 userId 时 MustUserID 应返回 true")
	}
	if uid != "123" {
		t.Errorf("uid = %s, want 123", uid)
	}
}

// TestOptionalSession 测试可选会话中间件
func TestOptionalSession(t *testing.T) {
	r := setupMiddlewareTest()
	r.Use(OptionalSession())
	r.GET("/test", func(c *gin.Context) {
		uid, ok := MustUserID(c)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"userId": "anonymous"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"userId": uid})
	})

	// 无 session 应正常通过
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("无 session 时应返回 200, 得到 %d", w.Code)
	}

	// 有 session 应注入 userId
	store := utils.GetSessionStore()
	sess, _ := store.Create(300, "optUser", time.Hour)
	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.Header.Set("X-Session-Id", sess.SessionID)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("期望 200, 得到 %d", w2.Code)
	}
}