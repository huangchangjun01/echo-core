package config

import (
	"echo-core/models"
	"echo-core/utils"
	"fmt"
	"gorm.io/gorm/schema"
	"log"
	"os"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// DatabaseConfig 数据库配置结构
type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	Charset  string
}

// InitDB 初始化数据库连接
// 优先尝试 MySQL，失败则回退到 SQLite
func InitDB() {
	// 配置 GORM
	gormConfig := &gorm.Config{
		Logger: logger.New(
			log.New(os.Stdout, "\r\n", log.LstdFlags),
			logger.Config{
				SlowThreshold: time.Second, // 慢 SQL 阈值
				LogLevel:      logger.Info, // 日志级别
				Colorful:      true,        // 彩色打印
			},
		),
		NamingStrategy: schema.NamingStrategy{
			SingularTable: true, // 禁用复数表名
		},
	}

	// 尝试 MySQL 连接
	config := DatabaseConfig{
		Host:     utils.GetEnv("DB_HOST", "localhost"),
		Port:     utils.GetEnv("DB_PORT", "3306"),
		User:     utils.GetEnv("DB_USER", "root"),
		Password: utils.GetEnv("DB_PASSWORD", ""),
		DBName:   utils.GetEnv("DB_NAME", "testdb"),
		Charset:  "utf8mb4",
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=%s&parseTime=True&loc=Local&timeout=3s&readTimeout=3s&writeTimeout=3s",
		config.User, config.Password, config.Host, config.Port, config.DBName, config.Charset,
	)

	var err error
	DB, err = gorm.Open(mysql.Open(dsn), gormConfig)
	if err != nil {
		log.Printf("[InitDB] MySQL连接失败: %v, 回退到SQLite", err)
		DB, err = gorm.Open(sqlite.Open("echo_core.db"), gormConfig)
		if err != nil {
			log.Fatalf("数据库连接失败: %v", err)
		}
		log.Println("数据库连接成功 (SQLite)")
	} else {
		// 验证 MySQL 连接
		sqlDB, dbErr := DB.DB()
		if dbErr != nil {
			log.Printf("[InitDB] 获取底层DB失败: %v, 回退到SQLite", dbErr)
			DB, err = gorm.Open(sqlite.Open("echo_core.db"), gormConfig)
			if err != nil {
				log.Fatalf("数据库连接失败: %v", err)
			}
			log.Println("数据库连接成功 (SQLite)")
		} else if pingErr := sqlDB.Ping(); pingErr != nil {
			log.Printf("[InitDB] MySQL ping失败: %v, 回退到SQLite", pingErr)
			DB, err = gorm.Open(sqlite.Open("echo_core.db"), gormConfig)
			if err != nil {
				log.Fatalf("数据库连接失败: %v", err)
			}
			log.Println("数据库连接成功 (SQLite)")
		} else {
			// 连接池配置
			sqlDB.SetMaxIdleConns(10)
			sqlDB.SetMaxOpenConns(100)
			sqlDB.SetConnMaxLifetime(time.Hour)
			log.Println("数据库连接成功 (MySQL)")
		}
	}

	// 自动迁移表结构
	if err := autoMigrate(DB); err != nil {
		log.Printf("自动迁移失败: %v", err)
	}
}

// GetDB 获取数据库实例（供其他包使用）
func GetDB() *gorm.DB {
	return DB
}

// SetDBForTest 测试钩子：替换为内存数据库（仅测试代码使用）
func SetDBForTest(db *gorm.DB) {
	DB = db
}

// autoMigrate 自动迁移表结构
func autoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&models.File{},
		&models.SessionMessage{},
		&models.User{},
	)
}
