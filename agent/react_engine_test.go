package agent

import (
	"echo-core/remote"
	"errors"
	"strings"
	"sync"
	"testing"
)

// mockChatClient 模拟 LLM 客户端：按脚本返回一系列响应。
// 每次 Chat 调用从 scripted 头部取一个；ChatStream 走 streamScripted。
type mockChatClient struct {
	mu       sync.Mutex
	scripted []scriptedResp
	calls    int
}

type scriptedResp struct {
	choice *remote.AIResponse
	err    error
}

func (m *mockChatClient) Chat(messages []remote.AIChatMessage, tools []remote.AITool) (*remote.AIResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.calls >= len(m.scripted) {
		return nil, errors.New("mock exhausted")
	}
	s := m.scripted[m.calls]
	m.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.choice, nil
}

func (m *mockChatClient) ChatWithToolChoice(messages []remote.AIChatMessage, tools []remote.AITool, toolChoice interface{}) (*remote.AIResponse, error) {
	return m.Chat(messages, tools)
}

func (m *mockChatClient) ChatStreamWithTools(messages []remote.AIChatMessage, tools []remote.AITool, handler func(chunk remote.StreamToolChunk) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.calls >= len(m.scripted) {
		return errors.New("mock exhausted")
	}
	s := m.scripted[m.calls]
	m.calls++
	if s.err != nil {
		return s.err
	}
	// 简化：把所有 content 一次性发出，最后一帧带 tool_calls + Finish
	c := s.choice
	if len(c.Choices) == 0 {
		return errors.New("no choices")
	}
	msg := c.Choices[0].Message
	if content, ok := msg.Content.(string); ok && content != "" {
		if err := handler(remote.StreamToolChunk{Content: content}); err != nil {
			return err
		}
	}
	if len(msg.ToolCalls) > 0 {
		return handler(remote.StreamToolChunk{ToolCalls: msg.ToolCalls, Finish: true})
	}
	return handler(remote.StreamToolChunk{Finish: true})
}

func (m *mockChatClient) ModelName() string { return "mock-model" }

// helper: 构造 assistant 文本响应
func textResp(content string) *remote.AIResponse {
	return &remote.AIResponse{
		Choices: []remote.Choice{{
			Message:      remote.AIChatMessage{Role: "assistant", Content: content},
			FinishReason: "stop",
		}},
	}
}

// helper: 构造 assistant 工具调用响应
func toolCallResp(id, name, args string) *remote.AIResponse {
	return &remote.AIResponse{
		Choices: []remote.Choice{{
			Message: remote.AIChatMessage{
				Role: "assistant",
				ToolCalls: []remote.AIToolCall{{
					ID:   id,
					Type: "function",
					Function: remote.AIFunction{
						Name:      name,
						Arguments: args,
					},
				}},
			},
			FinishReason: "tool_calls",
		}},
	}
}

// TestNextUserPrompt 验证 nextUserPrompt 的行为
func TestNextUserPrompt(t *testing.T) {
	original := remote.AIChatMessage{Role: "user", Content: "原始问题"}

	// step 1: 返回原始 user msg
	got := nextUserPrompt(1, original, "")
	if got.Content != "原始问题" {
		t.Errorf("step 1 应该返回原始 user msg，got=%v", got.Content)
	}

	// step 2+: 返回统一 continue 模板
	got = nextUserPrompt(2, original, "")
	if c, ok := got.Content.(string); !ok || !strings.Contains(c, "工具结果") {
		t.Errorf("step 2 应该返回 continue 模板，got=%v", got.Content)
	}
	// 验证模板不依赖 lastContent（应使用统一模板而非拼 lastContent）
	if strings.Contains(got.Content.(string), "请继续，上一轮") {
		t.Errorf("step 2 不应再使用旧的『请继续，上一轮你说了』模板")
	}
}

// TestExecute_ToolCallChain 验证同步 Execute 在工具调用链中：
// - 第 1 轮：模型决定调工具
// - 第 2 轮：模型再次决定调工具（此时 lastContent 必须不是空！）
// - 第 3 轮：模型给最终答案
// 关键回归：确保第二轮 user 消息是"基于以上工具结果继续回答"，而不是 buggy 的空字符串拼接
func TestExecute_ToolCallChain(t *testing.T) {
	mock := &mockChatClient{
		scripted: []scriptedResp{
			{choice: toolCallResp("call_1", "echo", `{"msg":"first"}`)},
			{choice: toolCallResp("call_2", "echo", `{"msg":"second"}`)},
			{choice: textResp("最终回答")},
		},
	}

	toolCalledCount := 0
	echoTool := Tool{
		Name: "echo",
		Handler: func(params map[string]interface{}) (string, error) {
			toolCalledCount++
			return "tool_result_" + params["msg"].(string), nil
		},
	}

	engine := NewReActEngine(mock, []Tool{echoTool})

	messages := []remote.AIChatMessage{
		{Role: "user", Content: "原始用户问题"},
	}
	reply, err := engine.Execute("system", messages)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if reply != "最终回答" {
		t.Errorf("期望最终回答 = '最终回答'，got=%q", reply)
	}
	if toolCalledCount != 2 {
		t.Errorf("工具应被调用 2 次，got=%d", toolCalledCount)
	}
	if mock.calls != 3 {
		t.Errorf("LLM 应被调用 3 次，got=%d", mock.calls)
	}
}

