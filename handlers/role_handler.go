package handlers

import (
	"echo-core/dto"
	"echo-core/middleware"
	"echo-core/service"
	"echo-core/utils"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// RoleHandler 角色 HTTP 处理器
type RoleHandler struct {
	service *service.RoleService
}

// NewRoleHandler 构造 RoleHandler
func NewRoleHandler() *RoleHandler {
	return &RoleHandler{service: service.NewRoleService()}
}

// Create 新建角色
// POST /api/role
func (h *RoleHandler) Create(c *gin.Context) {
	ctx := c.Request.Context()
	start := time.Now()
	utils.LogWith(c, "Role", "Create 入口 | method=POST path=%s", c.Request.URL.Path)

	userId, ok := middleware.MustUserID(c)
	if !ok || userId == "" {
		utils.LogWith(c, "Role", "Create 鉴权失败")
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "未登录"})
		return
	}

	var req dto.CreateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.LogWith(c, "Role", "Create 参数错误 | err=%v", err)
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误: " + err.Error()})
		return
	}
	utils.LogWith(c, "Role", "Create 入参 | userId=%s name=%s descLen=%d", userId, req.Name, len(req.Desc))

	out, err := h.service.Create(ctx, userId, req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrRoleExists):
			c.JSON(http.StatusConflict, gin.H{"code": 409, "message": "角色名已被占用"})
		default:
			utils.LogWith(c, "Role", "Create 失败 | err=%v latency=%dms", err, time.Since(start).Milliseconds())
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "创建失败"})
		}
		return
	}
	utils.LogWith(c, "Role", "Create 成功 | userId=%s id=%d name=%s latency=%dms",
		userId, out.ID, out.Name, time.Since(start).Milliseconds())
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "创建成功", "data": out})
}

// List 列出角色（无角色自动创建默认角色）
// GET /api/role
func (h *RoleHandler) List(c *gin.Context) {
	ctx := c.Request.Context()
	start := time.Now()
	utils.LogWith(c, "Role", "List 入口 | method=GET path=%s", c.Request.URL.Path)

	userId, ok := middleware.MustUserID(c)
	if !ok || userId == "" {
		utils.LogWith(c, "Role", "List 鉴权失败")
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "未登录"})
		return
	}

	out, err := h.service.List(ctx, userId)
	if err != nil {
		utils.LogWith(c, "Role", "List 失败 | err=%v latency=%dms", err, time.Since(start).Milliseconds())
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "查询失败"})
		return
	}
	utils.LogWith(c, "Role", "List 成功 | userId=%s count=%d latency=%dms", userId, len(out), time.Since(start).Milliseconds())
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": out})
}

// Update 修改角色
// PUT /api/role/:id
func (h *RoleHandler) Update(c *gin.Context) {
	ctx := c.Request.Context()
	start := time.Now()
	utils.LogWith(c, "Role", "Update 入口 | method=PUT path=%s", c.Request.URL.Path)

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

	var req dto.UpdateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误: " + err.Error()})
		return
	}

	out, err := h.service.Update(ctx, userId, id, req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrRoleExists):
			c.JSON(http.StatusConflict, gin.H{"code": 409, "message": "角色名已被占用"})
		case errors.Is(err, service.ErrRoleNotFound):
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "角色不存在"})
		default:
			utils.LogWith(c, "Role", "Update 失败 | id=%d err=%v", id, err)
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "更新失败"})
		}
		return
	}
	utils.LogWith(c, "Role", "Update 成功 | id=%d latency=%dms", id, time.Since(start).Milliseconds())
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok", "data": out})
}

// Delete 软删除角色
// DELETE /api/role/:id
func (h *RoleHandler) Delete(c *gin.Context) {
	ctx := c.Request.Context()
	start := time.Now()
	utils.LogWith(c, "Role", "Delete 入口 | method=DELETE path=%s", c.Request.URL.Path)

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

	if err := h.service.Delete(ctx, userId, id); err != nil {
		switch {
		case errors.Is(err, service.ErrRoleNotFound):
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "角色不存在"})
		default:
			utils.LogWith(c, "Role", "Delete 失败 | id=%d err=%v", id, err)
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		}
		return
	}
	utils.LogWith(c, "Role", "Delete 成功 | id=%d latency=%dms", id, time.Since(start).Milliseconds())
	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "ok"})
}
