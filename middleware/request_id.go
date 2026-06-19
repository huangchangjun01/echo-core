package middleware

import (
	"fmt"
	"log"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// CtxKeyRequestID gin.Context 里 request_id 的键名。
const CtxKeyRequestID = "request_id"

// HeaderRequestID 请求/响应都携带的 header 名称。
const HeaderRequestID = "X-Request-Id"

// RequestID 注入 request_id 中间件
// 优先使用请求方传入的 X-Request-Id（便于跨服务追踪），缺失时生成 UUID v4。
// 注入到 c.Set("request_id", id) 并写回 response header。
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.GetHeader(HeaderRequestID)
		if rid == "" {
			rid = uuid.NewString()
		}
		c.Set(CtxKeyRequestID, rid)
		c.Writer.Header().Set(HeaderRequestID, rid)
		c.Next()
	}
}

// MustRequestID 从 context 取 request_id；handler 内调用。
func MustRequestID(c *gin.Context) string {
	if v, ok := c.Get(CtxKeyRequestID); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// LogWith 打印一条带 request_id/user_id 前缀的日志（stdlib log）。
// 业务代码里调用：middleware.LogWith(c, "[ChatService] xxx %s", arg)
// 注意：本函数是 graceful fallback——未注入 request_id 时只输出原 format，
// 不会 panic；适合在 service 层（拿不到 gin.Context）也能"安全失败"。
func LogWith(c *gin.Context, format string, args ...interface{}) {
	rid := MustRequestID(c)
	uid, _ := c.Get(CtxKeyUserID)
	uidStr, _ := uid.(string)
	log.Printf("[rid=%s uid=%s] %s", rid, uidStr, fmt.Sprintf(format, args...))
}
