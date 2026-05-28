package service

import (
	"echo-core/config"
	"echo-core/models"
	"echo-core/remote"
	remoteRequest "echo-core/remote/request"
	"echo-core/repository"
	serviceRequest "echo-core/service/request"
	"echo-core/utils"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/qiniu/go-sdk/v7/auth"
	"github.com/qiniu/go-sdk/v7/storage"
	"log"
	"path"
	"time"
)

type FileService struct {
	fileRepository *repository.FileRepository
	vectorRemote    *remote.VectorRemote
}

// GetUploadTokenResult 获取上传token的结果
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
}

func NewFileService() (*FileService, error) {
	return &FileService{
		fileRepository: repository.NewFileRepository(),
		vectorRemote:    remote.NewVectorRemote(),
	}, nil
}

// getQiniuConfig 获取七牛云配置
func getQiniuConfig() (accessKey, secretKey, bucket, domain string, err error) {
	accessKey = utils.GetEnv("QINIU_ACCESS_KEY", "")
	secretKey = utils.GetEnv("QINIU_SECRET_KEY", "")
	bucket = utils.GetEnv("QINIU_BUCKET_NAME", "")
	domain = utils.GetEnv("QINIU_DOMAIN", "")
	if accessKey == "" || secretKey == "" || bucket == "" || domain == "" {
		return "", "", "", "", errors.New("七牛云配置不完整，请检查 QINIU_ACCESS_KEY、QINIU_SECRET_KEY、QINIU_BUCKET_NAME 和 QINIU_DOMAIN")
	}
	return accessKey, secretKey, bucket, domain, nil
}

// GetPrivateURL 生成七牛云私有空间临时访问链接
func (h *FileService) GetPrivateURL(key string, expiresInSeconds int64) (string, error) {
	// 获取七牛云配置
	accessKey, secretKey, _, domain, err := getQiniuConfig()
	if err != nil {
		log.Println(err)
		return "", err
	}

	mac := auth.New(accessKey, secretKey)

	deadline := time.Now().Add(time.Duration(expiresInSeconds) * time.Second).Unix()
	privateURL := storage.MakePrivateURL(mac, domain, key, deadline)
	log.Println("七牛云文件url获取成功：", privateURL)
	return privateURL, nil
}

// GetPublicURL 生成七牛云公开空间访问链接
func (h *FileService) GetPublicURL(key string) (string, error) {
	// 获取七牛云配置
	_, _, _, domain, err := getQiniuConfig()
	if err != nil {
		log.Println(err)
		return "", err
	}

	publicURL := storage.MakePublicURL(domain, key)
	log.Println("七牛云文件url获取成功：", publicURL)
	return publicURL, nil
}

// GetUploadToken 获取七牛云上传token（带重试机制）
func (h *FileService) GetUploadToken(fileName string, fileSize int64, mimeType string, bizType string) (*GetUploadTokenResult, error) {
	accessKey, secretKey, bucket, _, err := getQiniuConfig()
	if err != nil {
		log.Printf("[GetUploadToken] 配置检查失败: %v", err)
		return nil, err
	}

	mac := auth.New(accessKey, secretKey)
	putPolicy := storage.PutPolicy{
		Scope: bucket,
	}

	// 设置 token 过期时间（1小时）
	putPolicy.Expires = 3600

	// 生成唯一文件名
	key := fmt.Sprintf("%s/%s/%s%s", bizType, time.Now().Format("20060102"), uuid.New().String(), path.Ext(fileName))

	putPolicy.InsertOnly = 1
	upToken := putPolicy.UploadToken(mac)

	const maxRetries = 3
	var lastErr error

	for i := 1; i <= maxRetries; i++ {
		log.Printf("[GetUploadToken] 第 %d 次尝试获取token, bizType=%s, fileName=%s, fileSize=%d, mimeType=%s",
			i, bizType, fileName, fileSize, mimeType)

		// 验证token是否生成成功
		if upToken == "" {
			lastErr = errors.New("生成的token为空")
			log.Printf("[GetUploadToken] 第 %d 次尝试失败: %v", i, lastErr)
			continue
		}

		log.Printf("[GetUploadToken] 第 %d 次尝试成功, key=%s, token长度=%d", i, key, len(upToken))
		return &GetUploadTokenResult{
			Token:     upToken,
			UploadURL: "https://upload.qiniup.com",
			Key:       key,
		}, nil
	}

	log.Printf("[GetUploadToken] 所有 %d 次重试均失败: %v", maxRetries, lastErr)
	return nil, fmt.Errorf("获取七牛云token失败，已重试 %d 次，最后一次错误: %v", maxRetries, lastErr)
}

// RegisterFile 注册文件信息（带事务）
func (h *FileService) RegisterFile(req *serviceRequest.RegisterFileRequest) (*RegisterFileResult, error) {
	log.Printf("[RegisterFile] 收到请求: userId=%s, fileName=%s, key=%s, fileType=%d, bizType=%d",
		req.UserId, req.FileName, req.Key, req.FileType, req.BizType)

	// 开启事务
	tx := config.DB.Begin()
	if tx.Error != nil {
		log.Printf("[RegisterFile] 开启事务失败: %v", tx.Error)
		return nil, fmt.Errorf("开启事务失败: %v", tx.Error)
	}

	// 创建文件记录
	file := models.File{
		Name:     req.FileName,
		UserId:   req.UserId,
		Key:      req.Key,
		FileType: req.FileType,
		BizType:  req.BizType,
		Status:   1,                    // 默认可用
		Config:   &models.FileConfig{}, // 默认空JSON对象
	}

	// 使用 FileRepository 创建记录
	if err := h.fileRepository.CreateWithTx(tx, &file); err != nil {
		tx.Rollback()
		log.Printf("[RegisterFile] 文件入库失败，回滚事务: %v", err)
		return nil, fmt.Errorf("文件入库失败: %v", err)
	}
	log.Printf("[RegisterFile] 文件入库成功: id=%d, key=%s", file.Id, file.Key)

	_, _, _, domain, err := getQiniuConfig()
	if err != nil {
		log.Printf("[RegisterFile] 配置检查失败: %v", err)
		return nil, err
	}
	// 调用Python服务 /ingest_file 接口
	ingestReq := &remoteRequest.IngestFileRequest{
		UserID: req.UserId,
	}
	ingestReq.File.FileID = fmt.Sprintf("%d", file.Id)
	ingestReq.File.FileName = req.FileName
	ingestReq.File.FileKey = req.Key
	ingestReq.File.Url = domain + "/" + req.Key
	log.Printf("[RegisterFile] 开始调用Python /ingest_file 接口: userId=%s, fileId=%s, fileName=%s, FileKey=%s, Url=%s",
		ingestReq.UserID, ingestReq.File.FileID, ingestReq.File.FileName, ingestReq.File.FileKey, ingestReq.File.Url)

	if err := h.vectorRemote.IngestFile(ingestReq); err != nil {
		tx.Rollback()
		log.Printf("[RegisterFile] 调用Python /ingest_file 接口失败，回滚事务: %v", err)
		return nil, fmt.Errorf("调用Python服务失败: %v", err)
	}

	// 提交事务
	if err := tx.Commit().Error; err != nil {
		log.Printf("[RegisterFile] 提交事务失败: %v", err)
		return nil, fmt.Errorf("提交事务失败: %v", err)
	}

	log.Printf("[RegisterFile] 注册文件成功: id=%d, userId=%s, key=%s", file.Id, file.UserId, file.Key)
	return &RegisterFileResult{
		ID:     file.Id,
		UserId: file.UserId,
		Key:    file.Key,
		Status: file.Status,
	}, nil
}
