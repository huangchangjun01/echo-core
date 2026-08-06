package repository

import (
	"context"
	"echo-core/config"
	"echo-core/models"
	"echo-core/utils"
	"time"

	"gorm.io/gorm"
)

// RecallMemoryRepository 回忆记忆主题 + 源文件的持久化封装。
// 全部继承 WithContext(ctx)，日志走统一 repoLog。
type RecallMemoryRepository struct {
	db *gorm.DB
}

func NewRecallMemoryRepository() *RecallMemoryRepository {
	return &RecallMemoryRepository{db: config.GetDB()}
}

// ===== 主题（recall_memory）=====

// CreateWithTx 事务内创建记忆主题
func (r *RecallMemoryRepository) CreateWithTx(ctx context.Context, tx *gorm.DB, m *models.RecallMemory) error {
	start := time.Now()
	err := tx.WithContext(ctx).Create(m).Error
	repoLog(ctx, "RecallMemoryRepository.CreateWithTx", start, err, "memoryId="+m.MemoryId+" userId="+m.UserId)
	return err
}

// GetByMemoryId 按 memoryId 取可用主题
func (r *RecallMemoryRepository) GetByMemoryId(ctx context.Context, memoryId string) (*models.RecallMemory, error) {
	start := time.Now()
	var m models.RecallMemory
	err := r.db.WithContext(ctx).
		Where("memory_id = ? AND status = ?", memoryId, models.RecallStatusActive).
		First(&m).Error
	repoLog(ctx, "RecallMemoryRepository.GetByMemoryId", start, err, "memoryId="+memoryId)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// UpdateMemoryFieldsWithTx 事务内更新主题/主观描述（编辑记忆）。
//
// 同步把 parse_status 重置为 PENDING：当 needReparse=true 时由 service 再推到 RUNNING；
// 即使 needReparse=false 也保持 0（让前端看到状态清晰）。
//   - 写入 updated_at 由 GORM autoUpdateTime 自动处理。
func (r *RecallMemoryRepository) UpdateMemoryFieldsWithTx(ctx context.Context, tx *gorm.DB, memoryId, topic, subjectiveDesc string, parseStatus int) error {
	start := time.Now()
	err := tx.WithContext(ctx).Model(&models.RecallMemory{}).
		Where("memory_id = ?", memoryId).
		Updates(map[string]any{
			"topic":           topic,
			"subjective_desc": subjectiveDesc,
			"parse_status":    parseStatus,
		}).Error
	repoLog(ctx, "RecallMemoryRepository.UpdateMemoryFieldsWithTx", start, err,
		"memoryId="+memoryId+" parseStatus="+intToStr(parseStatus))
	return err
}

// ListActiveSourceFileKeys 列出某记忆下所有可用源文件的 fileKey（用于编辑场景的增量 diff）。
func (r *RecallMemoryRepository) ListActiveSourceFileKeys(ctx context.Context, memoryId string) ([]string, error) {
	start := time.Now()
	var keys []string
	err := r.db.WithContext(ctx).Model(&models.RecallSourceFile{}).
		Where("memory_id = ? AND status = ?", memoryId, models.RecallStatusActive).
		Order("created_at ASC").
		Pluck("file_key", &keys).Error
	repoLog(ctx, "RecallMemoryRepository.ListActiveSourceFileKeys", start, err,
		"memoryId="+memoryId+" count="+intToStr(len(keys)))
	return keys, err
}

// ListActiveSourceFileTypes 列出某记忆下所有可用源文件的 fileKey → fileType 映射。
// 用于编辑场景：把 DB 权威 fileType 喂给 service 做"扩展名 > DB > 请求"归一，
// 修复"mp4 被前端误标为文本"的 bug。
func (r *RecallMemoryRepository) ListActiveSourceFileTypes(ctx context.Context, memoryId string) map[string]int {
	start := time.Now()
	type kv struct {
		FileKey  string
		FileType int
	}
	var rows []kv
	err := r.db.WithContext(ctx).Model(&models.RecallSourceFile{}).
		Select("file_key, file_type").
		Where("memory_id = ? AND status = ?", memoryId, models.RecallStatusActive).
		Scan(&rows).Error
	if err != nil {
		utils.LogWithCtx(ctx, "RecallMemoryRepository.ListActiveSourceFileTypes", "失败 | err=%v", err)
		return map[string]int{}
	}
	out := make(map[string]int, len(rows))
	for _, r := range rows {
		out[r.FileKey] = r.FileType
	}
	repoLog(ctx, "RecallMemoryRepository.ListActiveSourceFileTypes", start, nil,
		"memoryId="+memoryId+" count="+intToStr(len(out)))
	return out
}

// HardDeleteSourceWithTx 事务内物理删除源文件记录（按 memoryId + fileKey）。
//
// 与 SoftDeleteSourceWithTx 的区别：本方法走硬删，方便编辑场景下"先 INSERT 再清掉旧记录"的实现；
// 同时硬删的记录在 data_scope='source' 审计下不会再现。
func (r *RecallMemoryRepository) HardDeleteSourceWithTx(ctx context.Context, tx *gorm.DB, memoryId, fileKey string) (int64, error) {
	start := time.Now()
	res := tx.WithContext(ctx).Where("memory_id = ? AND file_key = ?", memoryId, fileKey).
		Delete(&models.RecallSourceFile{})
	repoLog(ctx, "RecallMemoryRepository.HardDeleteSourceWithTx", start, res.Error,
		"memoryId="+memoryId+" affected="+intToStr(int(res.RowsAffected)))
	return res.RowsAffected, res.Error
}

// UpdateMdContent 单独更新 md_content 字段（不触发 updated_at 之外的其它字段变化）
func (r *RecallMemoryRepository) UpdateMdContent(ctx context.Context, memoryId, mdContent string) error {
	start := time.Now()
	err := r.db.WithContext(ctx).Model(&models.RecallMemory{}).
		Where("memory_id = ?", memoryId).
		Update("md_content", mdContent).Error
	repoLog(ctx, "RecallMemoryRepository.UpdateMdContent", start, err, "memoryId="+memoryId+" bytes="+intToStr(len(mdContent)))
	return err
}

// UpdateParseStatus 更新解析状态（0待解析 1解析中 2完成 3失败）。
//
// 用途：save 后由 service 推进到 RUNNING；触发失败或 echo-ai 不可达时置 FAILED。
// echo-ai 也会通过 MySQL 直写或内部回调接口回写最终状态（2/3）覆盖本字段。
func (r *RecallMemoryRepository) UpdateParseStatus(ctx context.Context, memoryId string, status int) error {
	start := time.Now()
	err := r.db.WithContext(ctx).Model(&models.RecallMemory{}).
		Where("memory_id = ?", memoryId).
		Update("parse_status", status).Error
	repoLog(ctx, "RecallMemoryRepository.UpdateParseStatus", start, err, "memoryId="+memoryId+" status="+intToStr(status))
	return err
}

// ExistsTopic 校验 (userId, roleId, topic) 在可用状态下是否已存在
func (r *RecallMemoryRepository) ExistsTopic(ctx context.Context, userId, roleId, topic string) (bool, error) {
	start := time.Now()
	var cnt int64
	err := r.db.WithContext(ctx).Model(&models.RecallMemory{}).
		Where("user_id = ? AND role_id = ? AND topic = ? AND status = ?", userId, roleId, topic, models.RecallStatusActive).
		Count(&cnt).Error
	repoLog(ctx, "RecallMemoryRepository.ExistsTopic", start, err, "userId="+userId+" roleId="+roleId+" topic="+topic+" cnt="+intToStr(int(cnt)))
	return cnt > 0, err
}

// ListByUserRole 列出某用户某角色下的可用主题（按创建时间倒序）
func (r *RecallMemoryRepository) ListByUserRole(ctx context.Context, userId, roleId string) ([]models.RecallMemory, error) {
	start := time.Now()
	var list []models.RecallMemory
	query := r.db.WithContext(ctx).Model(&models.RecallMemory{}).
		Where("user_id = ? AND status = ?", userId, models.RecallStatusActive)
	if roleId != "" {
		query = query.Where("role_id = ?", roleId)
	}
	err := query.Order("created_at DESC").Find(&list).Error
	repoLog(ctx, "RecallMemoryRepository.ListByUserRole", start, err, "userId="+userId+" roleId="+roleId+" count="+intToStr(len(list)))
	return list, err
}

// SoftDeleteWithTx 事务内软删除主题
func (r *RecallMemoryRepository) SoftDeleteWithTx(ctx context.Context, tx *gorm.DB, memoryId string) error {
	start := time.Now()
	err := tx.WithContext(ctx).Model(&models.RecallMemory{}).
		Where("memory_id = ?", memoryId).
		Update("status", models.RecallStatusDeleted).Error
	repoLog(ctx, "RecallMemoryRepository.SoftDeleteWithTx", start, err, "memoryId="+memoryId)
	return err
}

// HardDeleteWithTx 事务内硬删除主题（用于级联删除：让用户从 DB 中真正看不到记录）。
func (r *RecallMemoryRepository) HardDeleteWithTx(ctx context.Context, tx *gorm.DB, memoryId string) error {
	start := time.Now()
	err := tx.WithContext(ctx).Where("memory_id = ?", memoryId).
		Delete(&models.RecallMemory{}).Error
	repoLog(ctx, "RecallMemoryRepository.HardDeleteWithTx", start, err, "memoryId="+memoryId)
	return err
}

// ===== 源文件（recall_source_file）=====

// BatchCreateSourceWithTx 事务内批量创建源文件
func (r *RecallMemoryRepository) BatchCreateSourceWithTx(ctx context.Context, tx *gorm.DB, files []models.RecallSourceFile) error {
	if len(files) == 0 {
		return nil
	}
	start := time.Now()
	err := tx.WithContext(ctx).Create(&files).Error
	repoLog(ctx, "RecallMemoryRepository.BatchCreateSourceWithTx", start, err, "count="+intToStr(len(files)))
	return err
}

// ListSourceByMemoryId 列出某主题下的可用源文件
func (r *RecallMemoryRepository) ListSourceByMemoryId(ctx context.Context, memoryId string) ([]models.RecallSourceFile, error) {
	start := time.Now()
	var list []models.RecallSourceFile
	err := r.db.WithContext(ctx).
		Where("memory_id = ? AND status = ?", memoryId, models.RecallStatusActive).
		Order("created_at ASC").Find(&list).Error
	repoLog(ctx, "RecallMemoryRepository.ListSourceByMemoryId", start, err, "memoryId="+memoryId+" count="+intToStr(len(list)))
	return list, err
}

// SoftDeleteSourceWithTx 事务内软删除单个源文件（按 memoryId + fileKey）
func (r *RecallMemoryRepository) SoftDeleteSourceWithTx(ctx context.Context, tx *gorm.DB, memoryId, fileKey string) (int64, error) {
	start := time.Now()
	res := tx.WithContext(ctx).Model(&models.RecallSourceFile{}).
		Where("memory_id = ? AND file_key = ? AND status = ?", memoryId, fileKey, models.RecallStatusActive).
		Update("status", models.RecallStatusDeleted)
	repoLog(ctx, "RecallMemoryRepository.SoftDeleteSourceWithTx", start, res.Error, "memoryId="+memoryId+" affected="+intToStr(int(res.RowsAffected)))
	return res.RowsAffected, res.Error
}

// SoftDeleteAllSourceWithTx 事务内软删除某主题的全部源文件
func (r *RecallMemoryRepository) SoftDeleteAllSourceWithTx(ctx context.Context, tx *gorm.DB, memoryId string) error {
	start := time.Now()
	err := tx.WithContext(ctx).Model(&models.RecallSourceFile{}).
		Where("memory_id = ? AND status = ?", memoryId, models.RecallStatusActive).
		Update("status", models.RecallStatusDeleted).Error
	repoLog(ctx, "RecallMemoryRepository.SoftDeleteAllSourceWithTx", start, err, "memoryId="+memoryId)
	return err
}

// HardDeleteAllSourceWithTx 事务内硬删除某主题的全部源文件（级联删除场景）。
func (r *RecallMemoryRepository) HardDeleteAllSourceWithTx(ctx context.Context, tx *gorm.DB, memoryId string) error {
	start := time.Now()
	err := tx.WithContext(ctx).Where("memory_id = ?", memoryId).
		Delete(&models.RecallSourceFile{}).Error
	repoLog(ctx, "RecallMemoryRepository.HardDeleteAllSourceWithTx", start, err, "memoryId="+memoryId)
	return err
}
