package service

import (
	"context"
	"echo-core/config"
	"echo-core/dto"
	"echo-core/models"
	"echo-core/remote"
	"echo-core/repository"
	"echo-core/service/request"
	"echo-core/utils"
	"errors"
	"fmt"
	"path"
	"time"

	"github.com/google/uuid"
	"github.com/qiniu/go-sdk/v7/auth"
	"github.com/qiniu/go-sdk/v7/storage"
)

// FileService 文件存储服务（七牛云）+ RAG 入库触发
// 职责：
//  1. 七牛云上传 token 生成
//  2. 文件元数据落 MySQL
//  3. 文件登记成功后，转调 Python /ingest_file 触发 RAG 入库
//
// 入库失败仅记日志：RAG 是异步链路，不应阻塞主注册流程。
type FileService struct {
	fileRepo     *repository.FileRepository
	ingestClient *remote.PythonIngestClient
}

// GetUploadTokenResult 获取上传 token 的结果
type GetUploadTokenResult struct {
	Token     string `json:"token"`
	UploadURL string `json:"uploadURL"`
	Key       string `json:"key"`
}

// RegisterFileResult 注册文件结果
type RegisterFileResult struct {
	ID     uint   `json:"id"`
	UserId string `json:"userId"`
	Key    string `json:"key"`
	Status int    `json:"status"`
	// Ingestion 字段为非空时表示已成功转发 Python /ingest_file
	Ingestion *dto.IngestFileResponse `json:"ingestion,omitempty"`
}

// NewFileService 构造 FileService
func NewFileService() (*FileService, error) {
	return &FileService{
		fileRepo:     repository.NewFileRepository(),
		ingestClient: remote.NewPythonIngestClient(),
	}, nil
}

// getQiniuConfig 读取七牛云配置（任一缺失即报错）
func getQiniuConfig() (accessKey, secretKey, bucket, domain string, err error) {
	accessKey = utils.GetEnv("QINIU_ACCESS_KEY", "")
	secretKey = utils.GetEnv("QINIU_SECRET_KEY", "")
	bucket = utils.GetEnv("QINIU_BUCKET_NAME", "")
	domain = utils.GetEnv("QINIU_DOMAIN", "")
	if accessKey == "" || secretKey == "" || bucket == "" || domain == "" {
		missing := []string{}
		if accessKey == "" {
			missing = append(missing, "QINIU_ACCESS_KEY")
		}
		if secretKey == "" {
			missing = append(missing, "QINIU_SECRET_KEY")
		}
		if bucket == "" {
			missing = append(missing, "QINIU_BUCKET_NAME")
		}
		if domain == "" {
			missing = append(missing, "QINIU_DOMAIN")
		}
		return "", "", "", "", fmt.Errorf("七牛云配置不完整，缺失: %v", missing)
	}
	return accessKey, secretKey, bucket, domain, nil
}

// GetPrivateURL 生成七牛云私有空间临时访问链接
func (h *FileService) GetPrivateURL(ctx context.Context, key string, expiresInSeconds int64) (string, error) {
	accessKey, secretKey, _, domain, err := getQiniuConfig()
	if err != nil {
		utils.LogWithCtx(ctx, "FileService.GetPrivateURL", "配置检查失败 | err=%v", err)
		return "", err
	}

	mac := auth.New(accessKey, secretKey)
	deadline := time.Now().Add(time.Duration(expiresInSeconds) * time.Second).Unix()
	privateURL := storage.MakePrivateURL(mac, domain, key, deadline)
	utils.LogWithCtx(ctx, "FileService.GetPrivateURL", "七牛云文件url生成成功 | key=%s expireIn=%ds", key, expiresInSeconds)
	return privateURL, nil
}

// GetPublicURL 生成七牛云公开空间访问链接
func (h *FileService) GetPublicURL(ctx context.Context, key string) (string, error) {
	_, _, _, domain, err := getQiniuConfig()
	if err != nil {
		utils.LogWithCtx(ctx, "FileService.GetPublicURL", "配置检查失败 | err=%v", err)
		return "", err
	}

	publicURL := storage.MakePublicURL(domain, key)
	utils.LogWithCtx(ctx, "FileService.GetPublicURL", "七牛云文件url生成成功 | key=%s url=%s", key, publicURL)
	return publicURL, nil
}

