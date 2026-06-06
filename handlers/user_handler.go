package handlers

import (
	"echo-core/dto"
	"echo-core/service"
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

// UserHandler 用户 HTTP 处理器
type UserHandler struct {
	service *service.UserService
}

// NewUserHandler 构造 UserHandler
func NewUserHandler() *UserHandler {
	return &UserHandler{service: service.NewUserService()}
}

// Login 用户登录
// POST /api/auth/login
func (h *UserHandler) Login(c *gin.Context) {
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[UserHandler.Login] 参数错误: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误: " + err.Error()})
		return
	}
	clientIP := c.ClientIP()
	log.Printf("[UserHandler.Login] 收到登录请求: username=%s, ip=%s", req.Username, clientIP)

	result, err := h.service.Login(req, clientIP)
	if err != nil {
		log.Printf("[UserHandler.Login] 登录失败: username=%s, err=%v", req.Username, err)
		switch {
		case errors.Is(err, service.ErrUserDisabled):
			c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": err.Error()})
		case errors.Is(err, service.ErrInvalidCredentials):
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "账号或密码错误"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "登录失败，请稍后再试"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "登录成功",
		"data":    result,
	})
}

// Register 用户注册
// POST /api/user/register
func (h *UserHandler) Register(c *gin.Context) {
	var req dto.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[UserHandler.Register] 参数错误: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误: " + err.Error()})
		return
	}
	log.Printf("[UserHandler.Register] 收到注册请求: username=%s", req.Username)

	user, err := h.service.Register(req)
	if err != nil {
		log.Printf("[UserHandler.Register] 注册失败: username=%s, err=%v", req.Username, err)
		if errors.Is(err, service.ErrUserExists) {
			c.JSON(http.StatusConflict, gin.H{"code": 409, "message": "账号已存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "注册成功",
		"data":    user,
	})
}

// CheckAccount 账号占用校验
// POST /api/user/check-account
func (h *UserHandler) CheckAccount(c *gin.Context) {
	var req dto.CheckAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[UserHandler.CheckAccount] 参数错误: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误: " + err.Error()})
		return
	}
	log.Printf("[UserHandler.CheckAccount] 收到账号校验请求: username=%s", req.Username)

	result, err := h.service.CheckAccount(req)
	if err != nil {
		if errors.Is(err, service.ErrUsernameUnavailable) {
			c.JSON(http.StatusOK, gin.H{
				"code":    200,
				"message": "账号已被占用",
				"data":    result,
			})
			return
		}
		log.Printf("[UserHandler.CheckAccount] 账号校验失败: username=%s, err=%v", req.Username, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "账号校验失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "账号可用",
		"data":    result,
	})
}

// Check 校验会话是否有效
// POST /api/auth/check
func (h *UserHandler) Check(c *gin.Context) {
	var req dto.CheckSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[UserHandler.Check] 参数错误: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误: " + err.Error()})
		return
	}
	log.Printf("[UserHandler.Check] 收到会话校验请求: sessionID=%s", req.SessionID)

	result, err := h.service.CheckSession(req.SessionID)
	if err != nil {
		log.Printf("[UserHandler.Check] 会话校验失败: sessionID=%s, err=%v", req.SessionID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}
	if !result.Valid {
		c.JSON(http.StatusOK, gin.H{
			"code":    200,
			"message": "会话无效或已过期",
			"data":    result,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "会话有效",
		"data":    result,
	})
}

// Logout 注销会话
// POST /api/auth/logout
func (h *UserHandler) Logout(c *gin.Context) {
	var req dto.LogoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[UserHandler.Logout] 参数错误: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误: " + err.Error()})
		return
	}
	log.Printf("[UserHandler.Logout] 收到注销请求: sessionID=%s", req.SessionID)

	if err := h.service.Logout(req.SessionID); err != nil {
		log.Printf("[UserHandler.Logout] 注销失败: sessionID=%s, err=%v", req.SessionID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "注销失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "退出成功",
	})
}
