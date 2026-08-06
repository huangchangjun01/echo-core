package handlers

import (
	"echo-core/middleware"
	"echo-core/service"
	"echo-core/service/request"
	"echo-core/utils"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

// DownloadFileHandler 下载文件二进制流（GET）
// GET /api/file/:id/download
//
// 设计要点：
//  1. 走七牛 SDK 源站 API，不依赖用户配置的 CDN 域名（CDN 可能 DNS 解析失败 /
//     已下线，导致前端 fetch 直接 NetworkError）。
//  2. 设置 Content-Disposition: attachment 让浏览器原生触发下载，而不是渲染。
//  3. 鉴权：session 中取 userId，service 层校验文件归属，防止越权下载。
//  4. 流式转发（io.Copy），不在服务端把整个文件加载到内存。
func (h *FileHandler) DownloadFileHandler(c *gin.Context) {
	ctx := c.Request.Context()
	start := time.Now()
	utils.LogWith(c, "File", "DownloadFile 入口 | method=GET path=%s", c.Request.URL.Path)

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

	result, err := h.service.DownloadFile(ctx, userId, id)
	if err != nil {
		// service 层把"不存在"和"无权访问"合并为同一错误，对外只透出 404
		msg := err.Error()
		status := http.StatusInternalServerError
		code := 500
		if msg == "文件不存在" {
			status = http.StatusNotFound
			code = 404
		} else if msg == "该文件无媒体内容，不可下载" {
			status = http.StatusBadRequest
			code = 400
		}
		utils.LogWith(c, "File", "DownloadFile 失败 | id=%d err=%v latency=%dms",
			id, err, time.Since(start).Milliseconds())
		c.JSON(status, gin.H{"code": code, "message": msg})
		return
	}
	defer result.Body.Close()

	// 文件名需要 URL 编码，避免中文 / 特殊字符被浏览器截断
	disposition := fmt.Sprintf("attachment; filename*=UTF-8''%s", url.QueryEscape(result.FileName))
	c.Header("Content-Type", result.ContentType)
	c.Header("Content-Disposition", disposition)
	c.Header("X-Content-Type-Options", "nosniff")
	// 把状态写入响应头后才能继续写 body
	c.Status(http.StatusOK)

	written, copyErr := io.Copy(c.Writer, result.Body)
	utils.LogWith(c, "File", "DownloadFile 完成 | id=%d bytes=%d err=%v latency=%dms",
		id, written, copyErr, time.Since(start).Milliseconds())
	if copyErr != nil {
		// 已经写过 header，不能再 c.JSON；只记日志，靠连接关闭告知客户端
		utils.LogWithCtx(ctx, "FileHandler.DownloadFileHandler", "流式转发失败 | id=%d err=%v", id, copyErr)
	}
}
