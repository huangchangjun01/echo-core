// migrate_role_active_preset.go
// role_active_preset 表创建脚本(active preset 跟踪 — OCEAN 当前使用性格)。
//
// 设计要点：
//   - 历史实现:role_traits 主键 (user_id, role_id, preset_type),但没有
//     「当前在用哪个」的明确状态。UpdateMood / GetCoreSnapshot 硬编码优先
//     取 companion_default,不能反映用户实际选择。
//   - 修复:新增 role_active_preset 表,(user_id, role_id) 主键,记录每个
//     角色当前使用的 preset;SaveTraits 自动把被保存的 preset 设为 active。
//   - 步骤(幂等)：
//     1) 检查表是否存在 → 已存在则跳过
//     2) CREATE TABLE role_active_preset(...)
//   - 现有数据不需要回填 —— 没设过 active 的角色默认走 companion_default
//     (向后兼容,详见 service.resolveActivePresetType)。
//
// 触发方式：在 config.InitDB() 末尾调用 EnsureRoleActivePresetMigrated(DB)。
package scripts

import (
	"echo-core/config"
	"echo-core/utils"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// EnsureRoleActivePresetMigrated 执行 role_active_preset 表创建(幂等)。
//
// 在 echo-core 启动时由 main.go 调用一次。
// 失败仅记 WARN 日志(不阻塞启动,确保业务可用;下次启动重试)。
func EnsureRoleActivePresetMigrated(db *gorm.DB) {
	if db == nil {
		return
	}

	// 1. 幂等判断:表已存在则跳过
	if db.Migrator().HasTable("role_active_preset") {
		utils.LogStartup("migrate.role_active_preset", "status", "skip", "reason", "table exists")
		return
	}

	utils.LogStartup("migrate.role_active_preset", "status", "begin")
	start := time.Now()

	// 2. 建表。user_id/role_id 类型与 role_traits 主键保持一致(BIGINT UNSIGNED)。
	//    utf8mb4 字符集以支持中文 label 备注(comment),虽然本表无中文字段但保持一致。
	createSQL := `
CREATE TABLE role_active_preset (
  user_id     BIGINT UNSIGNED NOT NULL COMMENT '所属用户ID',
  role_id     BIGINT UNSIGNED NOT NULL COMMENT '角色ID',
  preset_type VARCHAR(32) NOT NULL COMMENT '当前使用的OCEAN预设key',
  updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (user_id, role_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='每角色当前使用的OCEAN预设'
`
	if err := db.Exec(createSQL).Error; err != nil {
		utils.LogWithCtx(nil, "migrate.role_active_preset", "建表失败 | err=%v", err)
		return
	}

	utils.LogStartup("migrate.role_active_preset", "status", "done",
		"latency", fmt.Sprintf("%dms", time.Since(start).Milliseconds()))
}

// RunRoleActivePresetMigration 直接调用入口(用于 main.go 启动时调用)。
func RunRoleActivePresetMigration() {
	EnsureRoleActivePresetMigrated(config.GetDB())
}
