package remote

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"echo-core/dto"
)

// TestPythonChatClient_StreamEvents_CancelOnContextDone 验证：调用 ChatStreamEvents
// 的 ctx 被取消后,正在进行的 HTTP 请求会立即终止(远端 echo-ai 也应观察到自己
// 的 r.Context() 被取消),而不是继续跑满 http.Client.Timeout。
//
// 这个测试是任务 2.3「客户端断开 → 取消 Python 请求」的核心回归保护:
// 一旦哪天有人把 NewRequest 换回 NewRequest(无 ctx),这个测试就会失败。
func TestPythonChatClient_StreamEvents_CancelOnContextDone(t *testing.T) {
	var requestStarted atomic.Bool
	var serverObservedCancel atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestStarted.Store(true)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		// 模拟 echo-ai 持续产出 — 只要 ctx 没取消就一直挂着,
		// ctx 取消意味着 echo-core 已经断开 → 我们要确认 echo-ai
		// 立刻感知到客户端断开。
		select {
		case <-r.Context().Done():
			serverObservedCancel.Store(true)
		case <-time.After(3 * time.Second):
			t.Logf("mock python server safety timeout (3s)")
		}
	}))
	defer srv.Close()

	// 通过 env 把客户端的 baseURL 指到本地 mock server
	t.Setenv("ECHO_AI_REMOTE_BASE_URL", srv.URL)

	client := NewPythonChatClient()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handlerCalled := atomic.Int32{}
	errCh := make(chan error, 1)
	go func() {
		err := client.ChatStreamEvents(ctx, PythonChatRequest{
			UserID:    "u-cancel",
			SessionID: "s-cancel",
			Message:   "hi",
			Stream:    true,
		}, func(_ dto.ChatEvent) error {
			handlerCalled.Add(1)
			return nil
		})
		errCh <- err
	}()

	// 等待请求真的到达 Python mock
	waitFor := func(pred func() bool, label string, total time.Duration) bool {
		t.Helper()
		deadline := time.Now().Add(total)
		for time.Now().Before(deadline) {
			if pred() {
				return true
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("timed out waiting for %s after %s", label, total)
		return false
	}
	waitFor(requestStarted.Load, "request to reach mock server", 2*time.Second)

	// 模拟客户端断开(等价于 SSE browser close → gin.Request.Context cancel)
	cancel()

	waitFor(serverObservedCancel.Load, "mock python server to observe ctx cancel", 2*time.Second)

	// ChatStreamEvents 应该在 ctx 取消后及时返回,
	// 否则就是 http.Client.Timeout 在拖时间(默认 120s)。
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected ctx.Canceled after ctx cancel, got %v (handler called %d times)", err, handlerCalled.Load())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ChatStreamEvents did not return within 2s after ctx cancel — HTTP request likely not bound to ctx")
	}
}

