package handlers

import (
	"echo-core/middleware"
	"echo-core/repository"
	"echo-core/service"
	"echo-core/service/request"
	"echo-core/utils"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// RecallMemoryHandler 回忆记忆（文档即记忆）HTTP 适配层。
type RecallMemoryHandler struct {
	service *service.RecallMemoryService
}

// NewRecallMemoryHandler 构造
func NewRecallMemoryHandler() (*RecallMemoryHandler, error) {
	svc, err := service.NewRecallMemoryService()
	if err != nil {
		return nil, err
	}
	return &RecallMemoryHandler{service: svc}, nil
}

// requireUser 统一取 session userId
func (h *RecallMemoryHandler) requireUser(c *gin.Context) (string, bool) {
	userId, ok := middleware.MustUserID(c)
	if !ok || userId == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "未登录"})
		return "", false
	}
	return userId, true
}

// Apply 申请记忆 ID (POST /api/memory/apply)
func (h *RecallMemoryHandler) Apply(c *gin.Context) {
	ctx := c.Request.Context()
	userId, ok := h.requireUser(c)
	if !ok {
		return
	}
	memoryId := h.service.ApplyMemoryId(ctx)
	utils.LogWith(c, "Recall", "Apply 成功 | userId=%s memoryId=%s", userId, memoryId)
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": gin.H{"memoryId": memoryId}})
}

// CheckTopic 主题唯一性校验 (POST /api/memory/check-topic)
func (h *RecallMemoryHandler) CheckTopic(c *gin.Context) {
	ctx := c.Request.Context()
	userId, ok := h.requireUser(c)
	if !ok {
		return
	}
	var req request.CheckTopicRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误: " + err.Error()})
		return
	}
	exists, err := h.service.CheckTopicExists(ctx, userId, req.RoleId, req.Topic)
	if err != nil {
		utils.LogWith(c, "Recall", "CheckTopic 失败 | err=%v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": gin.H{"exists": exists}})
}

// UploadToken 记忆目录内上传 token (POST /api/memory/upload-token)
func (h *RecallMemoryHandler) UploadToken(c *gin.Context) {
	ctx := c.Request.Context()
	userId, ok := h.requireUser(c)
	if !ok {
		return
	}
	var req request.MemoryUploadTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误: " + err.Error()})
		return
	}
	result, err := h.service.GetMemoryUploadToken(ctx, userId, req.RoleId, req.MemoryId, req.FileName, req.IsMd)
	if err != nil {
		utils.LogWith(c, "Recall", "UploadToken 失败 | err=%v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": result})
}

// Save 保存记忆 (POST /api/memory/save)
func (h *RecallMemoryHandler) Save(c *gin.Context) {
	ctx := c.Request.Context()
	start := time.Now()
	userId, ok := h.requireUser(c)
	if !ok {
		return
	}
	var req request.SaveMemoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误: " + err.Error()})
		return
	}
	item, err := h.service.SaveMemory(ctx, userId, &req)
	if err != nil {
		if errors.Is(err, service.ErrTopicExists) {
			c.JSON(http.StatusConflict, gin.H{"code": 409, "message": "记忆主题已存在"})
			return
		}
		utils.LogWith(c, "Recall", "Save 失败 | err=%v latency=%dms", err, time.Since(start).Milliseconds())
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}
	utils.LogWith(c, "Recall", "Save 成功 | memoryId=%s latency=%dms", item.MemoryID, time.Since(start).Milliseconds())
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": item})
}

