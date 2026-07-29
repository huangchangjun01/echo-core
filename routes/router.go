package routes

import (
	"echo-core/handlers"
	"echo-core/middleware"
	"echo-core/service"
	"fmt"

	"github.com/gin-gonic/gin"
)

// SetupRoutes 设置所有路由
func SetupRoutes(r *gin.Engine) error {
	// 全局 request_id 中间件——所有路由都受益，便于日志串联
	r.Use(middleware.RequestID())
	// 全局 CORS：浏览器跨域调 SSE 必须有 Access-Control-* 响应头，
	// 否则浏览器会拦截响应导致页面"无输出"（网络看着 200，JS 拿不到 body）。
	r.Use(middleware.CORS())
	// 全局访问日志：放在 CORS 之后，预检（OPTIONS）请求会被 CORS 短路不会到这里，
	// 所以 access log 只记录真实业务请求；必须在 RequestID 之后才能拿到 rid。
	r.Use(middleware.AccessLog())

	// 顶层存活探针（无 /api 前缀、无鉴权）
	healthHandler := handlers.NewHealthHandler()
	r.GET("/health", healthHandler.Health)

	api := r.Group("/api")
	// 公开路由：用户与会话（无需鉴权）
	if err := userRegisterRoutes(api); err != nil {
		return err
	}
	// 鉴权路由：文件存储
	if err := fileRegisterRoutes(api); err != nil {
		return err
	}
	// 鉴权路由：角色（CRUD + 自动默认角色）
	if err := roleRegisterRoutes(api); err != nil {
		return err
	}
	// 鉴权路由：聊天（同步 + SSE 流式）
	if err := chatRegisterRoutes(api); err != nil {
		return err
	}
	return nil
}

// 角色相关路由（鉴权）
func roleRegisterRoutes(api *gin.RouterGroup) error {
	roleHandler := handlers.NewRoleHandler()
	role := api.Group("/role", middleware.RequireSession())
	{
		role.POST("", roleHandler.Create)      // 创建角色
		role.GET("", roleHandler.List)         // 列出角色（无角色自动建默认）
		role.PUT("/:id", roleHandler.Update)   // 修改角色
		role.DELETE("/:id", roleHandler.Delete) // 删除角色（至少保留 1 个）
	}
	return nil
}

// 文件存储相关路由（鉴权）
func fileRegisterRoutes(api *gin.RouterGroup) error {
	fileHandler, err := handlers.NewFileHandler()
	if err != nil {
		return fmt.Errorf("create file handler: %w", err)
	}
	file := api.Group("/file", middleware.RequireSession())
	{
		file.POST("/token", fileHandler.GetUploadTokenHandler)    // 获取上传 token
		file.POST("/register", fileHandler.RegisterFileHandler)   // 登记文件元数据
		file.GET("/list", fileHandler.ListMemoryFilesHandler)     // 记忆管理：列出文件
		file.PUT("/:id/desc", fileHandler.UpdateFileDescHandler) // 记忆管理：编辑描述
		file.POST("/text", fileHandler.CreateTextMemoryHandler)   // 记忆管理：新增纯文本
	}
	return nil
}

// 聊天相关路由（鉴权）
func chatRegisterRoutes(api *gin.RouterGroup) error {
	// 构造聊天服务（Python 同步/流式透传）
	chatSvc := service.NewChatService()
	chatHandler := handlers.NewChatHandler(chatSvc)

	chat := api.Group("/chat", middleware.RequireSession())
	{
		// POST /api/chat
		//   stream=false（默认）：同步返回 JSON
		//   stream=true        ：SSE 流式逐帧推送 ChatEvent
		chat.POST("", chatHandler.Chat)
	}
	return nil
}

// 用户与会话相关路由（公开）
func userRegisterRoutes(api *gin.RouterGroup) error {
	userHandler := handlers.NewUserHandler()
	auth := api.Group("/auth")
	{
		auth.POST("/login", userHandler.Login)               // 登录
		auth.POST("/register", userHandler.Register)         // 注册
		auth.POST("/checkAccount", userHandler.CheckAccount) // 账号占用校验
		auth.POST("/check", userHandler.Check)               // 校验会话
		auth.POST("/logout", userHandler.Logout)             // 注销会话
	}
	return nil
}
