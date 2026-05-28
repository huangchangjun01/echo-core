package response

// EmbeddingResponse 定义 Python 服务返回的结构
type EmbeddingResponse struct {
	Embedding []float32 `json:"embedding"`
}
