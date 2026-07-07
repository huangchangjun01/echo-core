package middleware

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"time"

	"echo-core/utils"

	"github.com/gin-gonic/gin"
)

// responseRecorder 包一层 gin.ResponseWriter，记录 status/bytes/chunks。
// 关键点：
//   - Status() 在 WriteHeader/Write 后必须能拿到最终状态码
//   - Size() 累计已写入字节数
//   - chunks 仅对流式响应（SSE）有意义，每次 Write 算一次
//   - Hijack() 必须透传，否则 SSE/WS 升级会失败
type responseRecorder struct {
	gin.ResponseWriter
	status      int
	bytes       int
	chunks      int
	wroteHeader bool
}

func (r *responseRecorder) WriteHeader(code int) {
	if !r.wroteHeader {
		r.status = code
		r.wroteHeader = true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		// 显式 Write 但没 WriteHeader 的情况：Gin 默认 200
		r.status = http.StatusOK
		r.wroteHeader = true
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	if n > 0 {
		r.chunks++
	}
	return n, err
}

func (r *responseRecorder) WriteString(s string) (int, error) {
	return r.Write([]byte(s))
}

func (r *responseRecorder) Flush() {
	r.ResponseWriter.Flush()
}

func (r *responseRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("the ResponseWriter doesn't support the Hijacker interface")
	}
	return hijacker.Hijack()
}

// AccessLog 访问日志中间件
// 在 c.Next() 返回后打印一行：method / path / status / latency / size / ip / rid / uid
// 流式响应（SSE）会单独标记 chunks 计数。
//
// 挂载位置：建议 RequestID() 之后、CORS 之前——
//   - 必须在 RequestID 之后，否则 rid 为 empty
//   - 在 CORS 之前会包含 OPTIONS 预检；放 CORS 之后更安静（按需取舍）
func AccessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		recorder := &responseRecorder{ResponseWriter: c.Writer, status: http.StatusOK}
		c.Writer = recorder

		c.Next()

		latency := time.Since(start)
		utils.LogAccess(c, recorder.status, latency, recorder.bytes)
		// 流式响应额外打一条 chunks 汇总，便于区分"200 短响应"和"200 流式输出"
		if recorder.chunks > 1 {
			utils.LogWith(c, "Access", "流式响应 chunks=%d totalBytes=%d", recorder.chunks, recorder.bytes)
		}
	}
}