// Update 编辑已有记忆 (POST /api/memory/update)。
//
// 与 Save 的差异：
//   - 不校验主题唯一性（编辑是更新自身，不存在与自身冲突的问题）。
//   - 源文件按 delta 增量同步。
//   - 前端按"源文件增删 OR 主观描述改动"算出 needReparse；服务端据此决定是否异步触发 AI 重新解析。
//   - 入参 memoryId 必须已存在，否则 404。
func (h *RecallMemoryHandler) Update(c *gin.Context) {
	ctx := c.Request.Context()
	start := time.Now()
	userId, ok := h.requireUser(c)
	if !ok {
		return
	}
	var req request.UpdateMemoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误: " + err.Error()})
		return
	}
	item, err := h.service.UpdateMemory(ctx, userId, &req)
	if err != nil {
		if errors.Is(err, service.ErrMemoryNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "记忆不存在"})
			return
		}
		utils.LogWith(c, "Recall", "Update 失败 | err=%v latency=%dms", err, time.Since(start).Milliseconds())
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}
	utils.LogWith(c, "Recall", "Update 成功 | memoryId=%s needReparse=%v latency=%dms",
		item.MemoryID, req.NeedReparse, time.Since(start).Milliseconds())
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": item})
}

// List 记忆列表 (GET /api/memory/list?roleId=)
func (h *RecallMemoryHandler) List(c *gin.Context) {
	ctx := c.Request.Context()
	userId, ok := h.requireUser(c)
	if !ok {
		return
	}
	roleId := c.Query("roleId")
	items, err := h.service.ListMemories(ctx, userId, roleId)
	if err != nil {
		utils.LogWith(c, "Recall", "List 失败 | err=%v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": items})
}

// Detail 记忆详情 (GET /api/memory/detail?memoryId=)
func (h *RecallMemoryHandler) Detail(c *gin.Context) {
	ctx := c.Request.Context()
	userId, ok := h.requireUser(c)
	if !ok {
		return
	}
	memoryId := c.Query("memoryId")
	if memoryId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "memoryId 必填"})
		return
	}
	item, err := h.service.GetMemoryDetail(ctx, userId, memoryId)
	if err != nil {
		if errors.Is(err, service.ErrMemoryNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "记忆不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": item})
}

// DeleteFile 删除单个源文件 (DELETE /api/memory/file)
func (h *RecallMemoryHandler) DeleteFile(c *gin.Context) {
	ctx := c.Request.Context()
	userId, ok := h.requireUser(c)
	if !ok {
		return
	}
	var req request.DeleteSourceFileReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误: " + err.Error()})
		return
	}
	if err := h.service.DeleteSourceFile(ctx, userId, &req); err != nil {
		if errors.Is(err, service.ErrMemoryNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "记忆不存在"})
			return
		}
		utils.LogWith(c, "Recall", "DeleteFile 失败 | err=%v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}
	utils.LogWith(c, "Recall", "DeleteFile 成功 | memoryId=%s fileKey=%s", req.MemoryId, req.FileKey)
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok"})
}

// DeleteTheme 删除整个记忆主题 (DELETE /api/memory/theme)
func (h *RecallMemoryHandler) DeleteTheme(c *gin.Context) {
	ctx := c.Request.Context()
	userId, ok := h.requireUser(c)
	if !ok {
		return
	}
	var req request.DeleteMemoryThemeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误: " + err.Error()})
		return
	}
	if err := h.service.DeleteMemoryTheme(ctx, userId, &req); err != nil {
		if errors.Is(err, service.ErrMemoryNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "记忆不存在"})
			return
		}
		utils.LogWith(c, "Recall", "DeleteTheme 失败 | err=%v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}
	utils.LogWith(c, "Recall", "DeleteTheme 成功 | memoryId=%s", req.MemoryId)
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok"})
}

