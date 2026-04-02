package service

import (
	"context"

	"github.com/sashabaranov/go-openai"
)

// AgentTool 定义了一个可以被 LLM 调用的工具。利用接口做底层实现抽象。
type AgentTool interface {
	Name() string
	Description() string
	// Definition 返回 OpenAI 需要的 Function 模式定义。利用 jsonschema 做参数描述。
	Definition() openai.FunctionDefinition
	// Execute 执行真正的逻辑
	Execute(ctx context.Context, args string) (string, error)
}

// Registry 维护了所有可用的工具配置
type Registry struct {
	tools map[string]AgentTool
}

func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]AgentTool),
	}
}

func (r *Registry) Register(t AgentTool) {
	r.tools[t.Name()] = t
}

func (r *Registry) GetTool(name string) (AgentTool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// GetOpenAITools 返回供大模型使用的工具列表定义
func (r *Registry) GetOpenAITools() []openai.Tool {
	var openAITools []openai.Tool
	for _, t := range r.tools {
		openAITools = append(openAITools, openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        t.Definition().Name,
				Description: t.Definition().Description,
				Parameters:  t.Definition().Parameters,
			},
		})
	}
	return openAITools
}
