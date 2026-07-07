package main

import (
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	"echo-core/config"
	"echo-core/routes"
	"echo-core/utils"
)

func main() {
	// 1. 加载 .env
	initConfig()

	// 2. 数据库连接
	config.InitDB()

	// 3. 显式初始化 SessionStore 单例 + 优雅关停
	utils.InitSessionStore(0)
	defer utils.StopSessionStore()

	// 4. 启动横幅：环境/端口/Python baseURL，方便核对配置
	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "8080"
	}
	pythonBase := os.Getenv("ECHO_AI_REMOTE_BASE_URL")
	if pythonBase == "" {
		pythonBase = "http://localhost:8000"
	}
	dbHost := os.Getenv("DB_HOST")
	if dbHost == "" {
		dbHost = "localhost"
	}
	dbPort := os.Getenv("DB_PORT")
	if dbPort == "" {
		dbPort = "3306"
	}
	dbUser := os.Getenv("DB_USER")
	if dbUser == "" {
		dbUser = "root"
	}
	dbName := os.Getenv("DB_NAME")
	if dbName == "" {
		dbName = "testdb"
	}
	utils.LogStartup("config",
		"env", gin.Mode(),
		"port", port,
		"db", dbUser+"@tcp("+dbHost+":"+dbPort+")/"+dbName,
		"pythonBase", pythonBase,
		"qiniu", "see QINIU_* envs",
	)

	// 5. 设置路由
	r := gin.New()
	// gin.New() 不带默认 logger（自定义 AccessLog 覆盖），保留 Recovery 防 panic
	r.Use(gin.Recovery())
	if err := routes.SetupRoutes(r); err != nil {
		log.Fatalf("setup routes failed: %v", err)
	}

	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "Hello from Gin!",
		})
	})

	// 6. 启动
	utils.LogStartup("server", "listen", ":"+port, "version", "echo-core")
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("server run failed: %v", err)
	}
}

func initConfig() {
	// 加载 .env 文件（必须在最开始加载）
	if err := godotenv.Load(); err != nil {
		log.Println("警告: 未找到 .env 文件，使用系统环境变量")
	}
}
