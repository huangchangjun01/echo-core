package routes

import (
	"go-start/handlers"

	"github.com/gin-gonic/gin"
)

// SetupRoutes 设置所有路由
func SetupRoutes(r *gin.Engine) {
	api := r.Group("/api")
	departmentRegisterRoutes(api)
	fileRegisterRoutes(api)
}

// 注册部门相关的路由
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
func fileRegisterRoutes(api *gin.RouterGroup) {
	fileHandler := handlers.NewFileHandler()
	{
		file := api.Group("/file")
		{
			file.POST("/upload", fileHandler.UploadHandler)            // 文件上传
			file.GET("/download", fileHandler.DownloadRedirectHandler) // 文件下载
		}
	}
}
