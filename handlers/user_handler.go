package handlers

import (
	"echo-core/dto"
	"echo-core/service"
	"echo-core/utils"
	"errors"
	"net/http"
	"time"

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
// 注意：**不打请求体**（含 password 字段），只打印 username / passwordLen / ip。
func (h *UserHandler) Login(c *gin.Context) {
	ctx := c.Request.Context()
	start := time.Now()
	utils.LogWith(c, "User", "Login 入口 | method=POST path=%s ip=%s contentType=%s",
		c.Request.URL.Path, c.ClientIP(), c.GetHeader("Content-Type"))

	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.LogWith(c, "User", "Login 参数错误 | err=%v", err)
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误: " + err.Error()})
		return
	}
	// 关键：只打 username / passwordLen / ip；不打印 password 明文
	utils.LogWith(c, "User", "Login 入参 | username=%s passwordLen=%d ip=%s", req.Username, len(req.Password), c.ClientIP())

	result, err := h.service.Login(ctx, req, c.ClientIP())
	if err != nil {
		utils.LogWith(c, "User", "Login 失败 | username=%s latency=%dms err=%v", req.Username, time.Since(start).Milliseconds(), err)
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

	utils.LogWith(c, "User", "Login 成功 | username=%s sid=%s... userId=%d latency=%dms",
		req.Username, truncateSID(result.SessionID), result.User.ID, time.Since(start).Milliseconds())
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "登录成功",
		"data":    result,
	})
}

// Register 用户注册
// POST /api/user/register
// 注意：**不打请求体**（含 password 字段），只打印 username / 字段长度。
func (h *UserHandler) Register(c *gin.Context) {
	ctx := c.Request.Context()
	start := time.Now()
	utils.LogWith(c, "User", "Register 入口 | method=POST path=%s ip=%s", c.Request.URL.Path, c.ClientIP())

	var req dto.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.LogWith(c, "User", "Register 参数错误 | err=%v", err)
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误: " + err.Error()})
		return
	}
	// 关键：不打 password 明文，只打字段名+长度
	utils.LogWith(c, "User", "Register 入参 | username=%s passwordLen=%d nicknameLen=%d emailLen=%d",
		req.Username, len(req.Password), len(req.Nickname), len(req.Email))

	user, err := h.service.Register(ctx, req)
	if err != nil {
		utils.LogWith(c, "User", "Register 失败 | username=%s latency=%dms err=%v", req.Username, time.Since(start).Milliseconds(), err)
		if errors.Is(err, service.ErrUserExists) {
			c.JSON(http.StatusConflict, gin.H{"code": 409, "message": "账号已存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	utils.LogWith(c, "User", "Register 成功 | username=%s userId=%d latency=%dms", req.Username, user.ID, time.Since(start).Milliseconds())
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "注册成功",
		"data":    user,
	})
}

// CheckAccount 账号占用校验
// POST /api/user/check-account
func (h *UserHandler) CheckAccount(c *gin.Context) {
	ctx := c.Request.Context()
	utils.LogWith(c, "User", "CheckAccount 入口 | method=POST path=%s", c.Request.URL.Path)

	var req dto.CheckAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.LogWith(c, "User", "CheckAccount 参数错误 | err=%v", err)
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误: " + err.Error()})
		return
	}
	utils.LogWith(c, "User", "CheckAccount 入参 | username=%s", req.Username)

	result, err := h.service.CheckAccount(ctx, req)
	if err != nil {
		if errors.Is(err, service.ErrUsernameUnavailable) {
			c.JSON(http.StatusOK, gin.H{
				"code":    200,
				"message": "账号已被占用",
				"data":    result,
			})
			return
		}
		utils.LogWith(c, "User", "CheckAccount 失败 | username=%s err=%v", req.Username, err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "账号校验失败"})
		return
	}

	utils.LogWith(c, "User", "CheckAccount 成功 | username=%s available=%v", req.Username, result.Available)
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "账号可用",
		"data":    result,
	})
}

// Check 校验会话是否有效
// POST /api/auth/check
func (h *UserHandler) Check(c *gin.Context) {
	ctx := c.Request.Context()
	utils.LogWith(c, "User", "Check 入口 | method=POST path=%s", c.Request.URL.Path)

	var req dto.CheckSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.LogWith(c, "User", "Check 参数错误 | err=%v", err)
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误: " + err.Error()})
		return
	}
	utils.LogWith(c, "User", "Check 入参 | sid=%s...", truncateSID(req.SessionID))

	result, err := h.service.CheckSession(ctx, req.SessionID)
	if err != nil {
		utils.LogWith(c, "User", "Check 失败 | sid=%s... err=%v", truncateSID(req.SessionID), err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}
	utils.LogWith(c, "User", "Check 完成 | sid=%s... valid=%v userId=%d",
		truncateSID(req.SessionID), result.Valid, result.UserID)
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
	ctx := c.Request.Context()
	utils.LogWith(c, "User", "Logout 入口 | method=POST path=%s", c.Request.URL.Path)

	var req dto.LogoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.LogWith(c, "User", "Logout 参数错误 | err=%v", err)
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数错误: " + err.Error()})
		return
	}
	utils.LogWith(c, "User", "Logout 入参 | sid=%s...", truncateSID(req.SessionID))

	if err := h.service.Logout(ctx, req.SessionID); err != nil {
		utils.LogWith(c, "User", "Logout 失败 | sid=%s... err=%v", truncateSID(req.SessionID), err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "注销失败"})
		return
	}
	utils.LogWith(c, "User", "Logout 成功 | sid=%s...", truncateSID(req.SessionID))
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "退出成功",
	})
}

// truncateSID 日志中只打 sid 前 8 字符 + 长度，避免完整 token 刷屏
func truncateSID(s string) string {
	if len(s) <= 8 {
		return s
	}
	return s[:8] + "..."
}
