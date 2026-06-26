package remote

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// PythonClient Python Echo-AI 服务客户端，封装对 Python 服务的 HTTP 调用
type PythonClient struct {
	baseURL string
	client  *http.Client
}

// NewPythonClient 创建 Python Echo-AI 服务客户端
func NewPythonClient(baseURL string) *PythonClient {
	log.Printf("[PythonClient] 创建Python客户端 | baseURL: %s", baseURL)
	return &PythonClient{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// PythonMessage Python Echo-AI 服务使用的消息格式（OpenAI 兼容）
type PythonMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// PythonChatCompletionsRequest /v1/chat/completions 接口请求体
type PythonChatCompletionsRequest struct {
	Messages []PythonMessage `json:"messages"`
	UserID   string          `json:"userId"`
	Stream   bool            `json:"stream"`
	K        int             `json:"k,omitempty"`
}

// PythonChatStreamChunk Python 服务 /v1/chat/completions 流式响应块
// Echo-AI 的 SSE 格式为 data: {"token": "文本片段"}，而非 OpenAI 标准格式
type PythonChatStreamChunk struct {
	Token string `json:"token"`
}

// ChatStream 流式调用 Python Echo-AI 服务 /v1/chat/completions 接口
// 该接口内置 ReAct 循环（推理→工具调用→观察→推理），流式输出最终回复的 token
// handler 每收到一个 token 触发一次；返回错误时终止流
func (c *PythonClient) ChatStream(req PythonChatCompletionsRequest, handler func(delta string) error) error {
	log.Printf("[PythonClient] ChatStream开始 | userId: %s | messagesCount: %d | stream: %v",
		req.UserID, len(req.Messages), req.Stream)

	jsonData, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request failed: %w", err)
	}

	httpReq, err := http.NewRequest("POST", c.baseURL+"/v1/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("create request failed: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[PythonClient] Python服务返回错误 | status: %d | body: %s", resp.StatusCode, string(body))
		return fmt.Errorf("python service returned status %d: %s", resp.StatusCode, string(body))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	chunkCount := 0

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			if payload == "[DONE]" {
				break
			}
			continue
		}

		var chunk PythonChatStreamChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			log.Printf("[PythonClient] SSE帧解析失败 | error: %v | payload: %s", err, payload)
			continue
		}
		if chunk.Token == "" {
			continue
		}
		chunkCount++
		if err := handler(chunk.Token); err != nil {
			return err
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read SSE stream failed: %w", err)
	}

	log.Printf("[PythonClient] ChatStream完成 | chunks: %d", chunkCount)
	return nil
}

// ChatCompletions 非流式调用 Python Echo-AI 服务 /v1/chat/completions 接口
// 返回 OpenAI 兼容的完整响应 JSON
func (c *PythonClient) ChatCompletions(req PythonChatCompletionsRequest) (map[string]interface{}, error) {
	log.Printf("[PythonClient] ChatCompletions开始 | userId: %s | messagesCount: %d",
		req.UserID, len(req.Messages))

	req.Stream = false
	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request failed: %w", err)
	}

	httpReq, err := http.NewRequest("POST", c.baseURL+"/v1/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[PythonClient] Python服务返回错误 | status: %d | body: %s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("python service returned status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response failed: %w", err)
	}

	log.Printf("[PythonClient] ChatCompletions完成")
	return result, nil
}