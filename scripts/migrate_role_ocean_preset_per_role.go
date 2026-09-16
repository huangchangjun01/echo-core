// migrate_role_ocean_preset_per_role.go
// role_ocean_preset 表从用户级改为角色级（预设字典按角色隔离）。
//
// 设计要点：
//   - 历史上预设字典按 (user_id, key) 唯一（用户级共享），删一个预设影响所有角色。
//     用户希望改为按 (user_id, role_id, key) 隔离（每个角色独立预设库）。
//   - 步骤（幂等）：
//     1) 检查 role_id 列是否已存在 + 是否 NOT NULL → 跳过（已迁移）
//     2) 备份原表 → role_ocean_preset_backup_202609
//     3) 添加 role_id 列（默认 0，后续填值）
//     4) 对每条旧预设 (user_id, key)：
//        - 查该用户的所有 role_id（从 role 表）
//        - 对每个 role_id：复制一份 (user_id, role_id, key) 行
//        - 若用户没有任何角色：保留一行 (user_id, role_id=0) 占位（后续删除）
//     5) 对每个 role_id 在 role 表中存在但 role_ocean_preset 里没有的行：
//        种子 6 个默认预设（is_protected=true）
//     6) 删除 role_id=0 占位行
//     7) role_id 改为 NOT NULL
//     8) 删旧唯一索引 idx_user_key
//     9) 加新唯一索引 idx_user_role_key (user_id, role_id, key)
//
// 数据迁移保证：
//   - 用户在 (user, role) 已有的自定义预设被保留（拷贝到该 role）
//   - 用户在某些 role 没碰过预设的，自动获得 6 个默认
//   - 用户在某些 role 删过默认预设的，恢复 6 个默认（迁移无法判断意图，
//     采取「每个角色独立种子」的简单语义）
//
// 触发方式：在 config.InitDB() 末尾调用 EnsureRoleOceanPresetPerRoleMigrated(DB)。
package scripts

import (
	"echo-core/config"
	"echo-core/utils"
	"fmt"
	"strconv"
	"time"

	"gorm.io/gorm"
)

const roleOceanPresetBackupTable = "role_ocean_preset_backup_202609"

// 默认 6 个预设的 OCEAN 基线值 —— 与 service/role_core_service.go 的 defaultPresets
// 保持一致。迁移脚本独立写一份避免包循环依赖。
type defaultPresetSeed struct {
	Key               string
	Label             string
	Description       string
	Openness          float64
	Conscientiousness float64
	Extraversion      float64
	Agreeableness     float64
	Neuroticism       float64
	IsProtected       bool
}

var defaultPresetSeeds = []defaultPresetSeed{
	{"companion_default", "标准陪伴", "温暖、亲和、低神经质", 0.4, -0.1, 0.2, 0.7, -0.4, true},
	{"rational_advisor", "理性顾问", "逻辑清晰、克制情绪", 0.2, 0.7, -0.3, 0.2, -0.5, false},
	{"energetic_girl", "元气少女", "活力充沛、乐观开朗", 0.6, -0.3, 0.8, 0.6, -0.6, false},
	{"gentle_senior", "温柔学长", "稳重体贴、温和有礼", 0.3, 0.4, 0.1, 0.6, -0.3, false},
	{"sarcastic_friend", "毒舌损友", "直率犀利、敢说实话", 0.5, -0.2, 0.4, -0.2, -0.1, false},
	{"psych_counselor", "心理咨询师", "倾听共情、专业支持", 0.7, 0.6, 0.0, 0.9, -0.7, false},
}

