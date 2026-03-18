package remote

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
)

// EmbeddingResponse 定义 Python 服务返回的结构
type EmbeddingResponse struct {
	Embedding []float32 `json:"embedding"`
}

// EchoRemote 定义 Python 服务返回的结构
type EchoRemote struct {
}

// NewEchoRemote 创建一个新的 EchoRemote 实例
func NewEchoRemote() *EchoRemote {
	return &EchoRemote{}
}

// GetImageEmbedding 调用 Python 服务获取图片向量
func (s *EchoRemote) GetImageEmbedding(imageData []byte) ([]float32, error) {
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
	req, err := http.NewRequest("POST", "http://localhost:8000/embedding", body)
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
	var embeddingResp EmbeddingResponse
	err = json.Unmarshal(respBody, &embeddingResp)
	if err != nil {
		return nil, err
	}

	return embeddingResp.Embedding, nil
}
