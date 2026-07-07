package dto

// HealthResponse GET /health 响应
// 与 Python 服务对外契约一致，便于统一探针。
type HealthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}
