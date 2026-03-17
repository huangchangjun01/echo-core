package utils

import "os"

// 辅助函数：获取环境变量
func GetEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
