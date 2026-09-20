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
	RoleID    string `json:"roleId,omitempty"`
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
//
// ctx 用于控制 HTTP 请求的生命周期:调用方(通常是 gin c.Request.Context())
// 取消时,正在进行的请求会被立刻中止,远端 echo-ai 也会通过 r.Context() 收到
// 断开信号,避免 SSE/同步请求在客户端已经断开后还在空跑。
func (c *PythonChatClient) ChatSync(ctx context.Context, req PythonChatRequest) (*dto.ChatSyncResponse, error) {
	utils.LogWithCtx(ctx, "PythonChatClient.ChatSync", "发送请求 | url=%s/chat stream=false userId=%s sessionId=%s msgLen=%d",
		c.baseURL, req.UserID, req.SessionID, len(req.Message))

	body, err := json.Marshal(req)
	if err != nil {
		utils.LogWithCtx(ctx, "PythonChatClient.ChatSync", "序列化请求失败 | err=%v", err)
		return nil, fmt.Errorf("marshal python chat request failed: %w", err)
	}
	// 必须用 NewRequestWithContext 把 ctx 绑到 HTTP 请求上,
	// 否则客户端断开(gin cancel)时 Python 端还要跑满 120s 超时。
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat", bytes.NewBuffer(body))
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
	// Python 同步响应里 events[] 也可能是扁平 stage 帧,先整体规范化。
	var out dto.ChatSyncResponse
	if err := json.Unmarshal(normalizePythonSyncBody(rawBody), &out); err != nil {
		utils.LogWithCtx(ctx, "PythonChatClient.ChatSync", "响应反序列化失败 | err=%v bodyBytes=%d", err, len(rawBody))
		return nil, fmt.Errorf("decode python chat response failed: %w", err)
	}
	utils.LogWithCtx(ctx, "PythonChatClient.ChatSync", "Python 响应完成 | status=200 latency=%dms bodyBytes=%d events=%d replyLen=%d",
		time.Since(start).Milliseconds(), len(rawBody), len(out.Events), len(out.Reply))
	return &out, nil
}

// normalizePythonFrame 把 Python 实际产出的 SSE 帧重写成 v2 wire-spec 期望的形状。
//
// 目前只处理一类：stage 事件。Python biz/chat.py 的 `_stage()` 构造器输出扁平形
// `{type, name, state, ts_ms, ...extras}`，而前端 spec 要求嵌套形
// `{type, stage: {name, state, ts_ms, ...extras}}`。直接把扁平 JSON 塞进
// ChatEvent 会发生两个问题：
//  1. ChatEvent.Name（tool 事件使用）会被误赋值为阶段名，导致 tool 帧后续解析错乱。
//  2. ChatEvent.Stage *StageEvent 永远为 nil，前端 parseStageEvent 拿到 null。
//
// 处理策略：检测到 `{type:"stage"}` 且顶层无 `stage` 字段时，把 name/state/ts_ms
// 移到一个嵌套 `stage` 对象里；其它顶层字段（如 tool_dispatch 的 iter/tool）保留
// 在顶层（前端不消费也不污染 schema）。其它事件类型原样返回。
//
// 入参失败 / 类型不匹配 / 形状异常均原样返回 raw，避免在解析失败时把帧彻底吞掉。
func normalizePythonFrame(raw []byte) []byte {
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil || probe.Type != "stage" {
		return raw
	}
	var generic map[string]json.RawMessage
	if err := json.Unmarshal(raw, &generic); err != nil {
		return raw
	}
	// 已经嵌套 → 原样返回,防止误把 stage 字段再次覆盖。
	if _, hasNested := generic["stage"]; hasNested {
		return raw
	}
	stage := make(map[string]json.RawMessage, 3)
	for _, key := range []string{"name", "state", "ts_ms"} {
		if v, ok := generic[key]; ok {
			stage[key] = v
			delete(generic, key)
		}
	}
	if len(stage) == 0 {
		return raw
	}
	stageJSON, err := json.Marshal(stage)
	if err != nil {
		return raw
	}
	generic["stage"] = stageJSON
	out, err := json.Marshal(generic)
	if err != nil {
		return raw
	}
	return out
}

// normalizePythonSyncBody 对 Python 同步响应整段 JSON 应用 normalizePythonFrame:
// 解析顶层 `events` 数组,逐个规范化,再回写。这样 ChatSync 路径
// (json.Unmarshal → ChatSyncResponse.Events)也能拿到正确嵌套的 stage 帧。
func normalizePythonSyncBody(raw []byte) []byte {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return raw
	}
	eventsRaw, ok := m["events"]
	if !ok {
		return raw
	}
	var events []json.RawMessage
	if err := json.Unmarshal(eventsRaw, &events); err != nil {
		return raw
	}
	changed := false
	for i, ev := range events {
		normalized := normalizePythonFrame(ev)
		if string(normalized) != string(ev) {
			events[i] = normalized
			changed = true
		}
	}
	if !changed {
		return raw
	}
	normalizedEvents, err := json.Marshal(events)
	if err != nil {
		return raw
	}
	m["events"] = normalizedEvents
	out, err := json.Marshal(m)
	if err != nil {
		return raw
	}
	return out
}