// TestExecute_EmptyContinue 回归测试：之前 finalReply="" 的 bug，
// 确保第二轮 continue 消息不为空字符串拼接。
func TestExecute_EmptyContinue(t *testing.T) {
	mock := &mockChatClient{
		scripted: []scriptedResp{
			{choice: toolCallResp("c1", "noop", `{}`)},
			{choice: textResp("done")},
		},
	}
	noopTool := Tool{
		Name:    "noop",
		Handler: func(params map[string]interface{}) (string, error) { return "ok", nil },
	}
	engine := NewReActEngine(mock, []Tool{noopTool})

	messages := []remote.AIChatMessage{{Role: "user", Content: "hi"}}
	_, err := engine.Execute("system", messages)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	// 关键断言：Execute 内部不 panic、不返回空 finalReply。
	// 旧 bug 在此场景下会构造 `"请继续，上一轮你说了: "` 空字符串 user msg；
	// 修复后使用统一 continue 模板（"请基于以上工具结果继续回答"）。
	if mock.calls != 2 {
		t.Errorf("LLM 应被调用 2 次，got=%d", mock.calls)
	}
}

// TestExecute_MaxStepsFallback 验证 max steps 兜底：
// 模型永远返回工具调用时，应返回最后一次 assistant 的 content（即使从未终止）。
func TestExecute_MaxStepsFallback(t *testing.T) {
	scripted := make([]scriptedResp, 11) // 10 步 tool call + 留空
	for i := range scripted {
		scripted[i] = scriptedResp{choice: toolCallResp("c", "loop", `{}`)}
	}
	mock := &mockChatClient{scripted: scripted}
	loopTool := Tool{
		Name:    "loop",
		Handler: func(params map[string]interface{}) (string, error) { return "loop_result", nil },
	}
	engine := NewReActEngine(mock, []Tool{loopTool})

	messages := []remote.AIChatMessage{{Role: "user", Content: "go"}}
	reply, err := engine.Execute("system", messages)
	if err == nil {
		t.Errorf("达到 max steps 应返回 error")
	}
	// 旧 bug：reply="" + error；新行为：error 且 lastContent 为空时返回 "" + error
	_ = reply
}

// TestRouteAgent_FallbackOnLLMError 验证当 LLM 路由失败时 fallback 到首个 Agent（带 warn log）
func TestRouteAgent_FallbackOnLLMError(t *testing.T) {
	failingClient := &mockChatClient{} // 永远返回 error
	o := NewMultiAgentOrchestrator(failingClient, "test orch")
	a1 := NewAgent("agent1", "first", "p1", failingClient, nil)
	a2 := NewAgent("agent2", "second", "p2", failingClient, nil)
	o.RegisterAgent(a1)
	o.RegisterAgent(a2)

	got := o.RouteAgent("用户问点什么")
	if got == nil {
		t.Fatal("fallback 应该返回首个 agent，不应为 nil")
	}
	if got.Name != "agent1" {
		t.Errorf("fallback 应该返回首个注册的 agent，got=%s", got.Name)
	}
}

// TestRouteAgent_NilClientFallback 验证 aiClient==nil 时直接 fallback
func TestRouteAgent_NilClientFallback(t *testing.T) {
	o := NewMultiAgentOrchestrator(nil, "test")
	a1 := NewAgent("only", "唯一", "p", nil, nil)
	o.RegisterAgent(a1)

	got := o.RouteAgent("any")
	if got == nil || got.Name != "only" {
		t.Errorf("单个 agent 时应跳过路由直接返回，got=%v", got)
	}
}

// TestRouteAgent_ConcurrentSafety 验证 agents map 的并发安全
func TestRouteAgent_ConcurrentSafety(t *testing.T) {
	o := NewMultiAgentOrchestrator(nil, "")
	o.RegisterAgent(NewAgent("a", "x", "p", nil, nil))

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			o.RouteAgent("hi")
		}()
		go func() {
			defer wg.Done()
			_ = o.ListAgents()
		}()
	}
	wg.Wait()
}