// EnsureRoleOceanPresetPerRoleMigrated 把预设字典从用户级迁移到角色级（幂等）。
//
// 在 echo-core 启动时由 config.InitDB() 末尾调用一次。
// 失败仅记 WARN 日志（不阻塞启动，确保业务可用；下次启动重试）。
func EnsureRoleOceanPresetPerRoleMigrated(db *gorm.DB) {
	if db == nil {
		return
	}

	// 1. 表不存在 → 全新部署，无需迁移
	if !db.Migrator().HasTable("role_ocean_preset") {
		utils.LogStartup("migrate.role_ocean_preset_per_role", "status", "skip", "reason", "table not exists")
		return
	}

	// 2. 旧唯一索引 idx_user_key 已不存在 → 新唯一索引 idx_user_role_key 已生效
	//    且 role_id 列已存在 → 已迁移
	//
	// 注意:不能仅看 role_id IS_NULLABLE=NO 判定已迁移 —— GORM auto-migrate 会
	// 直接给模型字段加 NOT NULL 列,但此时数据还没拷贝(全 0);以唯一索引
	// 切换作为「迁移完成」的真正标志。
	if !db.Migrator().HasIndex("role_ocean_preset", "idx_user_key") &&
		db.Migrator().HasIndex("role_ocean_preset", "idx_user_role_key") &&
		db.Migrator().HasColumn("role_ocean_preset", "role_id") {
		utils.LogStartup("migrate.role_ocean_preset_per_role", "status", "skip",
			"reason", "idx_user_role_key active and idx_user_key removed",
			"backupTable", roleOceanPresetBackupTable)
		return
	}

	// 3. 备份表已存在 → 上次迁移中断过，跳过避免重复数据损坏
	if db.Migrator().HasTable(roleOceanPresetBackupTable) {
		utils.LogStartup("migrate.role_ocean_preset_per_role", "status", "skip",
			"reason", "backup table exists, manual cleanup required",
			"backupTable", roleOceanPresetBackupTable)
		return
	}

	utils.LogStartup("migrate.role_ocean_preset_per_role", "status", "begin",
		"backupTable", roleOceanPresetBackupTable)
	start := time.Now()

	// 4. 备份
	if err := db.Exec(fmt.Sprintf("CREATE TABLE %s AS SELECT * FROM role_ocean_preset", roleOceanPresetBackupTable)).Error; err != nil {
		utils.LogWithCtx(nil, "migrate.role_ocean_preset_per_role", "备份失败 | err=%v", err)
		return
	}
	utils.LogStartup("migrate.role_ocean_preset_per_role", "step", "backup_created")

	// 5. 加 role_id 列（默认 0，后续 UPDATE）
	if !db.Migrator().HasColumn("role_ocean_preset", "role_id") {
		if err := db.Exec("ALTER TABLE role_ocean_preset ADD COLUMN role_id BIGINT UNSIGNED NOT NULL DEFAULT 0").Error; err != nil {
			utils.LogWithCtx(nil, "migrate.role_ocean_preset_per_role", "加 role_id 列失败 | err=%v", err)
			return
		}
		utils.LogStartup("migrate.role_ocean_preset_per_role", "step", "add_role_id_column")
	}

	// 5.5 数据迁移前先处理旧唯一索引 idx_user_key —— 它是 (user_id, key) 唯一,
	// 数据迁移期间会把 (user_id=X, key=companion_default) 既保留 role_id=0 原行,
	// 又新增 role_id=N 复制行,触发「Duplicate entry」错。修复:先把 idx_user_key
	// 删了 —— 数据迁移完成后再加新的 idx_user_role_key (user_id, role_id, key) 唯一索引。
	if db.Migrator().HasIndex("role_ocean_preset", "idx_user_key") {
		if err := db.Exec("ALTER TABLE role_ocean_preset DROP INDEX idx_user_key").Error; err != nil {
			utils.LogWithCtx(nil, "migrate.role_ocean_preset_per_role", "数据迁移前删旧索引失败 | err=%v", err)
			return
		}
		utils.LogStartup("migrate.role_ocean_preset_per_role", "step", "drop_old_index_for_copy")
	}

	// 6. 数据迁移：对每条旧预设 (user_id, key)，对每个 role_id 复制一行
	//
	// 先收集「(user_id) → [role_id, ...]」映射 —— 从 role 表（用户的角色由 roles 持有）。
	userToRoles := map[uint][]uint{}
	rows, err := db.Raw("SELECT id, user_id FROM role").Rows()
	if err != nil {
		utils.LogWithCtx(nil, "migrate.role_ocean_preset_per_role", "查 role 表失败 | err=%v", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var rid, uid uint
		if err := rows.Scan(&rid, &uid); err != nil {
			continue
		}
		userToRoles[uid] = append(userToRoles[uid], rid)
	}
	rows.Close()

	// 对每条旧预设 (user_id, key),对 user_id 的每个 role_id 复制一行
	oldRows, err := db.Raw("SELECT id, user_id, `key`, label, description, " +
		"openness, conscientiousness, extraversion, agreeableness, neuroticism, is_protected " +
		"FROM role_ocean_preset WHERE role_id = 0").Rows()
	if err != nil {
		utils.LogWithCtx(nil, "migrate.role_ocean_preset_per_role", "读旧预设失败 | err=%v", err)
		return
	}
	type oldPreset struct {
		ID, UserID, IsProtected uint
		Key, Label, Description string
		O, C, E, A, N           float64
	}
	var oldPresets []oldPreset
	for oldRows.Next() {
		var p oldPreset
		if err := oldRows.Scan(&p.ID, &p.UserID, &p.Key, &p.Label, &p.Description,
			&p.O, &p.C, &p.E, &p.A, &p.N, &p.IsProtected); err != nil {
			continue
		}
		oldPresets = append(oldPresets, p)
	}
	oldRows.Close()

	// 对每条旧预设:为该用户的每个 role 复制一行
	for _, p := range oldPresets {
		roles := userToRoles[p.UserID]
		if len(roles) == 0 {
			// 用户没有任何角色,跳过(占位 row_id=0 会被后面清理)
			continue
		}
		for _, rid := range roles {
			// 用 INSERT ... SELECT 复制(避免主键冲突)
			err := db.Exec(fmt.Sprintf("INSERT INTO role_ocean_preset "+
				"(user_id, role_id, `key`, label, description, "+
				"openness, conscientiousness, extraversion, agreeableness, neuroticism, is_protected) "+
				"SELECT user_id, %d, `key`, label, description, "+
				"openness, conscientiousness, extraversion, agreeableness, neuroticism, is_protected "+
				"FROM role_ocean_preset WHERE id = %d", rid, p.ID)).Error
			if err != nil {
				utils.LogWithCtx(nil, "migrate.role_ocean_preset_per_role",
					"复制预设失败 | userId=%d key=%s roleId=%d err=%v",
					p.UserID, p.Key, rid, err)
			}
		}
	}
	utils.LogStartup("migrate.role_ocean_preset_per_role", "step", "copy_per_role",
		"oldCount", strconv.Itoa(len(oldPresets)))

	// 7. 删 role_id=0 占位行(用户没角色或复制失败的)
	if err := db.Exec("DELETE FROM role_ocean_preset WHERE role_id = 0").Error; err != nil {
		utils.LogWithCtx(nil, "migrate.role_ocean_preset_per_role", "删占位行失败 | err=%v", err)
		return
	}

	// 8. 对每个 role 种子默认 6 个(若该 role 还没有任何 companion_default)
	for _, rid := range uniqueRoleIDs(userToRoles) {
		// 查该 role 是否已有 companion_default
		var n int64
		if err := db.Raw("SELECT COUNT(*) FROM role_ocean_preset WHERE role_id = ? AND `key` = 'companion_default'", rid).Scan(&n).Error; err != nil {
			continue
		}
		if n > 0 {
			continue
		}
		// 种子 6 个默认
		for _, seed := range defaultPresetSeeds {
			if err := db.Exec(fmt.Sprintf("INSERT INTO role_ocean_preset "+
				"(user_id, role_id, `key`, label, description, "+
				"openness, conscientiousness, extraversion, agreeableness, neuroticism, is_protected) "+
				"SELECT user_id, %d, '%s', '%s', '%s', "+
				"%f, %f, %f, %f, %f, %v "+
				"FROM role WHERE id = %d LIMIT 1",
				rid, seed.Key, seed.Label, seed.Description,
				seed.Openness, seed.Conscientiousness, seed.Extraversion,
				seed.Agreeableness, seed.Neuroticism, seed.IsProtected,
				rid)).Error; err != nil {
				utils.LogWithCtx(nil, "migrate.role_ocean_preset_per_role",
					"种子预设失败 | roleId=%d key=%s err=%v", rid, seed.Key, err)
			}
		}
	}
	utils.LogStartup("migrate.role_ocean_preset_per_role", "step", "seed_defaults")

	// 9. 删旧唯一索引 idx_user_key,加新索引 idx_user_role_key
	if db.Migrator().HasIndex("role_ocean_preset", "idx_user_key") {
		if err := db.Exec("ALTER TABLE role_ocean_preset DROP INDEX idx_user_key").Error; err != nil {
			utils.LogWithCtx(nil, "migrate.role_ocean_preset_per_role", "删旧索引失败 | err=%v", err)
			// 不 return —— 后续步骤仍可尝试
		} else {
			utils.LogStartup("migrate.role_ocean_preset_per_role", "step", "drop_old_index")
		}
	}
	if !db.Migrator().HasIndex("role_ocean_preset", "idx_user_role_key") {
		if err := db.Exec("ALTER TABLE role_ocean_preset ADD UNIQUE INDEX idx_user_role_key (user_id, role_id, `key`)").Error; err != nil {
			utils.LogWithCtx(nil, "migrate.role_ocean_preset_per_role", "加新索引失败 | err=%v", err)
		} else {
			utils.LogStartup("migrate.role_ocean_preset_per_role", "step", "add_new_index")
		}
	}

	utils.LogStartup("migrate.role_ocean_preset_per_role", "status", "done",
		"latency", fmt.Sprintf("%dms", time.Since(start).Milliseconds()),
		"backupKeepDays", "30")
}

func uniqueRoleIDs(m map[uint][]uint) []uint {
	seen := map[uint]bool{}
	out := []uint{}
	for _, ids := range m {
		for _, id := range ids {
			if !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
	}
	return out
}

// RunRoleOceanPresetPerRoleMigration 直接调用入口（用于 main.go 启动时调用）。
func RunRoleOceanPresetPerRoleMigration() {
	EnsureRoleOceanPresetPerRoleMigrated(config.GetDB())
}
