package utils

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// CtxKey 用于把 rid/uid 注入 context.Context 的键类型。
// context 键应为不可导出的具体类型，避免与第三方包冲突。
type ctxKey string

const (
	// CtxKeyRID 请求级追踪 ID（来自 X-Request-Id header 或中间件生成）。
	CtxKeyRID ctxKey = "rid"
	// CtxKeyUID 当前请求已认证用户 ID（来自鉴权中间件注入）。
	CtxKeyUID ctxKey = "uid"
)

// 内部约定：rid/uid 缺失时显示为 "empty"，便于 grep。
const emptyMarker = "empty"

// WithRID 把 rid 写入 context（中间件使用）。
func WithRID(ctx context.Context, rid string) context.Context {
	return context.WithValue(ctx, CtxKeyRID, rid)
}

// WithUID 把 uid 写入 context（鉴权中间件使用）。
func WithUID(ctx context.Context, uid string) context.Context {
	return context.WithValue(ctx, CtxKeyUID, uid)
}

// RIDFromCtx 从 context 取 rid；空时返回 "empty"。
func RIDFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(CtxKeyRID).(string); ok && v != "" {
		return v
	}
	return emptyMarker
}

// UIDFromCtx 从 context 取 uid；空时返回 "empty"。
func UIDFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(CtxKeyUID).(string); ok && v != "" {
		return v
	}
	return emptyMarker
}

// LogWithCtx 业务代码统一入口：从 ctx 读 rid/uid，打印一行带前缀的日志。
// 格式：[rid=xxx uid=yyy] <message>
//
// 使用：
//
//	utils.LogWithCtx(ctx, "ChatService", "ChatSync 完成 | events=%d", len(events))
//
// 当 ctx 来自 *gin.Context 时，建议 handler 调用 utils.LogWith(c, ...)，
// 由其内部转调本函数（避免每次手写 c.Request.Context()）。
func LogWithCtx(ctx context.Context, component, format string, args ...interface{}) {
	if ctx == nil {
		ctx = context.Background()
	}
	rid := RIDFromCtx(ctx)
	uid := UIDFromCtx(ctx)
	msg := fmt.Sprintf(format, args...)
	log.Printf("[rid=%s uid=%s] [%s] %s", rid, uid, component, msg)
}

// LogWith 是 middleware.LogWith 的同语义版本：传入 *gin.Context 时自动取
// 其 Request.Context() 调 LogWithCtx。保留 gin.Context 入参是为兼容
// middleware/auth.go 等已有调用方；service/remote 层应直接用 LogWithCtx。
func LogWith(c *gin.Context, component, format string, args ...interface{}) {
	if c == nil {
		LogWithCtx(context.Background(), component, format, args...)
		return
	}
	LogWithCtx(c.Request.Context(), component, format, args...)
}

// LogAccess 访问日志：由 middleware/access_log.go 在响应结束后调用一次。
// 字段：method / path / status / latency / bytes / ip / rid / uid
func LogAccess(c *gin.Context, status int, latency time.Duration, bytes int) {
	if c == nil {
		return
	}
	ctx := c.Request.Context()
	rid := RIDFromCtx(ctx)
	uid := UIDFromCtx(ctx)
	method := c.Request.Method
	path := c.Request.URL.Path
	ip := c.ClientIP()
	log.Printf("[rid=%s uid=%s] [Access] %s %s status=%d latency=%dms size=%dB ip=%s",
		rid, uid, method, path, status, latency.Milliseconds(), bytes, ip)
}

// LogStartup 启动横幅：打印一行 ==== 包裹的配置信息，便于核对环境。
// 用法：utils.LogStartup("startup", "env", "dev", "db", "testdb")
func LogStartup(component string, kv ...string) {
	if len(kv)%2 != 0 {
		kv = append(kv, "<missing>")
	}
	pairs := make([]string, 0, len(kv)/2)
	for i := 0; i < len(kv); i += 2 {
		pairs = append(pairs, fmt.Sprintf("%s=%s", kv[i], kv[i+1]))
	}
	log.Printf("==== [%s] %s ====", component, strings.Join(pairs, " "))
}
