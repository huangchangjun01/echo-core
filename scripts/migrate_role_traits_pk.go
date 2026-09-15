// migrate_role_traits_pk.go
// role_traits 表主键修复脚本（OCEAN 五维切换预设互窜 bug 修复）。
//
// 设计要点：
//   - 历史上 role_traits 用 (user_id, role_id) 复合主键,每个 (user, role) 只有
//     一行 OCEAN。切换预设并保存时,会 UPDATE 同一行,导致之前预设的自定义值
//     被覆盖(产品层期待每个预设独立保存一套五维)。
//   - 修复：把主键扩展为 (user_id, role_id, preset_type),每对 (user, role)
//     下每个 preset 独立一行。
//   - 步骤(幂等)：
//     1) 备份 role_traits → role_traits_backup_202609(保留 30 天后人工 DROP)
//     2) DROP PRIMARY KEY
//     3) ADD PRIMARY KEY (user_id, role_id, preset_type)
//   - 现有数据天然无重复(原 PK 保证 (user, role) 唯一,preset_type 字段在
//     迁移前已存在),迁移不会丢数据。
//
// 触发方式：在 config.InitDB() 末尾调用 EnsureRoleTraitsPKMigrated(DB)。
package scripts

import (
	"echo-core/config"
	"echo-core/utils"
	"fmt"
	"time"

	"gorm.io/gorm"
)

const roleTraitsBackupTable = "role_traits_backup_202609"

// EnsureRoleTraitsPKMigrated 执行 role_traits 表主键迁移(幂等)。
//
// 在 echo-core 启动时由 config.InitDB() 末尾调用一次。
// 失败仅记 WARN 日志(不阻塞启动,确保业务可用;下次启动重试)。
func EnsureRoleTraitsPKMigrated(db *gorm.DB) {
	if db == nil {
		return
	}

	// 1. 检查 role_traits 表是否存在
	if !db.Migrator().HasTable("role_traits") {
		utils.LogStartup("migrate.role_traits", "status", "skip", "reason", "role_traits table not exists")
		return
	}

	// 2. 检查备份表是否已存在 → 幂等判断
	if db.Migrator().HasTable(roleTraitsBackupTable) {
		utils.LogStartup("migrate.role_traits", "status", "skip", "reason", "backup exists, migration already done")
		return
	}

	// 3. 检测主键是否已经包含 preset_type(可能在新部署上已 OK)
	hasPresetInPK := false
	if db.Migrator().HasIndex("role_traits", "PRIMARY") {
		// 通过 SHOW INDEX 查询主键列
		type indexInfo struct {
			ColumnName string
		}
		var indexes []indexInfo
		if err := db.Raw(`SHOW INDEX FROM role_traits WHERE Key_name = 'PRIMARY'`).Scan(&indexes).Error; err == nil {
			for _, ix := range indexes {
				if ix.ColumnName == "preset_type" {
					hasPresetInPK = true
					break
				}
			}
		}
	}
	if hasPresetInPK {
		utils.LogStartup("migrate.role_traits", "status", "skip", "reason", "PK already includes preset_type")
		return
	}

	utils.LogStartup("migrate.role_traits", "status", "begin", "backupTable", roleTraitsBackupTable)
	start := time.Now()

	// 4. 创建备份表(结构+数据)
	if err := db.Exec(fmt.Sprintf("CREATE TABLE %s AS SELECT * FROM role_traits", roleTraitsBackupTable)).Error; err != nil {
		utils.LogWithCtx(nil, "migrate.role_traits", "备份失败 | err=%v", err)
		return
	}
	utils.LogStartup("migrate.role_traits", "step", "backup_created", "table", roleTraitsBackupTable)

	// 5. 删主键 + 加三段联合主键
	if err := db.Exec("ALTER TABLE role_traits DROP PRIMARY KEY").Error; err != nil {
		utils.LogWithCtx(nil, "migrate.role_traits", "删主键失败 | err=%v", err)
		return
	}
	if err := db.Exec("ALTER TABLE role_traits ADD PRIMARY KEY (user_id, role_id, preset_type)").Error; err != nil {
		utils.LogWithCtx(nil, "migrate.role_traits", "加联合主键失败 | err=%v", err)
		return
	}
	utils.LogStartup("migrate.role_traits", "step", "alter_pk", "status", "ok")

	utils.LogStartup("migrate.role_traits", "status", "done",
		"latency", fmt.Sprintf("%dms", time.Since(start).Milliseconds()),
		"backupKeepDays", "30")
}

// RunRoleTraitsMigration 直接调用入口(用于 main.go 启动时调用)。
func RunRoleTraitsMigration() {
	EnsureRoleTraitsPKMigrated(config.GetDB())
}
