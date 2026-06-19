package agent

import (
	"echo-core/remote"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
)

// Tool 工具定义
type Tool struct {
	Name        string                                              `json:"name"`
	Description string                                              `json:"description"`
	Parameters  map[string]interface{}                              `json:"parameters"`
	Handler     func(params map[string]interface{}) (string, error) `json:"-"`
	// MetaHandler 可选；当工具除了返回文本结果还需要透出结构化元数据
	// （例如 search_knowledge 命中文件的 fileId / fileName / 下载链接）时实现本回调。
	// ReActEngine 优先调用 MetaHandler，并把 attachments 写入 ToolExecutionEvent，
	// 由上层 service 透传给前端。未实现时回退到 Handler。
	MetaHandler func(params map[string]interface{}) (string, []Attachment, error) `json:"-"`
}

// ChatClient 是 ReActEngine 与 LLM 通信所需的最小接口。
// 实际由 *remote.AIClient 实现；测试里可注入 mock 来覆盖 ReAct 行为。
type ChatClient interface {
	Chat(messages []remote.AIChatMessage, tools []remote.AITool) (*remote.AIResponse, error)
	ChatStreamWithTools(messages []remote.AIChatMessage, tools []remote.AITool, handler func(chunk remote.StreamToolChunk) error) error
	ChatWithToolChoice(messages []remote.AIChatMessage, tools []remote.AITool, toolChoice interface{}) (*remote.AIResponse, error)
	ModelName() string
}

// ReActEngine ReAct模式引擎
type ReActEngine struct {
	aiClient ChatClient
	tools    map[string]*Tool
	maxSteps int
}

// NewReActEngine 创建ReAct引擎
func NewReActEngine(aiClient ChatClient, tools []Tool) *ReActEngine {
	toolMap := make(map[string]*Tool)
	for i := range tools {
		t := tools[i]
		toolMap[t.Name] = &t
	}
	log.Printf("[ReActEngine] 引擎创建完成 | tools_count: %d", len(toolMap))
	return &ReActEngine{
		aiClient: aiClient,
		tools:    toolMap,
		maxSteps: 10,
	}
}

// AddTool 添加工具
func (e *ReActEngine) AddTool(tool Tool) {
	e.tools[tool.Name] = &tool
	log.Printf("[ReActEngine] 工具添加 | tool_name: %s", tool.Name)
}

// Execute 执行ReAct循环
// 历史签名保留：只返回最终回复文本；若需要同时拿到工具命中的 attachments
// （例如 search_knowledge 的下载链接），请使用 ExecuteWithMeta。
func (e *ReActEngine) Execute(systemPrompt string, messages []remote.AIChatMessage) (string, error) {
	reply, _, err := e.ExecuteWithMeta(systemPrompt, messages)
	return reply, err
}

