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

// PythonClient Python 服务客户端，封装对 Python 服务的流式 HTTP 调用
type PythonClient struct {
	baseURL string
	client  *http.Client
}

// NewPythonClient 创建 Python 服务客户端
func NewPythonClient(baseURL string) *PythonClient {
	log.Printf("[PythonClient] 创建Python客户端 | baseURL: %s", baseURL)
	return &PythonClient{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// PythonChatRequest Python 服务 /chat 接口请求体
type PythonChatRequest struct {
	UserID    string          `json:"userId"`
	SessionID string          `json:"sessionId"`
	Message   string          `json:"message"`
	History   []PythonMessage `json:"history,omitempty"`
}

// PythonMessage Python 服务使用的消息格式
type PythonMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// PythonChatStreamChunk Python 服务流式响应块
type PythonChatStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

// ChatStream 流式调用 Python 服务 /chat 接口
// handler 每收到一个文本片段触发一次；返回错误时终止流
func (c *PythonClient) ChatStream(req PythonChatRequest, handler func(delta string) error) error {
	log.Printf("[PythonClient] ChatStream开始 | userId: %s | sessionId: %s | messageLen: %d | historyLen: %d",
		req.UserID, req.SessionID, len(req.Message), len(req.History))

	jsonData, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request failed: %w", err)
	}

	httpReq, err := http.NewRequest("POST", c.baseURL+"/chat", bytes.NewBuffer(jsonData))
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
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta.Content
		if delta == "" {
			continue
		}
		chunkCount++
		if err := handler(delta); err != nil {
			return err
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read SSE stream failed: %w", err)
	}

	log.Printf("[PythonClient] ChatStream完成 | chunks: %d", chunkCount)
	return nil
}