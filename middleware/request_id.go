package middleware

import (
	"echo-core/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// CtxKeyRequestID gin.Context 里 request_id 的键名。
const CtxKeyRequestID = "request_id"

// HeaderRequestID 请求/响应都携带的 header 名称。
const HeaderRequestID = "X-Request-Id"

// RequestID 注入 request_id 中间件
// 优先使用请求方传入的 X-Request-Id（便于跨服务追踪），缺失时生成 UUID v4。
// 注入到：
//   - gin.Context: c.Set("request_id", id)
//   - http.Request.Context(): utils.WithRID(ctx, id)
//
// 同时写回 response header。
//
// 之所以同时注入到两层：handler/service 既能从 gin 取（c.Get / MustRequestID），
// 也能从 c.Request.Context() 取（utils.LogWithCtx），任一调用方都方便。
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.GetHeader(HeaderRequestID)
		if rid == "" {
			rid = uuid.NewString()
		}
		c.Set(CtxKeyRequestID, rid)
		c.Writer.Header().Set(HeaderRequestID, rid)

		// 注入到 Request.Context()，让 service/remote 层通过 utils.LogWithCtx 拿到 rid。
		ctx := utils.WithRID(c.Request.Context(), rid)
		c.Request = c.Request.WithContext(ctx)

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

// LogWith 业务侧日志入口：自动带 [rid=.. uid=..] 前缀。
// 内部转调 utils.LogWithCtx，从 c.Request.Context() 读 rid/uid。
// 用法：middleware.LogWith(c, "ChatService", "xxx %s", arg)
func LogWith(c *gin.Context, component, format string, args ...interface{}) {
	utils.LogWith(c, component, format, args...)
}
