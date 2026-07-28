package remote

import (
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

// PythonMemoryClient 调用 echo-ai /memory/* 回忆记忆接口。
// 与 PythonIngestClient 共享 baseURL（ECHO_AI_REMOTE_BASE_URL）。
// 解析链路较慢（多模态下载+LLM），但 Python 端为异步 BackgroundTasks 立即返回，
// 故此处超时给到 60s 足够覆盖"接收并排队"的往返。
type PythonMemoryClient struct {
	baseURL string
	client  *http.Client
}

// NewPythonMemoryClient 构造回忆记忆客户端
func NewPythonMemoryClient() *PythonMemoryClient {
	baseURL := strings.TrimSpace(os.Getenv("ECHO_AI_REMOTE_BASE_URL"))
	if baseURL == "" {
		baseURL = "http://localhost:8000"
	}
	log.Printf("==== [PythonMemoryClient] 初始化 | baseURL=%s timeout=60s ====", baseURL)
	return &PythonMemoryClient{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

// ParseMemory 调用 Python /memory/parse 触发记忆源文件解析 + md 生成 + 向量入库
func (c *PythonMemoryClient) ParseMemory(ctx context.Context, req dto.ParseMemoryRequest) (*dto.RecallAsyncResponse, error) {
	return c.post(ctx, "/memory/parse", req,
		fmt.Sprintf("memoryId=%s userId=%s files=%d", req.MemoryID, req.UserID, len(req.SourceFiles)))
}

// DeleteSourceFile 调用 Python /memory/source/delete 更新 md 与向量
func (c *PythonMemoryClient) DeleteSourceFile(ctx context.Context, req dto.DeleteSourceFileRequest) (*dto.RecallAsyncResponse, error) {
	return c.post(ctx, "/memory/source/delete", req,
		fmt.Sprintf("memoryId=%s fileKey=%s", req.MemoryID, req.FileKey))
}

// DeleteMemory 调用 Python /memory/delete 删除向量记录
func (c *PythonMemoryClient) DeleteMemory(ctx context.Context, req dto.DeleteMemoryRequest) (*dto.RecallAsyncResponse, error) {
	return c.post(ctx, "/memory/delete", req,
		fmt.Sprintf("memoryId=%s", req.MemoryID))
}

// ReparseMemory 调用 Python /memory/reparse 重新解析（编辑已有记忆后走此路径）。
//
// 与 ParseMemory 的关键差异：
//   - Python 端会基于 removedFileKeys 提前在 md 中删除对应小节，避免出现"残留片段"。
//   - Python 端先 delete_by_memory_id 再走标准 parse_memory（确保向量被清后重建）。
func (c *PythonMemoryClient) ReparseMemory(ctx context.Context, req dto.ReparseMemoryRequest) (*dto.RecallAsyncResponse, error) {
	return c.post(ctx, "/memory/reparse", req,
		fmt.Sprintf("memoryId=%s userId=%s files=%d removed=%d",
			req.MemoryID, req.UserID, len(req.SourceFiles), len(req.RemovedFileKeys)))
}

// post 统一 POST + 日志 + 反序列化
func (c *PythonMemoryClient) post(ctx context.Context, apiPath string, req interface{}, logKV string) (*dto.RecallAsyncResponse, error) {
	comp := "PythonMemoryClient" + apiPath
	utils.LogWithCtx(ctx, comp, "发送请求 | url=%s%s %s", c.baseURL, apiPath, logKV)

	body, err := json.Marshal(req)
	if err != nil {
		utils.LogWithCtx(ctx, comp, "序列化请求失败 | err=%v", err)
		return nil, fmt.Errorf("marshal memory request failed: %w", err)
	}
	httpReq, err := http.NewRequest("POST", c.baseURL+apiPath, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("create memory request failed: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	start := time.Now()
	resp, err := c.client.Do(httpReq)
	if err != nil {
		utils.LogWithCtx(ctx, comp, "HTTP 请求失败 | err=%v latency=%dms", err, time.Since(start).Milliseconds())
		return nil, fmt.Errorf("memory request failed: %w", err)
	}
	defer resp.Body.Close()

	rawBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		utils.LogWithCtx(ctx, comp, "Python 返回非 200 | status=%d latency=%dms body=%s",
			resp.StatusCode, time.Since(start).Milliseconds(), truncateForLog(string(rawBody), 512))
		return nil, fmt.Errorf("memory api returned status %d: %s", resp.StatusCode, string(rawBody))
	}

	var out dto.RecallAsyncResponse
	if err := json.Unmarshal(rawBody, &out); err != nil {
		utils.LogWithCtx(ctx, comp, "响应反序列化失败 | err=%v bodyBytes=%d", err, len(rawBody))
		return nil, fmt.Errorf("decode memory response failed: %w", err)
	}
	utils.LogWithCtx(ctx, comp, "Python 响应完成 | status=200 latency=%dms ok=%v queued=%v",
		time.Since(start).Milliseconds(), out.OK, out.Queued)
	return &out, nil
}