// UpdateParseStatus 内部接口：echo-ai 解析完成后回调，更新 parse_status（POST /api/memory/internal/parse-status）
//
// 用途：把 echo-ai 解析完成的最终状态（2完成/3失败）写回 echo-core DB，
// 替代/补充 echo-ai 直写 MySQL 的方案。当 echo-ai 与 echo-core 共享同一个 DB 时直写更快，
// 但若未来拆库或 echo-ai 失去 DB 写权限，回调接口是兜底通道。
//   - 鉴权：可选 ECHO_CORE_INTERNAL_TOKEN（与 md-content 系列接口一致）
func (h *RecallMemoryHandler) UpdateParseStatus(c *gin.Context) {
	ctx := c.Request.Context()
	if utils.GetEnv("ECHO_CORE_INTERNAL_TOKEN", "") != "" {
		if c.GetHeader("X-Internal-Token") != utils.GetEnv("ECHO_CORE_INTERNAL_TOKEN", "") {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "internal token mismatch"})
			return
		}
	}
	var req struct {
		MemoryID string `json:"memoryId" binding:"required,len=32"`
		Status   int    `json:"status" binding:"required,oneof=0 1 2 3"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误: " + err.Error()})
		return
	}
	if err := h.service.UpdateParseStatus(ctx, req.MemoryID, req.Status); err != nil {
		if errors.Is(err, service.ErrMemoryNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "记忆不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}
	utils.LogWith(c, "Recall", "UpdateParseStatus ok | memoryId=%s status=%d", req.MemoryID, req.Status)
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok"})
}

// MdUrl 内部接口：保留兼容（已废弃，请用 MdContent）
func (h *RecallMemoryHandler) MdUrl(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "deprecated, use /api/memory/md-content"})
}

// UpdateMdContent 内部接口：echo-ai 把解析后的 md 写回数据库（POST /api/memory/md-content/save）
//   - 让 echo-ai 在 md 上传对象存储后立刻把内容也缓存到 DB
//   - 后续 echo-ai 内部使用 md 时不必再从对象存储下载（绕开 CDN 421）
func (h *RecallMemoryHandler) UpdateMdContent(c *gin.Context) {
	ctx := c.Request.Context()
	if utils.GetEnv("ECHO_CORE_INTERNAL_TOKEN", "") != "" {
		if c.GetHeader("X-Internal-Token") != utils.GetEnv("ECHO_CORE_INTERNAL_TOKEN", "") {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "internal token mismatch"})
			return
		}
	}
	var req struct {
		MemoryId  string `json:"memoryId" binding:"required,len=32"`
		MdContent string `json:"mdContent" binding:"required,min=1"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误: " + err.Error()})
		return
	}
	if err := h.service.UpdateMdContent(ctx, req.MemoryId, req.MdContent); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}
	utils.LogWith(c, "Recall", "MdContent saved | memoryId=%s bytes=%d", req.MemoryId, len(req.MdContent))
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok"})
}

// MdContent 内部接口：直接返回 md 内容（POST /api/memory/md-content）
//
//   - 从 echo-core 数据库直接读 md_content 字段（AI 解析后已落库，永远可达）
//   - 设计上避免从对象存储下载：Qiniu CDN 域名偶尔返回 421 Misdirected Request
//     （用户桶绑定单一 CDN 域且该域路由异常），把 md 内容缓存到 DB 是最稳的方案。
func (h *RecallMemoryHandler) MdContent(c *gin.Context) {
	ctx := c.Request.Context()
	if utils.GetEnv("ECHO_CORE_INTERNAL_TOKEN", "") != "" {
		if c.GetHeader("X-Internal-Token") != utils.GetEnv("ECHO_CORE_INTERNAL_TOKEN", "") {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "internal token mismatch"})
			return
		}
	}
	var req struct {
		UserId   string `json:"userId" binding:"required"`
		MemoryId string `json:"memoryId" binding:"required,len=32"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误: " + err.Error()})
		return
	}
	// 直接走 repo 取 md_content（不经过 service，避免大 md 被 service 转换）
	repo := repository.NewRecallMemoryRepository()
	m, err := repo.GetByMemoryId(ctx, req.MemoryId)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "记忆不存在"})
		return
	}
	if m.UserId != req.UserId {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "记忆不存在"})
		return
	}
	if m.MdContent == "" {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "md 内容未缓存（早期数据）"})
		return
	}
	utils.LogWith(c, "Recall", "MdContent from db | bytes=%d", len(m.MdContent))
	c.Data(http.StatusOK, "text/markdown; charset=utf-8", []byte(m.MdContent))
}
