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

// PythonIngestClient 调用 Python /ingest_file 把外部文档（RAG 知识库）灌入。
// 与 PythonChatClient 共享 baseURL（ECHO_AI_REMOTE_BASE_URL）与 HTTP 客户端。
type PythonIngestClient struct {
	baseURL string
	client  *http.Client
}

// NewPythonIngestClient 构造 Python 入库客户端
func NewPythonIngestClient() *PythonIngestClient {
	baseURL := strings.TrimSpace(os.Getenv("ECHO_AI_REMOTE_BASE_URL"))
	if baseURL == "" {
		baseURL = "http://localhost:8000"
	}
	log.Printf("==== [PythonIngestClient] 初始化 | baseURL=%s timeout=30s ====", baseURL)
	return &PythonIngestClient{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// IngestFile 调用 Python /ingest_file
// 文档要求：userId + file{fileId,fileName,fileKey,url} 必填。
// Python 端后续异步执行 下载→切片→embedding→写入 Weaviate。
func (c *PythonIngestClient) IngestFile(ctx context.Context, req dto.IngestFileRequest) (*dto.IngestFileResponse, error) {
	utils.LogWithCtx(ctx, "PythonIngestClient.IngestFile", "发送请求 | url=%s/ingest_file userId=%s fileId=%s fileName=%s",
		c.baseURL, req.UserID, req.File.FileID, req.File.FileName)

	body, err := json.Marshal(req)
	if err != nil {
		utils.LogWithCtx(ctx, "PythonIngestClient.IngestFile", "序列化请求失败 | err=%v", err)
		return nil, fmt.Errorf("marshal python ingest request failed: %w", err)
	}
	httpReq, err := http.NewRequest("POST", c.baseURL+"/ingest_file", bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("create python ingest request failed: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	start := time.Now()
	resp, err := c.client.Do(httpReq)
	if err != nil {
		utils.LogWithCtx(ctx, "PythonIngestClient.IngestFile", "HTTP 请求失败 | err=%v latency=%dms", err, time.Since(start).Milliseconds())
		return nil, fmt.Errorf("python ingest request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		utils.LogWithCtx(ctx, "PythonIngestClient.IngestFile", "Python 返回非 200 | status=%d latency=%dms body=%s",
			resp.StatusCode, time.Since(start).Milliseconds(), truncateForLog(string(raw), 512))
		return nil, fmt.Errorf("python ingest returned status %d: %s", resp.StatusCode, string(raw))
	}

	rawBody, _ := io.ReadAll(resp.Body)
	var out dto.IngestFileResponse
	if err := json.Unmarshal(rawBody, &out); err != nil {
		utils.LogWithCtx(ctx, "PythonIngestClient.IngestFile", "响应反序列化失败 | err=%v bodyBytes=%d", err, len(rawBody))
		return nil, fmt.Errorf("decode python ingest response failed: %w", err)
	}
	utils.LogWithCtx(ctx, "PythonIngestClient.IngestFile", "Python 响应完成 | status=200 latency=%dms bodyBytes=%d fileId=%s ok=%v queued=%v",
		time.Since(start).Milliseconds(), len(rawBody), out.FileID, out.OK, out.Queued)
	return &out, nil
}
