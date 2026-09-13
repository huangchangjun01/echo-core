// migrate_personas_pk.go
// personas 表主键修复脚本（PRD 历史教训 842d7c5）。
//
// 设计要点：
//   - personas 是 echo-ai 维护的表（DDL 在 echo-ai/database/schema_sql.py）；
//     但 echo-core 通过它做 (user_id) 查询，因此迁移脚本放在 echo-core 启动时调用一次。
//   - 步骤：
//     1) 创建 personas_backup_202608 备份表（保留 30 天后人工 DROP）
//     2) ALTER personas 主键 (user_id) → (user_id, role_id)
//     3) 如遇重复 (user_id, role_id) 记录，保留最新一条
//   - 仅在启动时执行一次，幂等（重复执行直接 return）。
//
// 触发方式：在 config.InitDB() 末尾调用 EnsurePersonasPKMigrated(DB)。
package scripts

import (
	"echo-core/config"
	"echo-core/utils"
	"fmt"
	"time"

	"gorm.io/gorm"
)

const personasBackupTable = "personas_backup_202608"

// EnsurePersonasPKMigrated 执行 personas 表主键迁移（幂等）。
//
// 在 echo-core 启动时由 config.InitDB() 末尾调用一次。
// 失败仅记 WARN 日志（不阻塞启动,确保业务可用；下次启动重试）。
func EnsurePersonasPKMigrated(db *gorm.DB) {
	if db == nil {
		return
	}

	// 1. 检查 personas 表是否存在
	if !db.Migrator().HasTable("personas") {
		utils.LogStartup("migrate.personas", "status", "skip", "reason", "personas table not exists")
		return
	}

	// 2. 检查备份表是否已存在 → 幂等判断
	if db.Migrator().HasTable(personasBackupTable) {
		utils.LogStartup("migrate.personas", "status", "skip", "reason", "backup exists, migration already done")
		return
	}

	utils.LogStartup("migrate.personas", "status", "begin", "backupTable", personasBackupTable)
	start := time.Now()

	// 3. 创建备份表（结构+数据）
	if err := db.Exec(fmt.Sprintf("CREATE TABLE %s AS SELECT * FROM personas", personasBackupTable)).Error; err != nil {
		utils.LogWithCtx(nil, "migrate.personas", "备份失败 | err=%v", err)
		return
	}
	utils.LogStartup("migrate.personas", "step", "backup_created", "table", personasBackupTable)

	// 4. 检查 personas 是否已有 role_id 列
	if !db.Migrator().HasColumn("personas", "role_id") {
		if err := db.Exec("ALTER TABLE personas ADD COLUMN role_id VARCHAR(128) NOT NULL DEFAULT 'default'").Error; err != nil {
			utils.LogWithCtx(nil, "migrate.personas", "加列失败 | err=%v", err)
			return
		}
		utils.LogStartup("migrate.personas", "step", "add_column_role_id", "status", "ok")
	}

	// 5. 重复数据处理：(user_id, role_id) 重复时保留 updated_at 最新那条
	// 注意：personas 表无 id 列,使用 updated_at 判定新旧
	if err := db.Exec(`
		DELETE p1 FROM personas p1
		INNER JOIN personas p2
		WHERE p1.user_id = p2.user_id
		  AND p1.role_id = p2.role_id
		  AND p1.updated_at < p2.updated_at
	`).Error; err != nil {
		utils.LogWithCtx(nil, "migrate.personas", "去重失败 | err=%v", err)
		return
	}
	utils.LogStartup("migrate.personas", "step", "dedupe_by_user_role", "status", "ok")

	// 6. 删主键 + 加联合主键
	if err := db.Exec("ALTER TABLE personas DROP PRIMARY KEY").Error; err != nil {
		utils.LogWithCtx(nil, "migrate.personas", "删主键失败 | err=%v", err)
		return
	}
	if err := db.Exec("ALTER TABLE personas ADD PRIMARY KEY (user_id, role_id)").Error; err != nil {
		utils.LogWithCtx(nil, "migrate.personas", "加联合主键失败 | err=%v", err)
		return
	}
	utils.LogStartup("migrate.personas", "step", "alter_pk", "status", "ok")

	utils.LogStartup("migrate.personas", "status", "done",
		"latency", fmt.Sprintf("%dms", time.Since(start).Milliseconds()),
		"backupKeepDays", "30")
}

// RunPersonasMigration 直接调用入口（用于 main.go 启动时调用）。
func RunPersonasMigration() {
	EnsurePersonasPKMigrated(config.GetDB())
}