// ExecuteWithMeta 与 Execute 等价，但额外聚合所有工具调用过程中产生的 attachments。
// 同步链路 (/api/chat) 通过本函数把 attachments 一路传给上层，让前端可以直接
// 渲染下载入口；前提是工具实现了 MetaHandler。未实现 MetaHandler 的工具返回空切片。
func (e *ReActEngine) ExecuteWithMeta(systemPrompt string, messages []remote.AIChatMessage) (string, []Attachment, error) {
	log.Printf("[ReActEngine] Execute开始 | messages_count: %d", len(messages))

	if len(messages) == 0 {
		log.Printf("[ReActEngine] 消息为空")
		return "", nil, errors.New("messages cannot be empty")
	}

	// 确保最后一条是用户消息
	lastMsg := messages[len(messages)-1]
	if lastMsg.Role != "user" {
		log.Printf("[ReActEngine] 最后一条不是用户消息 | role: %s", lastMsg.Role)
		return "", nil, errors.New("last message must be user role")
	}

	// 构建初始上下文
	log.Printf("[ReActEngine] 构建初始上下文")
	context := make([]remote.AIChatMessage, 0, len(messages)+10)
	context = append(context, remote.AIChatMessage{Role: "system", Content: systemPrompt})

	// 添加历史消息
	for i := 0; i < len(messages)-1; i++ {
		context = append(context, messages[i])
	}
	log.Printf("[ReActEngine] 上下文构建完成 | context_size: %d", len(context))

	// 收集工具定义
	var toolDefs []remote.AITool
	for _, t := range e.tools {
		toolDefs = append(toolDefs, remote.AITool{
			Type: "function",
			Function: remote.AIFunctionDef{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		})
	}
	log.Printf("[ReActEngine] 工具定义收集完成 | tools_count: %d", len(toolDefs))

	// ReAct循环
	var lastContent string
	var allAttachments []Attachment
	step := 0

	for step < e.maxSteps {
		step++
		log.Printf("[ReActEngine] ReAct循环 Step %d/%d", step, e.maxSteps)

		// 构建当前轮次的 user 消息：首轮用原 user msg；后续轮次用"继续"模板驱动 AI
		currentUserMsg := nextUserPrompt(step, messages[len(messages)-1], lastContent)

		// 添加上下文
		execMessages := make([]remote.AIChatMessage, len(context))
		copy(execMessages, context)
		execMessages = append(execMessages, currentUserMsg)

		log.Printf("[ReActEngine] 调用AI | execMessages: %d", len(execMessages))
		// 调用AI
		resp, err := e.aiClient.Chat(execMessages, toolDefs)
		if err != nil {
			log.Printf("[ReActEngine] AI调用失败 | error: %v", err)
			return "", nil, fmt.Errorf("AI call failed: %w", err)
		}
		log.Printf("[ReActEngine] AI调用成功")

		if len(resp.Choices) == 0 {
			log.Printf("[ReActEngine] AI响应choices为空")
			return "", nil, errors.New("no response choices")
		}

		choice := resp.Choices[0]
		assistantMsg := choice.Message

		// 记录助手消息
		context = append(context, assistantMsg)
		log.Printf("[ReActEngine] 助手消息已记录 | tool_calls_count: %d", len(assistantMsg.ToolCalls))

		// 记录上次 assistant 文本，供下一轮 continue 消息使用
		if c, ok := assistantMsg.Content.(string); ok {
			lastContent = c
		}

		// 检查是否有工具调用
		if len(assistantMsg.ToolCalls) > 0 {
			log.Printf("[ReActEngine] 检测到工具调用 | tool_calls_count: %d", len(assistantMsg.ToolCalls))
			// 执行工具调用
			for _, tc := range assistantMsg.ToolCalls {
				toolName := tc.Function.Name
				args := tc.Function.Arguments
				log.Printf("[ReActEngine] 执行工具调用 | tool_name: %s | args: %s", toolName, args)

				tool, exists := e.tools[toolName]
				if !exists {
					obs := fmt.Sprintf("Error: tool %s not found", toolName)
					log.Printf("[ReActEngine] 工具不存在 | tool_name: %s", toolName)
					context = append(context, remote.AIChatMessage{
						Role:       "tool",
						Content:    obs,
						ToolCallID: tc.ID,
					})
					continue
				}

				// 解析参数
				var params map[string]interface{}
				if err := json.Unmarshal([]byte(args), &params); err != nil {
					obs := fmt.Sprintf("Error: failed to parse arguments for %s: %v", toolName, err)
					log.Printf("[ReActEngine] 参数解析失败 | tool_name: %s | error: %v", toolName, err)
					context = append(context, remote.AIChatMessage{
						Role:       "tool",
						Content:    obs,
						ToolCallID: tc.ID,
					})
					continue
				}

				// 执行工具：优先 MetaHandler 取 attachments
				log.Printf("[ReActEngine] 执行工具 | tool_name: %s | params: %v", toolName, params)
				var (
					result      string
					attachments []Attachment
					err         error
				)
				if tool.MetaHandler != nil {
					result, attachments, err = tool.MetaHandler(params)
				} else {
					result, err = tool.Handler(params)
				}
				if err != nil {
					log.Printf("[ReActEngine] 工具执行出错 | tool_name: %s | error: %v", toolName, err)
					result = fmt.Sprintf("Error executing %s: %v", toolName, err)
				}
				if len(attachments) > 0 {
					allAttachments = append(allAttachments, attachments...)
				}
				log.Printf("[ReActEngine] 工具执行完成 | tool_name: %s | result_len: %d | attachments: %d", toolName, len(result), len(attachments))

				// 将结果注入上下文
				context = append(context, remote.AIChatMessage{
					Role:       "tool",
					Content:    result,
					ToolCallID: tc.ID,
				})
			}
			log.Printf("[ReActEngine] 工具调用循环完成，继续ReAct")
			continue
		}

		// 没有工具调用，返回最终回复
		if content, ok := assistantMsg.Content.(string); ok && content != "" {
			log.Printf("[ReActEngine] 无工具调用，返回最终回复 | reply_len: %d | attachments: %d", len(content), len(allAttachments))
			return content, allAttachments, nil
		}

		// 检查finish_reason
		if choice.FinishReason == "stop" || choice.FinishReason == "length" {
			if content, ok := assistantMsg.Content.(string); ok && content != "" {
				log.Printf("[ReActEngine] finish_reason: %s | reply_len: %d", choice.FinishReason, len(content))
				return content, allAttachments, nil
			}
		}
	}

	// 兜底：达到 maxSteps 时返回最后一次 assistant 内容，避免空字符串误导调用方
	log.Printf("[ReActEngine] 达到最大循环次数 | max_steps: %d | last_content_len: %d", e.maxSteps, len(lastContent))
	if lastContent != "" {
		return lastContent, allAttachments, nil
	}
	return "", allAttachments, errors.New("max steps reached without assistant content")
}

// nextUserPrompt 构造 ReAct 下一轮的 user 消息
// step == 1: 返回原始 user 消息
// step >  1: 返回统一的"继续"模板（lastContent 仅用于日志/调试，不进入 prompt）
// 该函数被 Execute 与 ExecuteStream 共用，确保两条链路行为一致。
func nextUserPrompt(step int, originalUserMsg remote.AIChatMessage, lastContent string) remote.AIChatMessage {
	if step <= 1 {
		return originalUserMsg
	}
	_ = lastContent // 当前模板不依赖 lastContent；保留参数便于未来切换"续说"策略
	return remote.AIChatMessage{
		Role:    "user",
		Content: "请基于以上工具结果继续回答。",
	}
}

// ExecuteWithHistory 基于历史消息执行
func (e *ReActEngine) ExecuteWithHistory(history []remote.AIChatMessage, currentInput string) (string, []remote.AIChatMessage, error) {
	log.Printf("[ReActEngine] ExecuteWithHistory | history_count: %d | input_len: %d", len(history), len(currentInput))

	// 构建带历史的完整消息列表
	messages := make([]remote.AIChatMessage, 0, len(history)+1)
	for _, h := range history {
		messages = append(messages, h)
	}
	messages = append(messages, remote.AIChatMessage{Role: "user", Content: currentInput})

	// 执行
	reply, err := e.Execute("", messages)
	if err != nil {
		log.Printf("[ReActEngine] ExecuteWithHistory Execute失败 | error: %v", err)
		return "", nil, err
	}

	// 更新历史
	history = append(history, remote.AIChatMessage{Role: "user", Content: currentInput})
	history = append(history, remote.AIChatMessage{Role: "assistant", Content: reply})

	log.Printf("[ReActEngine] ExecuteWithHistory完成 | reply_len: %d", len(reply))
	return reply, history, nil
}

// ToolExecutionEvent 单条工具调用的执行结果事件（ReAct 流式执行时逐条回调）
type ToolExecutionEvent struct {
	ToolCall    remote.AIToolCall
	ToolResult  string
	Attachments []Attachment
	Err         error
}

// ExecuteStream 流式 ReAct 循环
// 与 Execute 行为等价（同样的 ReAct 步骤、参数解析、错误处理、context 注入），
// 区别仅在于 AI 调用走 ChatStreamWithTools：每轮 AI 响应中的 content 增量
// 通过 onContent 回调外抛；工具执行结果通过 onToolEvent 回调外抛。
//
// 任一回调返回非 nil 错误会立即中断流。
func (e *ReActEngine) ExecuteStream(
	systemPrompt string,
	messages []remote.AIChatMessage,
	onContent func(delta string) error,
	onToolEvent func(event ToolExecutionEvent) error,
) (string, error) {
	log.Printf("[ReActEngine] ExecuteStream开始 | messages_count: %d", len(messages))

	if len(messages) == 0 {
		log.Printf("[ReActEngine] 消息为空")
		return "", errors.New("messages cannot be empty")
	}

	// 确保最后一条是用户消息
	lastMsg := messages[len(messages)-1]
	if lastMsg.Role != "user" {
		log.Printf("[ReActEngine] 最后一条不是用户消息 | role: %s", lastMsg.Role)
		return "", errors.New("last message must be user role")
	}

	// 构建初始上下文：system + 历史（不含最后一条 user）
	context := make([]remote.AIChatMessage, 0, len(messages)+10)
	context = append(context, remote.AIChatMessage{Role: "system", Content: systemPrompt})
	for i := 0; i < len(messages)-1; i++ {
		context = append(context, messages[i])
	}
	log.Printf("[ReActEngine] 上下文构建完成 | context_size: %d", len(context))

	// 工具定义
	var toolDefs []remote.AITool
	for _, t := range e.tools {
		toolDefs = append(toolDefs, remote.AITool{
			Type: "function",
			Function: remote.AIFunctionDef{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		})
	}
	log.Printf("[ReActEngine] 工具定义收集完成 | tools_count: %d", len(toolDefs))

	// ReAct 循环
	var lastContent string
	step := 0
	for step < e.maxSteps {
		step++
		log.Printf("[ReActEngine] ReAct Stream 循环 Step %d/%d", step, e.maxSteps)

		// 构造本轮 user 消息：与 Execute 同步链路共用同一个 nextUserPrompt
		currentUserMsg := nextUserPrompt(step, messages[len(messages)-1], lastContent)

		execMessages := make([]remote.AIChatMessage, len(context))
		copy(execMessages, context)
		execMessages = append(execMessages, currentUserMsg)
		log.Printf("[ReActEngine] 调用流式AI | execMessages: %d | tools: %d", len(execMessages), len(toolDefs))

		var (
			fullReply strings.Builder
			toolCalls []remote.AIToolCall
		)
		streamErr := e.aiClient.ChatStreamWithTools(execMessages, toolDefs, func(chunk remote.StreamToolChunk) error {
			if chunk.Content != "" {
				fullReply.WriteString(chunk.Content)
				if onContent != nil {
					return onContent(chunk.Content)
				}
			}
			// 仅在流结束帧上接受 tool_calls（与 AIClient.ChatStreamWithTools 的契约：
			// 工具调用只在 Finish=true 的 chunk 里一次性回吐）。非 finish 帧忽略，
			// 防止未来切到增量推送协议时被中途部分状态污染。
			if chunk.Finish && len(chunk.ToolCalls) > 0 {
				toolCalls = chunk.ToolCalls
			}
			return nil
		})
		if streamErr != nil {
			log.Printf("[ReActEngine] AI流式调用失败 | error: %v", streamErr)
			return "", fmt.Errorf("AI call failed: %w", streamErr)
		}

		// 记录上一次 assistant 文本，供下一轮 continue 消息使用
		if fullReply.Len() > 0 {
			lastContent = fullReply.String()
		}

		// 把助手消息（含可能的 tool_calls）写入 context
		assistantMsg := remote.AIChatMessage{
			Role:      "assistant",
			Content:   fullReply.String(),
			ToolCalls: toolCalls,
		}
		context = append(context, assistantMsg)
		log.Printf("[ReActEngine] 助手消息已记录 | tool_calls_count: %d", len(assistantMsg.ToolCalls))

		// 没有工具调用 → 完成
		if len(toolCalls) == 0 {
			reply := fullReply.String()
			if reply != "" {
				log.Printf("[ReActEngine] 无工具调用，返回最终回复 | reply_len: %d", len(reply))
				return reply, nil
			}
			log.Printf("[ReActEngine] 无工具调用且无文本，跳出等待下一轮")
		}

		// 执行工具
		log.Printf("[ReActEngine] 检测到工具调用 | tool_calls_count: %d", len(toolCalls))
		for _, tc := range toolCalls {
			toolName := tc.Function.Name
			args := tc.Function.Arguments
			log.Printf("[ReActEngine] 执行工具调用 | tool_name: %s | args: %s", toolName, args)

			tool, exists := e.tools[toolName]
			var result string
			var attachments []Attachment
			var execErr error
			if !exists {
				result = fmt.Sprintf("Error: tool %s not found", toolName)
				log.Printf("[ReActEngine] 工具不存在 | tool_name: %s", toolName)
			} else {
				var params map[string]interface{}
				if err := json.Unmarshal([]byte(args), &params); err != nil {
					result = fmt.Sprintf("Error: failed to parse arguments for %s: %v", toolName, err)
					log.Printf("[ReActEngine] 参数解析失败 | tool_name: %s | error: %v", toolName, err)
				} else {
					log.Printf("[ReActEngine] 执行工具 | tool_name: %s | params: %v", toolName, params)
					// 优先使用 MetaHandler（能携带 attachments 等结构化元数据）
					if tool.MetaHandler != nil {
						result, attachments, execErr = tool.MetaHandler(params)
					} else {
						result, execErr = tool.Handler(params)
					}
					if execErr != nil {
						log.Printf("[ReActEngine] 工具执行出错 | tool_name: %s | error: %v", toolName, execErr)
						result = fmt.Sprintf("Error executing %s: %v", toolName, execErr)
					} else {
						log.Printf("[ReActEngine] 工具执行完成 | tool_name: %s | result_len: %d | attachments: %d", toolName, len(result), len(attachments))
					}
				}
			}

			// 回调到 service 层（用于 SSE/WS 透出）
			if onToolEvent != nil {
				if err := onToolEvent(ToolExecutionEvent{
					ToolCall:    tc,
					ToolResult:  result,
					Attachments: attachments,
					Err:         execErr,
				}); err != nil {
					return "", err
				}
			}

			// 工具结果注入 context（对齐 Execute 行为）
			context = append(context, remote.AIChatMessage{
				Role:       "tool",
				Content:    result,
				ToolCallID: tc.ID,
			})
		}
		log.Printf("[ReActEngine] 工具调用循环完成，继续ReAct")
	}

	log.Printf("[ReActEngine] 达到最大循环次数 | max_steps: %d", e.maxSteps)
	return "", errors.New("max steps reached")
}

// Agent Agent定义
type Agent struct {
	Name        string
	Description string
	Prompt      string
	Tools       []Tool
	Engine      *ReActEngine
}

// NewAgent 创建Agent
func NewAgent(name, description, prompt string, aiClient ChatClient, tools []Tool) *Agent {
	log.Printf("[Agent] 创建Agent | name: %s | tools_count: %d", name, len(tools))
	return &Agent{
		Name:        name,
		Description: description,
		Prompt:      prompt,
		Tools:       tools,
		Engine:      NewReActEngine(aiClient, tools),
	}
}

// Run 运行Agent
// 返回最终回复文本。若同步链路需要附件信息，请使用 RunWithMeta。
func (a *Agent) Run(messages []remote.AIChatMessage) (string, error) {
	reply, _, err := a.RunWithMeta(messages)
	return reply, err
}

// RunWithMeta 同步运行 Agent，并额外返回所有工具命中的 attachments
func (a *Agent) RunWithMeta(messages []remote.AIChatMessage) (string, []Attachment, error) {
	log.Printf("[Agent] Run | name: %s | messages_count: %d", a.Name, len(messages))
	reply, attachments, err := a.Engine.ExecuteWithMeta(a.Prompt, messages)
	if err != nil {
		log.Printf("[Agent] Run失败 | name: %s | error: %v", a.Name, err)
		return "", nil, err
	}
	log.Printf("[Agent] Run完成 | name: %s | reply_len: %d | attachments: %d", a.Name, len(reply), len(attachments))
	return reply, attachments, nil
}

// RunStream 流式运行 Agent
func (a *Agent) RunStream(
	messages []remote.AIChatMessage,
	onContent func(delta string) error,
	onToolEvent func(event ToolExecutionEvent) error,
) (string, error) {
	log.Printf("[Agent] RunStream | name: %s | messages_count: %d", a.Name, len(messages))
	reply, err := a.Engine.ExecuteStream(a.Prompt, messages, onContent, onToolEvent)
	if err != nil {
		log.Printf("[Agent] RunStream失败 | name: %s | error: %v", a.Name, err)
		return "", err
	}
	log.Printf("[Agent] RunStream完成 | name: %s | reply_len: %d", a.Name, len(reply))
	return reply, nil
}

// MultiAgentOrchestrator 多Agent编排器
type MultiAgentOrchestrator struct {
	mu         sync.RWMutex
	agents     map[string]*Agent
	orchPrompt string
	aiClient   ChatClient
}

// NewMultiAgentOrchestrator 创建编排器
// aiClient 用于在 RouteAgent 中做 function-calling 路由决策：
// 注册 Agent 时若尚未持有 aiClient，可传 nil 并在 RegisterAgent 完成后通过
// SetRouterClient 注入；为简化使用，构造时一并传入更直观。
func NewMultiAgentOrchestrator(aiClient ChatClient, orchPrompt string) *MultiAgentOrchestrator {
	log.Printf("[Orchestrator] 创建编排器 | prompt_len: %d | has_ai_client: %v", len(orchPrompt), aiClient != nil)
	return &MultiAgentOrchestrator{
		agents:     make(map[string]*Agent),
		orchPrompt: orchPrompt,
		aiClient:   aiClient,
	}
}

// SetRouterClient 在 NewMultiAgentOrchestrator 时未传入 aiClient 的情况下注入
func (o *MultiAgentOrchestrator) SetRouterClient(aiClient ChatClient) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.aiClient = aiClient
}

// RegisterAgent 注册Agent（并发安全）
func (o *MultiAgentOrchestrator) RegisterAgent(agent *Agent) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.agents[agent.Name] = agent
	log.Printf("[Orchestrator] Agent注册 | name: %s | total_agents: %d", agent.Name, len(o.agents))
}