// TestNormalizePythonFrame_StageFlat 验证: Python 扁平 stage 帧
// {type, name, state, ts_ms} 被改写为 wire-spec 嵌套形
// {type, stage:{name, state, ts_ms}}。
//
// 这是 Phase 4 streaming UX 修复的核心:
// 1. 扁平形会让 ChatEvent.Name(本应是 tool.name)被误赋值为阶段名;
// 2. ChatEvent.Stage *StageEvent 永远为 nil,前端 parseStageEvent 拿到 null。
func TestNormalizePythonFrame_StageFlat(t *testing.T) {
	in := []byte(`{"type":"stage","name":"intent","state":"start","ts_ms":1737038400123}`)
	out := normalizePythonFrame(in)

	var env struct {
		Type  string `json:"type"`
		Name  string `json:"name"`
		State string `json:"state"`
		TsMs  int64  `json:"ts_ms"`
		Stage *struct {
			Name  string `json:"name"`
			State string `json:"state"`
			TsMs  int64  `json:"ts_ms"`
		} `json:"stage"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("normalized frame must unmarshal cleanly: %v", err)
	}
	if env.Type != "stage" {
		t.Errorf("type must round-trip, got %q", env.Type)
	}
	if env.Stage == nil {
		t.Fatal("stage object must be present after normalization")
	}
	if env.Stage.Name != "intent" || env.Stage.State != "start" || env.Stage.TsMs != 1737038400123 {
		t.Errorf("stage fields lost or wrong: name=%q state=%q ts_ms=%d",
			env.Stage.Name, env.Stage.State, env.Stage.TsMs)
	}
	if env.Name != "" {
		t.Errorf("top-level name must be removed (was %q)", env.Name)
	}

	// 进一步验证能正确填到 dto.ChatEvent(回归 ChatEvent.Name vs Stage 冲突)
	var ev dto.ChatEvent
	if err := json.Unmarshal(out, &ev); err != nil {
		t.Fatalf("ChatEvent unmarshal: %v", err)
	}
	if ev.Type != "stage" {
		t.Errorf("ChatEvent.Type must be stage, got %q", ev.Type)
	}
	if ev.Name != "" {
		t.Errorf("ChatEvent.Name must be empty (was %q) — otherwise tool-frame collision persists", ev.Name)
	}
	if ev.Stage == nil {
		t.Fatal("ChatEvent.Stage must be populated")
	}
	if ev.Stage.Name != "intent" || ev.Stage.State != "start" || ev.Stage.TsMs != 1737038400123 {
		t.Errorf("ChatEvent.Stage fields wrong: %+v", ev.Stage)
	}
}

// TestNormalizePythonFrame_StageFlatWithExtras 验证: Python stage 帧的
// 业务扩展字段(iter/tool 等)在规范化后仍保留在顶层,不会被无声吞掉。
// 这覆盖了 _stage(StageName.TOOL_DISPATCH, StageState.START, iter=0, tool="x") 的形态。
func TestNormalizePythonFrame_StageFlatWithExtras(t *testing.T) {
	in := []byte(`{"type":"stage","name":"tool_dispatch","state":"start","ts_ms":1234,"iter":0,"tool":"search_memory"}`)
	out := normalizePythonFrame(in)

	var env map[string]any
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env["type"] != "stage" {
		t.Errorf("type must be stage, got %v", env["type"])
	}
	stage, ok := env["stage"].(map[string]any)
	if !ok {
		t.Fatalf("stage must be an object, got %T", env["stage"])
	}
	if stage["name"] != "tool_dispatch" || stage["state"] != "start" {
		t.Errorf("stage fields lost: %+v", stage)
	}
	// extras 必须保留在顶层(它们对前端没意义,但 Python 业务需要)
	if env["iter"] == nil || env["tool"] == nil {
		t.Errorf("top-level extras must be preserved: iter=%v tool=%v", env["iter"], env["tool"])
	}
}

// TestNormalizePythonFrame_StageAlreadyNested 验证: 已嵌套的 stage 帧不会被二次改写
// (防止某些不规范实现 + relay 双重嵌套导致 wire 污染)。
func TestNormalizePythonFrame_StageAlreadyNested(t *testing.T) {
	in := []byte(`{"type":"stage","stage":{"name":"intent","state":"start","ts_ms":1737038400123}}`)
	out := normalizePythonFrame(in)
	if string(out) != string(in) {
		t.Errorf("already-nested stage must pass through unchanged\n got: %s\nwant: %s", out, in)
	}
}

// TestNormalizePythonFrame_NonStagePassthrough 验证: 非 stage 帧原样返回,
// 不会因错误归一化而污染 tool/context/delta 等其它类型。
func TestNormalizePythonFrame_NonStagePassthrough(t *testing.T) {
	cases := []string{
		`{"type":"tool","name":"search_memory","iter":1,"ok":true,"summary":"hit"}`,
		`{"type":"context","intent":"chat","intent_source":"rule_greeting","intent_ms":0.0,"persona_len":106,"l0_count":14,"l1_count":8}`,
		`{"type":"delta","text":"hi"}`,
		`not-json`,
	}
	for _, in := range cases {
		out := normalizePythonFrame([]byte(in))
		if string(out) != in {
			t.Errorf("non-stage frame must pass through unchanged\n got: %s\nwant: %s", out, in)
		}
	}
}

// TestNormalizePythonSyncBody_NormalizesEventsArray 验证: 同步响应里的
// events[] 数组被逐个规范化,只有 stage 帧被改写,其它帧原样保留。
func TestNormalizePythonSyncBody_NormalizesEventsArray(t *testing.T) {
	in := []byte(`{
		"reply":"hi",
		"events":[
			{"type":"context","intent":"chat","intent_source":"rule_greeting","intent_ms":0.0,"persona_len":106,"l0_count":14,"l1_count":8},
			{"type":"stage","name":"intent","state":"start","ts_ms":1737038400123},
			{"type":"delta","text":"hello"}
		]
	}`)
	out := normalizePythonSyncBody(in)

	var resp struct {
		Reply  string         `json:"reply"`
		Events []dto.ChatEvent `json:"events"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("sync body unmarshal: %v", err)
	}
	if len(resp.Events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(resp.Events))
	}
	// context 帧应当保留 intent/intent_source/intent_ms
	if resp.Events[0].Type != "context" {
		t.Fatalf("event[0] type=%q want context", resp.Events[0].Type)
	}
	if resp.Events[0].Intent != "chat" || resp.Events[0].IntentSource != "rule_greeting" {
		t.Errorf("event[0] intent fields lost: intent=%q intent_source=%q",
			resp.Events[0].Intent, resp.Events[0].IntentSource)
	}
	if resp.Events[0].IntentMs != 0.0 {
		t.Errorf("event[0] intent_ms wrong: %v", resp.Events[0].IntentMs)
	}
	// stage 帧应当被规范化为嵌套形
	if resp.Events[1].Type != "stage" {
		t.Fatalf("event[1] type=%q want stage", resp.Events[1].Type)
	}
	if resp.Events[1].Name != "" {
		t.Errorf("event[1] Name must be empty (collision fix), got %q", resp.Events[1].Name)
	}
	if resp.Events[1].Stage == nil {
		t.Fatal("event[1] Stage must be populated")
	}
	if resp.Events[1].Stage.Name != "intent" || resp.Events[1].Stage.State != "start" {
		t.Errorf("event[1] Stage fields wrong: %+v", resp.Events[1].Stage)
	}
	// delta 帧必须原样保留 text
	if resp.Events[2].Type != "delta" || resp.Events[2].Text != "hello" {
		t.Errorf("event[2] delta corrupted: %+v", resp.Events[2])
	}
}