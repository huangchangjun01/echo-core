package remote

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"echo-core/dto"
	"echo-core/utils"
)

// PythonChatRequest 调用 Python /chat 的请求体
type PythonChatRequest struct {
	UserID    string `json:"userId"`
	SessionID string `json:"sessionId"`
	Message   string `json:"message"`
	Stream    bool   `json:"stream"`
}

// PythonChatClient 负责把对话请求以同步 / 流式两种形态透传到 Python 服务。
//
// 协议：
//   - stream=false：POST /chat 返回单个 JSON（含 events 数组），由 ChatSync 处理。
//   - stream=true ：POST /chat 返回 text/event-stream；每帧 data: {...} 携带
//     一种 ChatEvent（type ∈ context/tool/prefix/delta/done/memory_extracted），
//     由 ChatStreamEvents 解析。
type PythonChatClient struct {
	baseURL string
	client  *http.Client
}

// NewPythonChatClient 构造 Python 聊天客户端
// baseURL 取自环境变量 ECHO_AI_REMOTE_BASE_URL，默认 http://localhost:8000
func NewPythonChatClient() *PythonChatClient {
	baseURL := strings.TrimSpace(os.Getenv("ECHO_AI_REMOTE_BASE_URL"))
	if baseURL == "" {
		baseURL = "http://localhost:8000"
	}
	log.Printf("==== [PythonChatClient] 初始化 | baseURL=%s timeout=120s ====", baseURL)
	return &PythonChatClient{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// ChatSync 同步调用 Python /chat（stream=false）
// 返回完整的 ChatSyncResponse（含 reply、events 数组、latencyMs）。
func (c *PythonChatClient) ChatSync(ctx context.Context, req PythonChatRequest) (*dto.ChatSyncResponse, error) {
	utils.LogWithCtx(ctx, "PythonChatClient.ChatSync", "发送请求 | url=%s/chat stream=false userId=%s sessionId=%s msgLen=%d",
		c.baseURL, req.UserID, req.SessionID, len(req.Message))

	body, err := json.Marshal(req)
	if err != nil {
		utils.LogWithCtx(ctx, "PythonChatClient.ChatSync", "序列化请求失败 | err=%v", err)
		return nil, fmt.Errorf("marshal python chat request failed: %w", err)
	}
	httpReq, err := http.NewRequest("POST", c.baseURL+"/chat", bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("create python chat request failed: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	start := time.Now()
	resp, err := c.client.Do(httpReq)
	if err != nil {
		utils.LogWithCtx(ctx, "PythonChatClient.ChatSync", "HTTP 请求失败 | err=%v latency=%dms", err, time.Since(start).Milliseconds())
		return nil, fmt.Errorf("python chat request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		utils.LogWithCtx(ctx, "PythonChatClient.ChatSync", "Python 返回非 200 | status=%d latency=%dms body=%s",
			resp.StatusCode, time.Since(start).Milliseconds(), truncateForLog(string(raw), 512))
		return nil, fmt.Errorf("python chat returned status %d: %s", resp.StatusCode, string(raw))
	}

	// 读 body 并统计字节数
	rawBody, _ := io.ReadAll(resp.Body)
	var out dto.ChatSyncResponse
	if err := json.Unmarshal(rawBody, &out); err != nil {
		utils.LogWithCtx(ctx, "PythonChatClient.ChatSync", "响应反序列化失败 | err=%v bodyBytes=%d", err, len(rawBody))
		return nil, fmt.Errorf("decode python chat response failed: %w", err)
	}
	utils.LogWithCtx(ctx, "PythonChatClient.ChatSync", "Python 响应完成 | status=200 latency=%dms bodyBytes=%d events=%d replyLen=%d",
		time.Since(start).Milliseconds(), len(rawBody), len(out.Events), len(out.Reply))
	return &out, nil
}

// ChatStreamEvents 流式调用 Python /chat（stream=true）
// handler 收到每条 ChatEvent（type 已在结构体中）。流结束或出错时返回。
func (c *PythonChatClient) ChatStreamEvents(ctx context.Context, req PythonChatRequest, handler func(dto.ChatEvent) error) error {
	utils.LogWithCtx(ctx, "PythonChatClient.ChatStreamEvents", "发送请求 | url=%s/chat stream=true userId=%s sessionId=%s msgLen=%d",
		c.baseURL, req.UserID, req.SessionID, len(req.Message))

	body, err := json.Marshal(req)
	if err != nil {
		utils.LogWithCtx(ctx, "PythonChatClient.ChatStreamEvents", "序列化请求失败 | err=%v", err)
		return fmt.Errorf("marshal python chat request failed: %w", err)
	}
	// 打印请求体预览，便于核对实际下发的 userId/sessionId/message
	utils.LogWithCtx(ctx, "PythonChatClient.ChatStreamEvents", "请求体预览 | body=%s", truncateForLog(string(body), 1024))

	httpReq, err := http.NewRequest("POST", c.baseURL+"/chat", bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("create python chat request failed: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	start := time.Now()
	resp, err := c.client.Do(httpReq)
	if err != nil {
		utils.LogWithCtx(ctx, "PythonChatClient.ChatStreamEvents", "HTTP 请求失败 | err=%v latency=%dms", err, time.Since(start).Milliseconds())
		return fmt.Errorf("python chat request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		utils.LogWithCtx(ctx, "PythonChatClient.ChatStreamEvents", "Python 返回非 200 | status=%d latency=%dms body=%s",
			resp.StatusCode, time.Since(start).Milliseconds(), truncateForLog(string(raw), 512))
		return fmt.Errorf("python chat returned status %d: %s", resp.StatusCode, string(raw))
	}

	utils.LogWithCtx(ctx, "PythonChatClient.ChatStreamEvents", "HTTP 200，TTFB=%dms，开始解析 SSE 流", time.Since(start).Milliseconds())
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	eventCount := 0
	lastTypes := make([]string, 0, 10) // 保留最近 10 条事件类型，便于回看
	typeCounts := make(map[string]int) // 事件类型分布统计
	progressTick := 10                 // 每 N 条打一条进度日志
	totalBytes := 0
	deltaSeq := 0 // delta 帧独立编号，便于查看分片
	deltaTotalLen := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		raw := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		totalBytes += len(line) + 1
		if raw == "" {
			continue
		}
		if raw == "[DONE]" {
			utils.LogWithCtx(ctx, "PythonChatClient.ChatStreamEvents", "收到 [DONE] 结束帧 | totalEvents=%d totalBytes=%d latency=%dms",
				eventCount, totalBytes, time.Since(start).Milliseconds())
			break
		}

		var event dto.ChatEvent
		if err := json.Unmarshal([]byte(raw), &event); err != nil {
			utils.LogWithCtx(ctx, "PythonChatClient.ChatStreamEvents", "SSE帧解析失败 | error=%v raw=%s", err, truncateForLog(raw, 256))
			continue
		}
		eventCount++
		typeCounts[event.Type]++
		if len(lastTypes) >= 10 {
			lastTypes = lastTypes[1:]
		}
		lastTypes = append(lastTypes, event.Type)

		// 打印每条 event 的真实字段内容，便于查看 Python 实际返回
		logStreamEvent(ctx, eventCount, &event, &deltaSeq, &deltaTotalLen)

		if err := handler(event); err != nil {
			utils.LogWithCtx(ctx, "PythonChatClient.ChatStreamEvents", "回调处理失败 | seq=%d type=%s error=%v", eventCount, event.Type, err)
			return err
		}

		// 节流：每 N 条打一条进度，避免 delta 风暴时刷屏
		if eventCount%progressTick == 0 {
			utils.LogWithCtx(ctx, "PythonChatClient.ChatStreamEvents", "进度 | events=%d lastType=%s totalBytes=%d latency=%dms",
				eventCount, event.Type, totalBytes, time.Since(start).Milliseconds())
		}
	}
	if err := scanner.Err(); err != nil {
		utils.LogWithCtx(ctx, "PythonChatClient.ChatStreamEvents", "读取 SSE 流失败 | err=%v events=%d", err, eventCount)
		return fmt.Errorf("read SSE stream failed: %w", err)
	}
	utils.LogWithCtx(ctx, "PythonChatClient.ChatStreamEvents", "SSE 流结束 | totalEvents=%d totalBytes=%d latency=%dms typeCounts=%v deltaTextAccLen=%d",
		eventCount, totalBytes, time.Since(start).Milliseconds(), typeCounts, deltaTotalLen)
	return nil
}

// truncateForLog 限制字符串长度，避免日志刷屏/泄漏大量数据
func truncateForLog(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated,total=" + itoa(len(s)) + ")"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// logStreamEvent 按 ChatEvent.Type 打印真实字段内容，方便核对 Python 返回。
// delta 帧采样：前 3 条 + 每 10 条打一次，避免长流刷屏；累计 text 长度由 deltaTotalLen 输出。
func logStreamEvent(ctx context.Context, seq int, e *dto.ChatEvent, deltaSeq, deltaTotalLen *int) {
	switch e.Type {
	case "context":
		utils.LogWithCtx(ctx, "PythonChatClient.ChatStreamEvents",
			"SSE 帧 #%d type=context | personaLen=%v coreCount=%v l1Count=%v",
			seq, derefInt(e.PersonaLen), derefInt(e.CoreCount), derefInt(e.L1Count))
	case "tool":
		utils.LogWithCtx(ctx, "PythonChatClient.ChatStreamEvents",
			"SSE 帧 #%d type=tool | name=%s iter=%v ok=%v summary=%s",
			seq, e.Name, derefInt(e.Iter), derefBool(e.OK), truncateForLog(e.Summary, 256))
	case "prefix":
		utils.LogWithCtx(ctx, "PythonChatClient.ChatStreamEvents",
			"SSE 帧 #%d type=prefix | text=%s",
			seq, truncateForLog(e.Text, 256))
	case "delta":
		*deltaSeq++
		*deltaTotalLen += len(e.Text)
		// 采样：前 3 条必打，之后每 10 条打一次，便于看分片又不刷屏
		if *deltaSeq <= 3 || *deltaSeq%10 == 0 {
			utils.LogWithCtx(ctx, "PythonChatClient.ChatStreamEvents",
				"SSE 帧 #%d type=delta #delta=%d | snippet=%s textLen=%d accTextLen=%d",
				seq, *deltaSeq, truncateForLog(e.Text, 80), len(e.Text), *deltaTotalLen)
		}
	case "done":
		utils.LogWithCtx(ctx, "PythonChatClient.ChatStreamEvents",
			"SSE 帧 #%d type=done | fullLen=%d full=%s",
			seq, len(e.Full), truncateForLog(e.Full, 1024))
	case "memory_extracted":
		utils.LogWithCtx(ctx, "PythonChatClient.ChatStreamEvents",
			"SSE 帧 #%d type=memory_extracted | ok=%v error=%q",
			seq, derefBool(e.OK), e.Error)
	default:
		// 未知 type：整帧 JSON 兜底打印
		if raw, err := json.Marshal(e); err == nil {
			utils.LogWithCtx(ctx, "PythonChatClient.ChatStreamEvents",
				"SSE 帧 #%d type=%s | payload=%s", seq, e.Type, string(raw))
		}
	}
}

// derefInt 安全的 *int → interface{}（nil 时打印 <nil>）
func derefInt(p *int) interface{} {
	if p == nil {
		return "<nil>"
	}
	return *p
}

// derefBool 安全的 *bool → interface{}（nil 时打印 <nil>）
func derefBool(p *bool) interface{} {
	if p == nil {
		return "<nil>"
	}
	return *p
}