// GetAgent 获取Agent（并发安全）
func (o *MultiAgentOrchestrator) GetAgent(name string) *Agent {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.agents[name]
}

// ListAgents 列出所有Agent（并发安全；返回拷贝避免外部修改 map）
func (o *MultiAgentOrchestrator) ListAgents() []string {
	o.mu.RLock()
	defer o.mu.RUnlock()
	names := make([]string, 0, len(o.agents))
	for name := range o.agents {
		names = append(names, name)
	}
	return names
}

// RouteAgent 根据用户输入选 Agent
// 优先走 LLM function-calling 路由器（详见 routeWithLLM）；失败时回退到第一个注册的 Agent。
// 无可用 Agent 时返回 nil。
//
// 实现注意：map 遍历必须在锁内完成；LLM 路由与具体 Agent 选择也都基于同一份快照，
// 避免外部并发注册导致数据竞争。
func (o *MultiAgentOrchestrator) RouteAgent(userInput string) *Agent {
	// 1) 在锁内做 agents 的快照 + 判断空
	o.mu.RLock()
	if len(o.agents) == 0 {
		o.mu.RUnlock()
		log.Printf("[Orchestrator] RouteAgent 无可用Agent | input: %s", userInput)
		return nil
	}
	if len(o.agents) == 1 {
		for _, agent := range o.agents {
			o.mu.RUnlock()
			log.Printf("[Orchestrator] RouteAgent 唯一Agent跳过路由 | agent: %s | input: %s", agent.Name, userInput)
			return agent
		}
	}
	// 复制 description/name 列表（routeWithLLM 内部仅读 snapshot）
	snapshot := make(map[string]*Agent, len(o.agents))
	for k, v := range o.agents {
		snapshot[k] = v
	}
	aiClient := o.aiClient
	o.mu.RUnlock()

	// 2) LLM 路由（不再持有锁）
	if aiClient != nil {
		name, err := o.routeWithLLM(userInput, snapshot)
		if err != nil {
			log.Printf("[Orchestrator] RouteAgent LLM路由失败，使用兜底Agent | error: %v | input: %s", err, userInput)
		} else if agent, ok := snapshot[name]; ok {
			log.Printf("[Orchestrator] RouteAgent LLM路由命中 | agent: %s | input: %s", name, userInput)
			return agent
		} else {
			log.Printf("[Orchestrator] RouteAgent LLM返回未知Agent | name: %s | input: %s", name, userInput)
		}
	} else {
		log.Printf("[Orchestrator] RouteAgent 未配置aiClient，跳过LLM路由 | input: %s", userInput)
	}

	// 3) 兜底：第一个注册的 Agent（按 map 迭代顺序，不保证但足以兜底）
	for _, agent := range snapshot {
		log.Printf("[Orchestrator] RouteAgent 兜底首个Agent | agent: %s | input: %s", agent.Name, userInput)
		return agent
	}
	return nil
}

