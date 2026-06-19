package middleware

import (
	"echo-core/utils"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// 常量：注入到 gin.Context 的键名，业务侧通过 c.GetString("userId") 取。
const (
	CtxKeyUserID    = "userId"
	CtxKeySessionID = "sessionId"
	CtxKeyUsername  = "username"
)

// Header / Cookie / Body 字段名约定。
const (
	HeaderSessionID = "X-Session-Id"
	CookieSessionID = "session_id"
	BodyFieldSID    = "sessionId"
)

// extractSessionID 读取 sessionId，优先级：Header > Cookie > Body(POST/PUT/DELETE)
// Body 仅在 Content-Type 为 application/json 时尝试解析。
func extractSessionID(c *gin.Context) string {
	if sid := c.GetHeader(HeaderSessionID); sid != "" {
		return sid
	}
	if sid, err := c.Cookie(CookieSessionID); err == nil && sid != "" {
		return sid
	}
	if c.Request.Method == http.MethodGet {
		return ""
	}
	ct := c.GetHeader("Content-Type")
	if ct != "" && ct != "application/json" && ct != "application/json; charset=utf-8" {
		return ""
	}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return ""
	}
	// 还原 Body 给后续 handler（ShouldBindJSON 需要再次读取）
	c.Request.Body = io.NopCloser(readSeekerFromBytes(body))

	// 轻量正则匹配 "sessionId":"xxx"——避免引入 json 依赖解析整个 body
	// 简化策略：寻找 "sessionId" 字段的字符串值；找不到返回空。
	sid := grepJSONString(body, BodyFieldSID)
	return sid
}

// readSeekerFromBytes 把 []byte 包成 io.ReadSeeker，便于 c.Request.Body 复用
func readSeekerFromBytes(b []byte) io.ReadSeeker {
	return &byteReader{buf: b, pos: 0}
}

type byteReader struct {
	buf []byte
	pos int
}

func (r *byteReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.buf) {
		return 0, io.EOF
	}
	n := copy(p, r.buf[r.pos:])
	r.pos += n
	return n, nil
}
func (r *byteReader) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case 0:
		r.pos = int(offset)
	case 1:
		r.pos += int(offset)
	case 2:
		r.pos = len(r.buf) + int(offset)
	}
	return int64(r.pos), nil
}

// grepJSONString 从原始 JSON 字节中简单提取指定 key 的字符串值。
// 不追求完美（嵌套/转义），仅用于中间件 fast path；真正的解析由后续 handler 负责。
func grepJSONString(raw []byte, key string) string {
	needle := `"` + key + `"`
	idx := indexOf(raw, needle)
	if idx < 0 {
		return ""
	}
	// 跳过 key 和可能的空白
	i := idx + len(needle)
	for i < len(raw) && (raw[i] == ' ' || raw[i] == '\t' || raw[i] == ':') {
		i++
	}
	if i >= len(raw) || raw[i] != '"' {
		return ""
	}
	// 找到闭合 "
	j := i + 1
	for j < len(raw) && raw[j] != '"' {
		if raw[j] == '\\' && j+1 < len(raw) {
			j += 2
			continue
		}
		j++
	}
	if j >= len(raw) {
		return ""
	}
	return string(raw[i+1 : j])
}

func indexOf(haystack []byte, needle string) int {
	n := []byte(needle)
	for i := 0; i+len(n) <= len(haystack); i++ {
		match := true
		for k := 0; k < len(n); k++ {
			if haystack[i+k] != n[k] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// RequireSession 强制要求请求携带有效 session。
// 成功时往 c.Set 注入 userId/sessionId/username；失败时 401 终止。
func RequireSession() gin.HandlerFunc {
	store := utils.GetSessionStore()
	return func(c *gin.Context) {
		sid := extractSessionID(c)
		if sid == "" {
			LogWith(c, "[AuthMiddleware] 缺少 sessionId | method=%s path=%s ip=%s", c.Request.Method, c.Request.URL.Path, c.ClientIP())
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    401,
				"message": "unauthorized: missing sessionId",
			})
			return
		}
		sess, err := store.Get(sid)
		if err != nil {
			if errors.Is(err, utils.ErrSessionNotFound) {
				LogWith(c, "[AuthMiddleware] session 无效或已过期 | sid=%s ip=%s", sid, c.ClientIP())
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"code":    401,
					"message": "unauthorized: invalid or expired session",
				})
				return
			}
			LogWith(c, "[AuthMiddleware] session 查询失败 | sid=%s err=%v", sid, err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "internal error",
			})
			return
		}
		// 续期（滑动过期）
		if err := store.Touch(sid); err != nil {
			LogWith(c, "[AuthMiddleware] session 续期失败 | sid=%s err=%v", sid, err)
		}
		// 注入上下文
		c.Set(CtxKeyUserID, strconv.FormatUint(uint64(sess.UserID), 10))
		c.Set(CtxKeySessionID, sess.SessionID)
		c.Set(CtxKeyUsername, sess.Username)
		c.Next()
	}
}

// OptionalSession 不强制要求 session；命中时注入 userId，未命中继续（用于匿名场景的弱识别）
func OptionalSession() gin.HandlerFunc {
	store := utils.GetSessionStore()
	return func(c *gin.Context) {
		sid := extractSessionID(c)
		if sid == "" {
			c.Next()
			return
		}
		sess, err := store.Get(sid)
		if err != nil {
			c.Next()
			return
		}
		_ = store.Touch(sid)
		c.Set(CtxKeyUserID, strconv.FormatUint(uint64(sess.UserID), 10))
		c.Set(CtxKeySessionID, sess.SessionID)
		c.Set(CtxKeyUsername, sess.Username)
		c.Next()
	}
}

// MustUserID 从 context 取出已注入的 userId；调用方需保证已在 RequireSession 之后。
func MustUserID(c *gin.Context) (string, bool) {
	v, ok := c.Get(CtxKeyUserID)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}
