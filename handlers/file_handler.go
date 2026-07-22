package handlers

import (
	"echo-core/middleware"
	"echo-core/service"
	"echo-core/service/request"
	"echo-core/utils"
	"fmt"
	"net/http"
	"strconv"
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

	utils.LogWith(c, "File", "GetUploadToken 成功 | key=%s domain=%s uploadURL=%s latency=%dms",
		result.Key, result.Domain, result.UploadURL, time.Since(start).Milliseconds())
	c.JSON(http.StatusOK, gin.H{
		"token":     result.Token,
		"uploadURL": result.UploadURL,
		"key":       result.Key,
		"domain":    result.Domain,
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

	// userId 一律从已鉴权的 session 取，不再信任请求体里的 userId 字段：
	//   - 防前端伪造他人 userId 越权上传
	//   - 防前端 authStore 还没就绪时 userId 为空串触发 binding 校验
	userId, ok := middleware.MustUserID(c)
	if !ok || userId == "" {
		utils.LogWith(c, "File", "RegisterFile 未取到 session userId | ip=%s", c.ClientIP())
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "未登录"})
		return
	}

	var req request.RegisterFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.LogWith(c, "File", "RegisterFile 参数解析失败 | err=%v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}

	result, err := h.service.RegisterFile(ctx, userId, &req)
	if err != nil {
		utils.LogWith(c, "File", "RegisterFile 失败 | err=%v latency=%dms", err, time.Since(start).Milliseconds())
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ingestStatus := "none"
	if result.Ingestion != nil {
		ingestStatus = fmt.Sprintf("ok=%v queued=%v", result.Ingestion.OK, result.Ingestion.Queued)
	}
	utils.LogWith(c, "File", "RegisterFile 成功 | id=%d userId=%s key=%s roleId=%s descLen=%d ingest=%s latency=%dms",
		result.ID, result.UserId, result.Key, req.RoleId, len(req.Desc), ingestStatus, time.Since(start).Milliseconds())
	c.JSON(http.StatusOK, result)
}

// ListMemoryFilesHandler 记忆管理：列出当前角色下的文件 (GET)
// GET /api/file/list?roleId=&fileType=
func (h *FileHandler) ListMemoryFilesHandler(c *gin.Context) {
	ctx := c.Request.Context()
	start := time.Now()
	utils.LogWith(c, "File", "ListMemoryFiles 入口 | method=GET path=%s", c.Request.URL.Path)

	userId, ok := middleware.MustUserID(c)
	if !ok || userId == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "未登录"})
		return
	}
	roleId := c.Query("roleId")
	fileTypeStr := c.DefaultQuery("fileType", "0")
	fileType, _ := strconv.Atoi(fileTypeStr)
	utils.LogWith(c, "File", "ListMemoryFiles 入参 | userId=%s roleId=%s fileType=%d", userId, roleId, fileType)

	items, err := h.service.ListMemoryFiles(ctx, userId, roleId, fileType)
	if err != nil {
		utils.LogWith(c, "File", "ListMemoryFiles 失败 | err=%v latency=%dms", err, time.Since(start).Milliseconds())
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}
	utils.LogWith(c, "File", "ListMemoryFiles 成功 | count=%d latency=%dms", len(items), time.Since(start).Milliseconds())
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": items})
}

// UpdateFileDescHandler 修改文件描述 (PUT)
// PUT /api/file/:id/desc
func (h *FileHandler) UpdateFileDescHandler(c *gin.Context) {
	ctx := c.Request.Context()
	start := time.Now()
	utils.LogWith(c, "File", "UpdateFileDesc 入口 | method=PUT path=%s", c.Request.URL.Path)

	userId, ok := middleware.MustUserID(c)
	if !ok || userId == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "未登录"})
		return
	}
	idStr := c.Param("id")
	id64, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || id64 == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "id 非法"})
		return
	}
	id := uint(id64)

	var req request.UpdateFileDescRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误: " + err.Error()})
		return
	}
	utils.LogWith(c, "File", "UpdateFileDesc 入参 | userId=%s id=%d descLen=%d", userId, id, len(req.Desc))

	if err := h.service.UpdateDesc(ctx, userId, id, req.Desc); err != nil {
		utils.LogWith(c, "File", "UpdateFileDesc 失败 | id=%d err=%v latency=%dms", id, err, time.Since(start).Milliseconds())
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}
	utils.LogWith(c, "File", "UpdateFileDesc 成功 | id=%d latency=%dms", id, time.Since(start).Milliseconds())
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok"})
}

// CreateTextMemoryHandler 新建纯文本记忆 (POST)
// POST /api/file/text
func (h *FileHandler) CreateTextMemoryHandler(c *gin.Context) {
	ctx := c.Request.Context()
	start := time.Now()
	utils.LogWith(c, "File", "CreateTextMemory 入口 | method=POST path=%s", c.Request.URL.Path)

	userId, ok := middleware.MustUserID(c)
	if !ok || userId == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "未登录"})
		return
	}
	var req request.CreateTextMemoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误: " + err.Error()})
		return
	}
	utils.LogWith(c, "File", "CreateTextMemory 入参 | userId=%s roleId=%s descLen=%d", userId, req.RoleId, len(req.Desc))

	item, err := h.service.CreateTextMemory(ctx, userId, req.RoleId, req.Desc)
	if err != nil {
		utils.LogWith(c, "File", "CreateTextMemory 失败 | err=%v latency=%dms", err, time.Since(start).Milliseconds())
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}
	utils.LogWith(c, "File", "CreateTextMemory 成功 | id=%d roleId=%s latency=%dms", item.ID, item.RoleID, time.Since(start).Milliseconds())
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": item})
}