// routeWithLLM 通过 LLM function-calling 选择 Agent
// agents 参数由调用方传入（一般是 RouteAgent 持有的锁内快照），避免本函数访问 o.agents map。
// 构造一个名为 select_agent 的工具，其 name 字段 enum 由已注册 Agent 动态生成；
// 强制 tool_choice=required 确保模型必须给出选择。
func (o *MultiAgentOrchestrator) routeWithLLM(userInput string, agents map[string]*Agent) (string, error) {
	// 1. 构造 Agent 描述
	agentNames := make([]string, 0, len(agents))
	agentList := make([]string, 0, len(agents))
	for name, agent := range agents {
		agentNames = append(agentNames, name)
		agentList = append(agentList, fmt.Sprintf("- %s: %s", name, agent.Description))
	}

	// 2. 构造 select_agent 工具定义（enum 由已注册 Agent 名称动态填充）
	selectTool := remote.AITool{
		Type: "function",
		Function: remote.AIFunctionDef{
			Name:        "select_agent",
			Description: "根据用户问题选择最合适的 Agent 处理。必须从 enum 中选择一个 Agent 名称。",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"enum":        agentNames,
						"description": "选中的 Agent 名称",
					},
				},
				"required": []interface{}{"name"},
			},
		},
	}

	// 3. 构造消息
	systemMsg := remote.AIChatMessage{
		Role: "system",
		Content: o.orchPrompt + "\n\n可用 Agent 列表：\n" + strings.Join(agentList, "\n") +
			"\n\n请调用 select_agent 工具，从 enum 中选择一个最合适的 Agent。",
	}
	userMsg := remote.AIChatMessage{
		Role:    "user",
		Content: userInput,
	}

	// 4. 调用 LLM，强制 tool_choice=required
	log.Printf("[Orchestrator] routeWithLLM 调用LLM选Agent | input_len: %d | agents: %v", len(userInput), agentNames)
	resp, err := o.aiClient.ChatWithToolChoice(
		[]remote.AIChatMessage{systemMsg, userMsg},
		[]remote.AITool{selectTool},
		"required",
	)
	if err != nil {
		return "", fmt.Errorf("router LLM call failed: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", errors.New("router LLM returned no choices")
	}

	// 5. 解析 tool_call
	choice := resp.Choices[0]
	if len(choice.Message.ToolCalls) == 0 {
		return "", errors.New("router LLM did not call select_agent (tool_choice=required was set)")
	}
	tc := choice.Message.ToolCalls[0]
	if tc.Function.Name != "select_agent" {
		return "", fmt.Errorf("router LLM called unexpected tool: %s", tc.Function.Name)
	}

	var args struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		return "", fmt.Errorf("parse router tool args: %w (raw: %s)", err, tc.Function.Arguments)
	}
	if args.Name == "" {
		return "", errors.New("router LLM returned empty agent name")
	}

	log.Printf("[Orchestrator] routeWithLLM 选中 | name: %s | input: %s", args.Name, userInput)
	return args.Name, nil
}

