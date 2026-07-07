package repository

import (
	"context"
	"echo-core/config"
	"echo-core/models"
	"time"

	"gorm.io/gorm"
)

type FileRepository struct {
	db *gorm.DB
}

func NewFileRepository() *FileRepository {
	return &FileRepository{db: config.GetDB()}
}

// GetByID 根据ID获取单条记录
func (r *FileRepository) GetByID(ctx context.Context, id uint) (*models.File, error) {
	start := time.Now()
	var file models.File
	err := r.db.WithContext(ctx).First(&file, id).Error
	repoLog(ctx, "FileRepository.GetByID", start, err, "id="+uintToStr(id))
	if err != nil {
		return nil, err
	}
	return &file, nil
}

// GetByKey 根据key获取记录
func (r *FileRepository) GetByKey(ctx context.Context, key string) (*models.File, error) {
	start := time.Now()
	var file models.File
	err := r.db.WithContext(ctx).Where("`key` = ?", key).First(&file).Error
	repoLog(ctx, "FileRepository.GetByKey", start, err, "key="+key)
	if err != nil {
		return nil, err
	}
	return &file, nil
}

// GetByUserID 根据用户ID获取文件列表
func (r *FileRepository) GetByUserID(ctx context.Context, userId string) ([]models.File, error) {
	start := time.Now()
	var files []models.File
	err := r.db.WithContext(ctx).Where("user_id = ?", userId).Find(&files).Error
	repoLog(ctx, "FileRepository.GetByUserID", start, err, "userId="+userId+" count="+intToStr(len(files)))
	return files, err
}

// Create 创建
func (r *FileRepository) Create(ctx context.Context, file *models.File) error {
	start := time.Now()
	err := r.db.WithContext(ctx).Create(file).Error
	extra := "key=" + file.Key
	if file.UserId != "" {
		extra += " userId=" + file.UserId
	}
	repoLog(ctx, "FileRepository.Create", start, err, extra)
	return err
}

// CreateWithTx 在事务中创建
func (r *FileRepository) CreateWithTx(ctx context.Context, tx *gorm.DB, file *models.File) error {
	start := time.Now()
	err := tx.WithContext(ctx).Create(file).Error
	extra := "key=" + file.Key
	if file.UserId != "" {
		extra += " userId=" + file.UserId
	}
	repoLog(ctx, "FileRepository.CreateWithTx", start, err, extra)
	return err
}

// Update 更新
func (r *FileRepository) Update(ctx context.Context, id uint, updates map[string]interface{}) error {
	start := time.Now()
	err := r.db.WithContext(ctx).Model(&models.File{}).Where("id = ?", id).Updates(updates).Error
	repoLog(ctx, "FileRepository.Update", start, err, "id="+uintToStr(id))
	return err
}

// UpdateWithTx 在事务中更新
func (r *FileRepository) UpdateWithTx(ctx context.Context, tx *gorm.DB, id uint, updates map[string]interface{}) error {
	start := time.Now()
	err := tx.WithContext(ctx).Model(&models.File{}).Where("id = ?", id).Updates(updates).Error
	repoLog(ctx, "FileRepository.UpdateWithTx", start, err, "id="+uintToStr(id))
	return err
}

// Delete 软删除
func (r *FileRepository) Delete(ctx context.Context, id uint) error {
	start := time.Now()
	err := r.db.WithContext(ctx).Model(&models.File{}).Where("id = ?", id).Update("status", 2).Error
	repoLog(ctx, "FileRepository.Delete", start, err, "id="+uintToStr(id))
	return err
}

// DeleteWithTx 在事务中软删除
func (r *FileRepository) DeleteWithTx(ctx context.Context, tx *gorm.DB, id uint) error {
	start := time.Now()
	err := tx.WithContext(ctx).Model(&models.File{}).Where("id = ?", id).Update("status", 2).Error
	repoLog(ctx, "FileRepository.DeleteWithTx", start, err, "id="+uintToStr(id))
	return err
}

// HardDelete 硬删除
func (r *FileRepository) HardDelete(ctx context.Context, id uint) error {
	start := time.Now()
	err := r.db.WithContext(ctx).Unscoped().Delete(&models.File{}, id).Error
	repoLog(ctx, "FileRepository.HardDelete", start, err, "id="+uintToStr(id))
	return err
}

// GetList 获取文件列表
func (r *FileRepository) GetList(ctx context.Context, userId string, fileType int, status int, page, pageSize int) ([]models.File, int64, error) {
	start := time.Now()
	var files []models.File
	var total int64

	query := r.db.WithContext(ctx).Model(&models.File{})

	if userId != "" {
		query = query.Where("user_id = ?", userId)
	}
	if fileType > 0 {
		query = query.Where("file_type = ?", fileType)
	}
	if status > 0 {
		query = query.Where("status = ?", status)
	}

	query.Count(&total)

	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	offset := (page - 1) * pageSize

	err := query.Order("created_at DESC").Limit(pageSize).Offset(offset).Find(&files).Error
	repoLog(ctx, "FileRepository.GetList", start, err,
		"userId="+userId+" fileType="+intToStr(fileType)+" status="+intToStr(status)+
			" page="+intToStr(page)+" pageSize="+intToStr(pageSize)+" total="+intToStr(int(total))+" returned="+intToStr(len(files)))
	if err != nil {
		return nil, 0, err
	}

	return files, total, nil
}
