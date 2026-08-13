package main

import (
	"log"
	"net/http"
	"os"
	"strings"

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
		"trustedProxies", getTrustedProxiesCSV(),
	)

	// 5. 设置路由
	r := gin.New()
	// gin.New() 不带默认 logger（自定义 AccessLog 覆盖），保留 Recovery 防 panic
	r.Use(gin.Recovery())
	// 反向代理信任：未配置时退化为不信任任何代理（Gin 默认信任所有 → IP 可被 XFF 伪造 → 审计不可信）
	if err := setupTrustedProxies(r); err != nil {
		log.Fatalf("setup trusted proxies failed: %v", err)
	}
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

// getTrustedProxiesCSV 读 TRUSTED_PROXIES（逗号分隔 CIDR/IP），返回用于日志的可读字符串。
func getTrustedProxiesCSV() string {
	v := os.Getenv("TRUSTED_PROXIES")
	if v == "" {
		return "(none, 客户端 IP 直接取自 RemoteAddr)"
	}
	return v
}

// setupTrustedProxies 配置 Gin 的可信代理列表。
//
// 背景：Gin 默认信任所有代理，会读取 X-Forwarded-For，导致 c.ClientIP() 被请求头伪造，
// 审计日志 IP 不可信。修复策略：
//   - TRUSTED_PROXIES 未配置：r.SetTrustedProxies(nil) → 退化为 RemoteAddr（直连最稳）
//   - TRUSTED_PROXIES 配了：仅信任这些 CIDR/IP，XFF 取最后一跳
//
// CIDR 解析失败时整个启动 fail-fast，避免"看似配置了实则没生效"的隐性风险。
func setupTrustedProxies(r *gin.Engine) error {
	v := os.Getenv("TRUSTED_PROXIES")
	if v == "" {
		// 不信任任何代理
		if err := r.SetTrustedProxies(nil); err != nil {
			return err
		}
		log.Println("[startup] TRUSTED_PROXIES 未配置，c.ClientIP() 取自 RemoteAddr（XFF 不生效）")
		return nil
	}
	parts := strings.Split(v, ",")
	proxies := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		proxies = append(proxies, p)
	}
	if len(proxies) == 0 {
		if err := r.SetTrustedProxies(nil); err != nil {
			return err
		}
		log.Println("[startup] TRUSTED_PROXIES 解析后为空，c.ClientIP() 取自 RemoteAddr")
		return nil
	}
	if err := r.SetTrustedProxies(proxies); err != nil {
		return err
	}
	log.Printf("[startup] TRUSTED_PROXIES 已配置: %v", proxies)
	return nil
}
