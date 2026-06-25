package handlers

import (
	"echo-core/config"
	"echo-core/models"
	"echo-core/utils"
	"log"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupTestDB 初始化内存数据库用于测试
// 返回清理函数
func setupTestDB() func() {
	// 静默 GORM 日志
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.New(log.New(log.Writer(), "", 0), logger.Config{LogLevel: logger.Silent}),
	})
	if err != nil {
		panic("内存数据库初始化失败: " + err.Error())
	}
	// 迁移表
	if err := db.AutoMigrate(&models.User{}, &models.SessionMessage{}, &models.File{}); err != nil {
		panic("迁移失败: " + err.Error())
	}
	config.SetDBForTest(db)

	// 重置 session store
	store := utils.NewMemorySessionStore(24 * time.Hour)
	utils.SetSessionStoreForTest(store)

	return func() {
		config.SetDBForTest(nil)
	}
}

// createTestSession 快速创建测试用会话
func createTestSession(userID uint, username string) string {
	store := utils.GetSessionStore()
	sess, _ := store.Create(userID, username, time.Hour)
	return sess.SessionID
}