// RunStream 流式编排执行：先按 RouteAgent 选 Agent，再以流式方式运行
// history 不应包含当前 user msg（调用方负责把 user msg 追加到 messages 末尾）。
func (o *MultiAgentOrchestrator) RunStream(
	userInput string,
	history []remote.AIChatMessage,
	onContent func(delta string) error,
	onToolEvent func(event ToolExecutionEvent) error,
) (string, error) {
	log.Printf("[Orchestrator] RunStream开始 | user_input: %s | history_count: %d | available_agents: %d", userInput, len(history), len(o.agents))

	agent := o.RouteAgent(userInput)
	if agent == nil {
		log.Printf("[Orchestrator] RunStream 无可用Agent")
		return "", errors.New("no agents registered")
	}

	// 构造完整消息列表：history + 当前 user msg
	fullMessages := make([]remote.AIChatMessage, 0, len(history)+1)
	fullMessages = append(fullMessages, history...)
	fullMessages = append(fullMessages, remote.AIChatMessage{Role: "user", Content: userInput})

	reply, err := agent.RunStream(fullMessages, onContent, onToolEvent)
	if err != nil {
		log.Printf("[Orchestrator] RunStream Agent.RunStream失败 | agent: %s | error: %v", agent.Name, err)
		return "", err
	}

	log.Printf("[Orchestrator] RunStream完成 | agent: %s | reply_len: %d", agent.Name, len(reply))
	return reply, nil
}

