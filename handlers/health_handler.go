package handlers

import (
	"echo-core/dto"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ServiceVersion 本服务对外版本号
// 与 Python 服务（v2.0.0）保持一致，便于统一监控/探针识别。
const ServiceVersion = "2.0.0"

// HealthHandler 存活/就绪探针
// GET /health
// 本地实现，不调用 Python /health；用于 LB/容器探活。
type HealthHandler struct{}

// NewHealthHandler 构造 HealthHandler
func NewHealthHandler() *HealthHandler {
	return &HealthHandler{}
}

// Health GET /health
func (h *HealthHandler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, dto.HealthResponse{
		Status:  "ok",
		Version: ServiceVersion,
	})
}