// GetUploadToken 获取七牛云上传 token（带重试）
func (h *FileService) GetUploadToken(ctx context.Context, fileName string, fileSize int64, mimeType string, bizType string) (*GetUploadTokenResult, error) {
	utils.LogWithCtx(ctx, "FileService.GetUploadToken", "收到请求 | fileName=%s fileSize=%d mimeType=%s bizType=%s",
		fileName, fileSize, mimeType, bizType)

	accessKey, secretKey, bucket, _, err := getQiniuConfig()
	if err != nil {
		utils.LogWithCtx(ctx, "FileService.GetUploadToken", "配置检查失败 | err=%v", err)
		return nil, err
	}

	mac := auth.New(accessKey, secretKey)
	putPolicy := storage.PutPolicy{
		Scope:      bucket,
		Expires:    3600, // token 1 小时有效
		InsertOnly: 1,
	}

	// 生成唯一文件名: bizType/YYYYMMDD/uuid.ext
	key := fmt.Sprintf("%s/%s/%s%s", bizType, time.Now().Format("20060102"), uuid.New().String(), path.Ext(fileName))

	upToken := putPolicy.UploadToken(mac)
	if upToken == "" {
		utils.LogWithCtx(ctx, "FileService.GetUploadToken", "token 生成结果为空 | key=%s", key)
		return nil, errors.New("生成七牛云上传 token 失败")
	}

	utils.LogWithCtx(ctx, "FileService.GetUploadToken", "token 生成成功 | key=%s tokenLen=%d", key, len(upToken))
	return &GetUploadTokenResult{
		Token:     upToken,
		UploadURL: "https://upload.qiniup.com",
		Key:       key,
	}, nil
}

// RegisterFile 登记文件元数据
// 当前实现：仅在 DB 中创建记录，不触发任何 RAG 入库。
// 事务保证：插入失败即整体回滚。
func (h *FileService) RegisterFile(ctx context.Context, req *request.RegisterFileRequest) (*RegisterFileResult, error) {
	utils.LogWithCtx(ctx, "FileService.RegisterFile", "收到请求 | userId=%s fileName=%s key=%s fileType=%d bizType=%d",
		req.UserId, req.FileName, req.Key, req.FileType, req.BizType)

	tx := config.DB.Begin()
	if tx.Error != nil {
		utils.LogWithCtx(ctx, "FileService.RegisterFile", "开启事务失败 | err=%v", tx.Error)
		return nil, fmt.Errorf("开启事务失败: %v", tx.Error)
	}

	file := models.File{
		Name:     req.FileName,
		UserId:   req.UserId,
		Key:      req.Key,
		FileType: req.FileType,
		BizType:  req.BizType,
		Status:   1,
		Config:   &models.FileConfig{},
	}

	if err := h.fileRepo.CreateWithTx(ctx, tx, &file); err != nil {
		tx.Rollback()
		utils.LogWithCtx(ctx, "FileService.RegisterFile", "文件入库失败，回滚事务 | err=%v", err)
		return nil, fmt.Errorf("文件入库失败: %v", err)
	}
	utils.LogWithCtx(ctx, "FileService.RegisterFile", "文件入库成功 | id=%d key=%s", file.Id, file.Key)

	if err := tx.Commit().Error; err != nil {
		utils.LogWithCtx(ctx, "FileService.RegisterFile", "提交事务失败 | err=%v", err)
		return nil, fmt.Errorf("提交事务失败: %v", err)
	}

	utils.LogWithCtx(ctx, "FileService.RegisterFile", "注册文件成功 | id=%d userId=%s key=%s", file.Id, file.UserId, file.Key)

	result := &RegisterFileResult{
		ID:     file.Id,
		UserId: file.UserId,
		Key:    file.Key,
		Status: file.Status,
	}

	// 入库成功后，异步转发 Python /ingest_file 触发 RAG 入库。
	// 失败仅记日志：RAG 链路是异步的，失败可后续对账重试，不应影响主注册返回。
	if ingestion := h.triggerIngest(ctx, &file); ingestion != nil {
		result.Ingestion = ingestion
	}
	return result, nil
}

// triggerIngest 调用 Python /ingest_file 把已登记文件送入 RAG 知识库。
// 返回值：成功 → *IngestFileResponse；失败 → nil（仅记日志，不影响上游）。
func (h *FileService) triggerIngest(ctx context.Context, file *models.File) *dto.IngestFileResponse {
	utils.LogWithCtx(ctx, "FileService.triggerIngest", "开始触发入库 | id=%d key=%s", file.Id, file.Key)
	url, err := h.GetPrivateURL(ctx, file.Key, 24*3600)
	if err != nil {
		utils.LogWithCtx(ctx, "FileService.triggerIngest", "生成可访问 URL 失败 | id=%d key=%s err=%v", file.Id, file.Key, err)
		return nil
	}
	resp, err := h.ingestClient.IngestFile(ctx, dto.IngestFileRequest{
		UserID: file.UserId,
		File: dto.IngestFileInfo{
			FileID:   fmt.Sprintf("f-%d", file.Id),
			FileName: file.Name,
			FileKey:  file.Key,
			URL:      url,
		},
	})
	if err != nil {
		utils.LogWithCtx(ctx, "FileService.triggerIngest", "Python /ingest_file 调用失败 | id=%d key=%s err=%v", file.Id, file.Key, err)
		return nil
	}
	utils.LogWithCtx(ctx, "FileService.triggerIngest", "Python /ingest_file 转发成功 | id=%d ok=%v queued=%v", file.Id, resp.OK, resp.Queued)
	return resp
}
