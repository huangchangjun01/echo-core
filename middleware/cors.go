package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// CORS 跨域中间件
// 触发场景：浏览器前端从不同 origin 访问 Go 服务时，浏览器会先发 OPTIONS 预检，
// 没有 Access-Control-* 响应头就直接拦截实际请求。SSE 响应被拦截后表现就是
// "页面一条信息也没有展示"——网络面板能看到请求发出 + 200 响应，但 JS 拿不到 body。
//
// 当前实现：放开所有 origin（开发期足够）；生产期应改为 env 白名单。
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin == "" {
			// 非浏览器（curl / 服务端调用）无 Origin，按 * 放行
			origin = "*"
		}
		c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Session-Id, X-Request-Id, Authorization, Cookie")
		c.Writer.Header().Set("Access-Control-Expose-Headers", "X-Request-Id, Content-Type, Cache-Control")
		c.Writer.Header().Set("Access-Control-Max-Age", "86400")
		c.Writer.Header().Set("Vary", "Origin")

		if c.Request.Method == http.MethodOptions {
			LogWith(c, "CORS", "预检通过 | origin=%s | %s %s", origin, c.Request.Method, c.Request.URL.Path)
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		LogWith(c, "CORS", "跨域放行 | origin=%s | %s %s", origin, c.Request.Method, c.Request.URL.Path)
		c.Next()
	}
}
