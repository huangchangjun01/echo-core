package handlers

import (
	"echo-core/service"
	"echo-core/service/request"
	"echo-core/utils"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type FileHandler struct {
	service *service.FileService
}

func NewFileHandler() (*FileHandler, error) {
	fileService, err := service.NewFileService()
	if err != nil {
		return nil, err
	}
	return &FileHandler{service: fileService}, nil
}

// GetUploadTokenHandler 获取七牛云上传token (POST)
func (h *FileHandler) GetUploadTokenHandler(c *gin.Context) {
	ctx := c.Request.Context()
	start := time.Now()
	utils.LogWith(c, "File", "GetUploadToken 入口 | method=POST path=%s ip=%s", c.Request.URL.Path, c.ClientIP())

	if h == nil || h.service == nil {
		utils.LogWith(c, "File", "GetUploadToken 服务未初始化")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "file service not initialized"})
		return
	}

	var req request.GetUploadTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.LogWith(c, "File", "GetUploadToken 参数解析失败 | err=%v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}

	result, err := h.service.GetUploadToken(ctx, req.FileName, req.FileSize, req.MimeType, req.BizType)
	if err != nil {
		utils.LogWith(c, "File", "GetUploadToken 失败 | err=%v latency=%dms", err, time.Since(start).Milliseconds())
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	utils.LogWith(c, "File", "GetUploadToken 成功 | key=%s uploadURL=%s latency=%dms",
		result.Key, result.UploadURL, time.Since(start).Milliseconds())
	c.JSON(http.StatusOK, gin.H{
		"token":     result.Token,
		"uploadURL": result.UploadURL,
		"key":       result.Key,
	})
}

// RegisterFileHandler 注册文件信息 (POST)
func (h *FileHandler) RegisterFileHandler(c *gin.Context) {
	ctx := c.Request.Context()
	start := time.Now()
	utils.LogWith(c, "File", "RegisterFile 入口 | method=POST path=%s ip=%s", c.Request.URL.Path, c.ClientIP())

	if h == nil || h.service == nil {
		utils.LogWith(c, "File", "RegisterFile 服务未初始化")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "file service not initialized"})
		return
	}

	var req request.RegisterFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.LogWith(c, "File", "RegisterFile 参数解析失败 | err=%v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}

	result, err := h.service.RegisterFile(ctx, &req)
	if err != nil {
		utils.LogWith(c, "File", "RegisterFile 失败 | err=%v latency=%dms", err, time.Since(start).Milliseconds())
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ingestStatus := "none"
	if result.Ingestion != nil {
		ingestStatus = fmt.Sprintf("ok=%v queued=%v", result.Ingestion.OK, result.Ingestion.Queued)
	}
	utils.LogWith(c, "File", "RegisterFile 成功 | id=%d userId=%s key=%s ingest=%s latency=%dms",
		result.ID, result.UserId, result.Key, ingestStatus, time.Since(start).Milliseconds())
	c.JSON(http.StatusOK, result)
}
