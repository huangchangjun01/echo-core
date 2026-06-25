package routes

import (
	"echo-core/handlers"
	"echo-core/middleware"
	"echo-core/remote"
	"echo-core/service"
	"fmt"
	"os"

	"github.com/gin-gonic/gin"
)

// SetupRoutes 设置所有路由
func SetupRoutes(r *gin.Engine) error {
	// 全局 request_id 中间件
	r.Use(middleware.RequestID())

	api := r.Group("/api")
	// /auth/* 走公开路由（注册/登录/校验），无需 session
	if err := userRegisterRoutes(api); err != nil {
		return err
	}
	// /file/* 与 /chat/* 走鉴权中间件
	if err := fileRegisterRoutes(api); err != nil {
		return err
	}
	if err := chatRegisterRoutes(api); err != nil {
		return err
	}
	return nil
}

// 文件相关的路由（鉴权）
func fileRegisterRoutes(api *gin.RouterGroup) error {
	fileHandler, err := handlers.NewFileHandler()
	if err != nil {
		return fmt.Errorf("create file handler: %w", err)
	}
	{
		file := api.Group("/file", middleware.RequireSession())
		{
			file.POST("/token", fileHandler.GetUploadTokenHandler)  // 获取上传token
			file.POST("/register", fileHandler.RegisterFileHandler) // 注册文件信息
		}
	}
	return nil
}

// 注册聊天相关的路由（鉴权）
func chatRegisterRoutes(api *gin.RouterGroup) error {
	// 创建 Python 服务客户端
	pythonBaseURL := os.Getenv("ECHO_AI_REMOTE_BASE_URL")
	if pythonBaseURL == "" {
		pythonBaseURL = "http://localhost:8000"
	}
	pythonClient := remote.NewPythonClient(pythonBaseURL)

	// 创建聊天服务
	chatSvc := service.NewChatService(pythonClient)
	// 创建聊天处理器
	chatHandler := handlers.NewChatHandler(chatSvc)
	// 创建流式聊天处理器（SSE）
	chatStreamHandler := handlers.NewChatStreamHandler(chatSvc)

	chat := api.Group("/chat", middleware.RequireSession())
	{
		// POST /api/chat         流式聊天（SSE，返回 text/event-stream）
		chat.POST("", chatStreamHandler.ChatHandleSSE)
		// GET  /api/chat/history  获取历史消息
		chat.GET("/history", chatHandler.GetHistoryHandle)
		// DELETE /api/chat/session 清理会话
		chat.DELETE("/session", chatHandler.ClearSessionHandle)
	}
	return nil
}

// 用户管理相关路由（公开）
func userRegisterRoutes(api *gin.RouterGroup) error {
	userHandler := handlers.NewUserHandler()
	auth := api.Group("/auth")
	{
		auth.POST("/login", userHandler.Login)               // 登录
		auth.POST("/register", userHandler.Register)         // 注册
		auth.POST("/checkAccount", userHandler.CheckAccount) // 账号占用校验
		auth.POST("/check", userHandler.Check)               // 校验会话是否有效
		auth.POST("/logout", userHandler.Logout)             // 注销会话
	}
	return nil
}