package repository

import (
	"echo-core/config"
	"echo-core/models"
	"gorm.io/gorm"
)

type FileRepository struct {
	db *gorm.DB
}

func NewFileRepository() *FileRepository {
	return &FileRepository{db: config.GetDB()}
}

// GetByID 根据ID获取单条记录
func (r *FileRepository) GetByID(id uint) (*models.File, error) {
	var file models.File
	if err := r.db.First(&file, id).Error; err != nil {
		return nil, err
	}
	return &file, nil
}

// GetByKey 根据key获取记录
func (r *FileRepository) GetByKey(key string) (*models.File, error) {
	var file models.File
	if err := r.db.Where("`key` = ?", key).First(&file).Error; err != nil {
		return nil, err
	}
	return &file, nil
}

// GetByUserID 根据用户ID获取文件列表
func (r *FileRepository) GetByUserID(userId string) ([]models.File, error) {
	var files []models.File
	err := r.db.Where("user_id = ?", userId).Find(&files).Error
	return files, err
}

// Create 创建
func (r *FileRepository) Create(file *models.File) error {
	return r.db.Create(file).Error
}

// CreateWithTx 在事务中创建
func (r *FileRepository) CreateWithTx(tx *gorm.DB, file *models.File) error {
	return tx.Create(file).Error
}

// Update 更新
func (r *FileRepository) Update(id uint, updates map[string]interface{}) error {
	return r.db.Model(&models.File{}).Where("id = ?", id).Updates(updates).Error
}

// UpdateWithTx 在事务中更新
func (r *FileRepository) UpdateWithTx(tx *gorm.DB, id uint, updates map[string]interface{}) error {
	return tx.Model(&models.File{}).Where("id = ?", id).Updates(updates).Error
}

// Delete 软删除
func (r *FileRepository) Delete(id uint) error {
	return r.db.Model(&models.File{}).Where("id = ?", id).Update("status", 2).Error
}

// DeleteWithTx 在事务中软删除
func (r *FileRepository) DeleteWithTx(tx *gorm.DB, id uint) error {
	return tx.Model(&models.File{}).Where("id = ?", id).Update("status", 2).Error
}

// HardDelete 硬删除
func (r *FileRepository) HardDelete(id uint) error {
	return r.db.Unscoped().Delete(&models.File{}, id).Error
}

// GetList 获取文件列表
func (r *FileRepository) GetList(userId string, fileType int, status int, page, pageSize int) ([]models.File, int64, error) {
	var files []models.File
	var total int64

	query := r.db.Model(&models.File{})

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

	if err := query.Order("created_at DESC").Limit(pageSize).Offset(offset).Find(&files).Error; err != nil {
		return nil, 0, err
	}

	return files, total, nil
}