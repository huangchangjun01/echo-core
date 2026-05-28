package remote

import (
	"bytes"
	"echo-core/remote/request"
	"echo-core/remote/response"
	"echo-core/utils"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
)

// VectorRemote 定义 Python 服务返回的结构
type VectorRemote struct {
	client  *http.Client
	baseURL string
}

// NewVectorRemote 创建一个新的 VectorRemote 实例
func NewVectorRemote() *VectorRemote {
	return &VectorRemote{
		client:  &http.Client{},
		baseURL: utils.GetEnv("ECHO_AI_REMOTE_BASE_URL", ""),
	}
}

// GetImageEmbedding 调用 Python 服务获取图片向量
func (s *VectorRemote) GetImageEmbedding(imageData []byte) ([]float32, error) {
	// 创建一个 multipart 表单
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// 创建表单文件字段
	part, err := writer.CreateFormFile("file", "image.jpg")
	if err != nil {
		return nil, err
	}
	// 写入图片数据
	_, err = part.Write(imageData)
	if err != nil {
		return nil, err
	}
	writer.Close() // 必须关闭以写入结尾boundary

	// 创建 HTTP 请求
	req, err := http.NewRequest("POST", s.baseURL+"/embedding", body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// 发送请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 读取响应
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("python service returned error: %s", string(respBody))
	}

	// 解析 JSON
	var embeddingResp response.EmbeddingResponse
	err = json.Unmarshal(respBody, &embeddingResp)
	if err != nil {
		return nil, err
	}

	return embeddingResp.Embedding, nil
}

// GetTextEmbedding sends text to the remote service and returns its vector embedding.
func (r *VectorRemote) GetTextEmbedding(text string) ([]float32, error) {
	if r.client == nil {
		return nil, errors.New("vector remote client not initialized")
	}

	payload := map[string]string{"text": text}
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal text embedding payload: %w", err)
	}

	req, err := http.NewRequest("POST", r.baseURL+"/text-embedding", bytes.NewBuffer(jsonPayload))
	if err != nil {
		return nil, fmt.Errorf("failed to create text embedding request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("text embedding request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("text embedding service returned status: %s", resp.Status)
	}

	var embeddingResp struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&embeddingResp); err != nil {
		return nil, fmt.Errorf("failed to decode text embedding response: %w", err)
	}

	return embeddingResp.Embedding, nil
}

// IngestFile 调用 Python /ingest_file 接口，存储文件向量
func (r *VectorRemote) IngestFile(req *request.IngestFileRequest) error {
	if r.client == nil {
		return errors.New("vector remote client not initialized")
	}

	jsonPayload, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal ingest file payload: %w", err)
	}

	resp, err := r.client.Post(r.baseURL+"/ingest_file", "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("ingest file request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ingest file service returned status: %s, body: %s", resp.Status, string(body))
	}

	return nil
}