// ChatStreamEvents 流式调用 Python /chat（stream=true）
// handler 收到每条 ChatEvent（type 已在结构体中）。流结束或出错时返回。
//
// ctx 用于控制 HTTP 请求的生命周期:调用方(通常是 gin c.Request.Context(),
// 即浏览器 EventSource 客户端连接断开时会自动 cancel)取消时,正在进行的
// HTTP 请求会被立刻中止,远端 echo-ai 也会通过 r.Context() 感知到客户端断开,
// 停止生成后续 delta 事件 — 避免 echo-ai 在客户端已经断开后还在空跑
// 直到 http.Client.Timeout(120s) 触发。这是任务 2.3 的核心修复点。
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

	// 必须用 NewRequestWithContext 把 ctx 绑到 HTTP 请求上。
	// 否则即便调用方已经 cancel(浏览器断开 EventSource),
	// 这个 POST 还会一直挂着,直到 http.Client.Timeout(120s) 兜底,
	// 远端 echo-ai 也无从感知客户端已断开。
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat", bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("create python chat request failed: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	start := time.Now()
	resp, err := c.client.Do(httpReq)
	if err != nil {
		utils.LogWithCtx(ctx, "PythonChatClient.ChatStreamEvents", "HTTP 请求失败 | err=%v latency=%dms", err, time.Since(start).Milliseconds())
		// ctx 取消导致的网络错误,直接返回 ctx.Err() 让上游(handler)能用 ctx.Err() 判断
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
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
		// 把 Python 实际产出的扁平 stage 帧转成 wire-spec 嵌套形,
		// 避免 ChatEvent.Name 与 ChatEvent.Stage *StageEvent 字段冲突。
		if err := json.Unmarshal(normalizePythonFrame([]byte(raw)), &event); err != nil {
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
		// ctx 取消 → resp.Body 被关闭 → scanner 拿到 io.ErrClosedPipe / 类似错误,
		// 这种情况下真正的根因是 ctx 取消,把它透传给上游(handler)
		// 以便上层直接判断「客户端断开」,不要再写一条 error SSE 帧给前端。
		if ctxErr := ctx.Err(); ctxErr != nil {
			utils.LogWithCtx(ctx, "PythonChatClient.ChatStreamEvents", "SSE 流被调用方取消 | events=%d ctxErr=%v scannerErr=%v",
				eventCount, ctxErr, err)
			return ctxErr
		}
		utils.LogWithCtx(ctx, "PythonChatClient.ChatStreamEvents", "读取 SSE 流失败 | err=%v events=%d", err, eventCount)
		return fmt.Errorf("read SSE stream failed: %w", err)
	}
	// scanner 正常 EOF 后,如果 ctx 已经被外部取消(例如客户端断开),
	// 也按 ctx 取消处理,避免下游误以为是「正常结束」。
	if ctxErr := ctx.Err(); ctxErr != nil {
		utils.LogWithCtx(ctx, "PythonChatClient.ChatStreamEvents", "SSE 流结束但 ctx 已取消 | events=%d ctxErr=%v",
			eventCount, ctxErr)
		return ctxErr
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
			"SSE 帧 #%d type=context | intent=%s intentSource=%s intentMs=%.2f personaLen=%v l0Count=%v l1Count=%v l2Count=%v",
			seq, e.Intent, e.IntentSource, e.IntentMs,
			derefInt(e.PersonaLen), derefInt(e.L0Count), derefInt(e.L1Count), derefInt(e.L2Count))
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
	case "thinking":
		utils.LogWithCtx(ctx, "PythonChatClient.ChatStreamEvents",
			"SSE 帧 #%d type=thinking | think_stage=%s text=%s",
			seq, e.ThinkStage, truncateForLog(e.Text, 256))
	case "memory_recall":
		utils.LogWithCtx(ctx, "PythonChatClient.ChatStreamEvents",
			"SSE 帧 #%d type=memory_recall | count=%d hits=%s",
			seq, e.RecallCount, truncateForLog(serializeHits(e.Hits), 512))
	case "stage":
		stageName, stageState, stageTs := "<nil>", "<nil>", int64(0)
		if e.Stage != nil {
			stageName, stageState, stageTs = e.Stage.Name, e.Stage.State, e.Stage.TsMs
		}
		utils.LogWithCtx(ctx, "PythonChatClient.ChatStreamEvents",
			"SSE 帧 #%d type=stage | name=%s state=%s ts_ms=%d flatName=%q",
			seq, stageName, stageState, stageTs, e.Name)
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

// serializeHits 把回忆命中数组序列化为简短 JSON 字符串，便于日志展示。
func serializeHits(hits []dto.RecallHitItem) string {
	if len(hits) == 0 {
		return "[]"
	}
	b, err := json.Marshal(hits)
	if err != nil {
		return fmt.Sprintf("[serialize_err=%v]", err)
	}
	return string(b)
}
