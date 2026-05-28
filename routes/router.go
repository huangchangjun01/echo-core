package routes

import (
	"echo-core/handlers"
	"fmt"

	"github.com/gin-gonic/gin"
)

// SetupRoutes 设置所有路由
func SetupRoutes(r *gin.Engine) error {
	api := r.Group("/api")
	departmentRegisterRoutes(api)
	if err := fileRegisterRoutes(api); err != nil {
		return err
	}
	if err := chatRegisterRoutes(api); err != nil {
		return err
	}
	return nil
}

// demo 注册部门相关的路由
func departmentRegisterRoutes(api *gin.RouterGroup) {
	departmentHandler := handlers.NewDepartmentHandler()
	{
		Departments := api.Group("/department")
		{
			Departments.GET("", departmentHandler.GetDepartmentList)       // 列表查询
			Departments.POST("", departmentHandler.CreateDepartment)       // 创建
			Departments.GET("/:id", departmentHandler.GetDepartment)       // 详情
			Departments.PUT("/:id", departmentHandler.UpdateDepartment)    // 更新
			Departments.DELETE("/:id", departmentHandler.DeleteDepartment) // 删除
		}
	}
}

// 文件相关的路由
func fileRegisterRoutes(api *gin.RouterGroup) error {
	fileHandler, err := handlers.NewFileHandler()
	if err != nil {
		return fmt.Errorf("create file handler: %w", err)
	}
	{
		file := api.Group("/file")
		{
			file.POST("/token", fileHandler.GetUploadTokenHandler)  // 获取上传token
			file.POST("/register", fileHandler.RegisterFileHandler) // 注册文件信息
		}
	}
	return nil
}

// 注册聊天相关的路由
func chatRegisterRoutes(api *gin.RouterGroup) error {

	return nil
}