// RunSync 同步编排执行：先按 RouteAgent 选 Agent，再以同步方式运行
// 历史兼容保留：替代已删除的 Orchestrate 同步链路。调用方负责把 user msg 追加到
// history 末尾（与 RunStream 行为一致）。
// 返回 (回复, 选中Agent名, 错误)。若需要附件信息，使用 RunSyncWithMeta。
func (o *MultiAgentOrchestrator) RunSync(userInput string, history []remote.AIChatMessage) (string, string, error) {
	reply, name, _, err := o.RunSyncWithMeta(userInput, history)
	return reply, name, err
}

// RunSyncWithMeta 同步编排执行，并返回工具命中的 attachments
func (o *MultiAgentOrchestrator) RunSyncWithMeta(userInput string, history []remote.AIChatMessage) (string, string, []Attachment, error) {
	log.Printf("[Orchestrator] RunSync开始 | user_input: %s | history_count: %d", userInput, len(history))

	agent := o.RouteAgent(userInput)
	if agent == nil {
		return "", "", nil, errors.New("no agents registered")
	}

	fullMessages := make([]remote.AIChatMessage, 0, len(history)+1)
	fullMessages = append(fullMessages, history...)
	fullMessages = append(fullMessages, remote.AIChatMessage{Role: "user", Content: userInput})

	reply, attachments, err := agent.RunWithMeta(fullMessages)
	if err != nil {
		log.Printf("[Orchestrator] RunSync Agent.Run失败 | agent: %s | error: %v", agent.Name, err)
		return "", "", nil, err
	}
	log.Printf("[Orchestrator] RunSync完成 | agent: %s | reply_len: %d | attachments: %d", agent.Name, len(reply), len(attachments))
	return reply, agent.Name, attachments, nil
}
