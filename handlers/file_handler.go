package handlers

import (
	"echo-core/service/request"
	"echo-core/service"
	"github.com/gin-gonic/gin"
	"log"
	"net/http"
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
	if h == nil || h.service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "file service not initialized"})
		return
	}

	var req request.GetUploadTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}

	log.Printf("[GetUploadTokenHandler] 收到请求: fileName=%s, fileSize=%d, mimeType=%s, bizType=%s",
		req.FileName, req.FileSize, req.MimeType, req.BizType)

	result, err := h.service.GetUploadToken(req.FileName, req.FileSize, req.MimeType, req.BizType)
	if err != nil {
		log.Printf("[GetUploadTokenHandler] 获取token失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[GetUploadTokenHandler] 获取token成功: key=%s", result.Key)
	c.JSON(http.StatusOK, gin.H{
		"token":     result.Token,
		"uploadURL": result.UploadURL,
		"key":       result.Key,
	})
}

// RegisterFileHandler 注册文件信息 (POST)
func (h *FileHandler) RegisterFileHandler(c *gin.Context) {
	if h == nil || h.service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "file service not initialized"})
		return
	}

	log.Printf("[RegisterFileHandler] 收到请求")

	var req request.RegisterFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[RegisterFileHandler] 参数解析失败: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}

	log.Printf("[RegisterFileHandler] 解析请求成功: userId=%s, fileName=%s, key=%s, fileType=%d, bizType=%d",
		req.UserId, req.FileName, req.Key, req.FileType, req.BizType)

	result, err := h.service.RegisterFile(&req)
	if err != nil {
		log.Printf("[RegisterFileHandler] 注册文件失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[RegisterFileHandler] 注册文件成功: id=%d", result.ID)
	c.JSON(http.StatusOK, result)
}
