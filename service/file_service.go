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
//
// Domain 是该 bucket 对外的公开访问域名（来自 QINIU_DOMAIN 环境变量），
// 前端拿到后可拼出可访问的文件 URL：`<domain>/<key>`，用于列表预览/缩略图。
// UploadURL 是七牛上传接口地址（前端实际上传直接走 qiniu-js，不依赖此字段），
// 仅做透传以备排查。
type GetUploadTokenResult struct {
	Token     string `json:"token"`
	UploadURL string `json:"uploadURL"`
	Key       string `json:"key"`
	Domain    string `json:"domain"`
}

// RegisterFileResult 注册文件结果
type RegisterFileResult struct {
	ID     uint   `json:"id"`
	UserId string `json:"userId"`
	Key    string `json:"key"`
	Status int    `json:"status"`
	// URL 是文件可访问的公开链接（GetPublicURL 拼出）。
	// 前端在上传成功页需要用这个 URL 渲染缩略图/预览。
	// 注册失败的资源（如纯文本记忆，Key 为空）会省略该字段。
	URL string `json:"url,omitempty"`
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

	accessKey, secretKey, bucket, domain, err := getQiniuConfig()
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

	utils.LogWithCtx(ctx, "FileService.GetUploadToken", "token 生成成功 | key=%s domain=%s tokenLen=%d", key, domain, len(upToken))
	return &GetUploadTokenResult{
		Token:     upToken,
		UploadURL: "https://upload.qiniup.com",
		Key:       key,
		Domain:    domain,
	}, nil
}

// RegisterFile 登记文件元数据
//
// userId 由 handler 层从已鉴权的 session 取（middleware.MustUserID(c)），
// 不再从请求体取 —— 防止前端伪造他人 userId，也避免 authStore 未就绪时
// userId 为空串触发 binding 校验失败。
//
// 当前实现：仅在 DB 中创建记录，不触发任何 RAG 入库。
// 事务保证：插入失败即整体回滚。
func (h *FileService) RegisterFile(ctx context.Context, userId string, req *request.RegisterFileRequest) (*RegisterFileResult, error) {
	if userId == "" {
		return nil, errors.New("userId is required")
	}
	utils.LogWithCtx(ctx, "FileService.RegisterFile", "收到请求 | userId=%s fileName=%s key=%s fileType=%d bizType=%d roleId=%s descLen=%d",
		userId, req.FileName, req.Key, req.FileType, req.BizType, req.RoleId, len(req.Desc))

	tx := config.DB.Begin()
	if tx.Error != nil {
		utils.LogWithCtx(ctx, "FileService.RegisterFile", "开启事务失败 | err=%v", tx.Error)
		return nil, fmt.Errorf("开启事务失败: %v", tx.Error)
	}

	roleId := req.RoleId
	if roleId == "" {
		roleId = "default"
	}
	file := models.File{
		Name:     req.FileName,
		UserId:   userId,
		Key:      req.Key,
		FileType: req.FileType,
		BizType:  req.BizType,
		Status:   1,
		Desc:     req.Desc,
		RoleId:   roleId,
		Config:   &models.FileConfig{},
	}

	if err := h.fileRepo.CreateWithTx(ctx, tx, &file); err != nil {
		tx.Rollback()
		utils.LogWithCtx(ctx, "FileService.RegisterFile", "文件入库失败，回滚事务 | err=%v", err)
		return nil, fmt.Errorf("文件入库失败: %v", err)
	}
	utils.LogWithCtx(ctx, "FileService.RegisterFile", "文件入库成功 | id=%d key=%s roleId=%s", file.Id, file.Key, file.RoleId)

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

	// 把可访问 URL 透出给前端：仅在 key 非空时尝试生成。
	// 纯文本记忆（key="")或七牛配置缺失时留空，前端会回退到占位图标。
	if file.Key != "" {
		if u, err := h.GetPublicURL(ctx, file.Key); err == nil {
			result.URL = u
		} else {
			utils.LogWithCtx(ctx, "FileService.RegisterFile", "生成可访问 URL 失败 | key=%s err=%v", file.Key, err)
		}
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
//
// 关键边界：
//   - key 为空（如纯文本记忆）→ 不生成 URL，让 Python 走"desc 分支"，避免空 key 拼出畸形 URL 去下载 404
//   - key 非空但 Qiniu 配置缺失 → 同样只透传 desc，由 Python 自行降级
func (h *FileService) triggerIngest(ctx context.Context, file *models.File) *dto.IngestFileResponse {
	utils.LogWithCtx(ctx, "FileService.triggerIngest", "开始触发入库 | id=%d key=%s roleId=%s descLen=%d",
		file.Id, file.Key, file.RoleId, len(file.Desc))

	var url string
	if file.Key != "" {
		u, err := h.GetPrivateURL(ctx, file.Key, 24*3600)
		if err != nil {
			// 配置不全时仅记日志，依然透传 fileKey，让 Python 端兜底
			utils.LogWithCtx(ctx, "FileService.triggerIngest", "生成可访问 URL 失败（仅记日志） | id=%d key=%s err=%v", file.Id, file.Key, err)
		} else {
			url = u
		}
	}
	resp, err := h.ingestClient.IngestFile(ctx, dto.IngestFileRequest{
		UserID: file.UserId,
		Desc:   file.Desc,
		RoleID: file.RoleId,
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

// MemoryFileItem 记忆管理页单条文件项
type MemoryFileItem struct {
	ID        uint      `json:"id"`
	UserID    string    `json:"userId"`
	RoleID    string    `json:"roleId"`
	FileType  int       `json:"fileType"`
	FileName  string    `json:"fileName"`
	Key       string    `json:"key"`
	URL       string    `json:"url,omitempty"`
	Desc      string    `json:"desc"`
	Status    int       `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// ListMemoryFiles 记忆管理：按 (userId, roleId?, fileType?) 拉取当前用户的文件。
// roleId="" 表示不过滤（前端未选角色时的兜底）。
func (h *FileService) ListMemoryFiles(ctx context.Context, userId, roleId string, fileType int) ([]MemoryFileItem, error) {
	utils.LogWithCtx(ctx, "FileService.ListMemoryFiles", "入参 | userId=%s roleId=%s fileType=%d", userId, roleId, fileType)
	if userId == "" {
		return nil, errors.New("userId is required")
	}
	files, _, err := h.fileRepo.GetList(ctx, userId, fileType, 1, 0, 200)
	if err != nil {
		utils.LogWithCtx(ctx, "FileService.ListMemoryFiles", "查询失败 | err=%v", err)
		return nil, err
	}
	out := make([]MemoryFileItem, 0, len(files))
	for i := range files {
		f := files[i]
		if roleId != "" && f.RoleId != roleId {
			continue
		}
		var url string
		if f.Key != "" {
			if u, err := h.GetPublicURL(ctx, f.Key); err == nil {
				url = u
			}
		}
		out = append(out, MemoryFileItem{
			ID:        f.Id,
			UserID:    f.UserId,
			RoleID:    f.RoleId,
			FileType:  f.FileType,
			FileName:  f.Name,
			Key:       f.Key,
			URL:       url,
			Desc:      f.Desc,
			Status:    f.Status,
			CreatedAt: f.CreatedAt,
			UpdatedAt: f.UpdatedAt,
		})
	}
	utils.LogWithCtx(ctx, "FileService.ListMemoryFiles", "完成 | count=%d", len(out))
	return out, nil
}

// UpdateDesc 修改文件描述，触发重新入库生成记忆
func (h *FileService) UpdateDesc(ctx context.Context, userId string, id uint, desc string) error {
	utils.LogWithCtx(ctx, "FileService.UpdateDesc", "入参 | userId=%s id=%d descLen=%d", userId, id, len(desc))
	if userId == "" {
		return errors.New("userId is required")
	}
	file, err := h.fileRepo.GetByID(ctx, id)
	if err != nil {
		return errors.New("文件不存在")
	}
	if file.UserId != userId {
		return errors.New("文件不存在")
	}
	if err := h.fileRepo.Update(ctx, id, map[string]interface{}{"desc": desc}); err != nil {
		return err
	}
	file.Desc = desc
	if ingestion := h.triggerIngest(ctx, file); ingestion == nil {
		utils.LogWithCtx(ctx, "FileService.UpdateDesc", "Python 重新入库转发失败（不影响描述落库） | id=%d", id)
	}
	utils.LogWithCtx(ctx, "FileService.UpdateDesc", "成功 | id=%d", id)
	return nil
}

// CreateTextMemory 新建纯文本记忆：直接以 desc 为文本入 RAG 库 + 生成记忆。
// file_type=1（文本），无 key/url，调用 ingest 时 Python 会走「纯文本记忆」分支。
func (h *FileService) CreateTextMemory(ctx context.Context, userId, roleId, desc string) (*MemoryFileItem, error) {
	utils.LogWithCtx(ctx, "FileService.CreateTextMemory", "入参 | userId=%s roleId=%s descLen=%d", userId, roleId, len(desc))
	if userId == "" {
		return nil, errors.New("userId is required")
	}
	if roleId == "" {
		roleId = "default"
	}
	tx := config.DB.Begin()
	if tx.Error != nil {
		return nil, fmt.Errorf("开启事务失败: %v", tx.Error)
	}
	file := models.File{
		Name:     truncateForFilename(desc, 32),
		UserId:   userId,
		Key:      "",
		FileType: 1, // 文本
		BizType:  0,
		Status:   1,
		Desc:     desc,
		RoleId:   roleId,
		Config:   &models.FileConfig{},
	}
	if err := h.fileRepo.CreateWithTx(ctx, tx, &file); err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("文件入库失败: %v", err)
	}
	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("提交事务失败: %v", err)
	}
	utils.LogWithCtx(ctx, "FileService.CreateTextMemory", "文件入库成功 | id=%d userId=%s roleId=%s", file.Id, file.UserId, file.RoleId)
	if ingestion := h.triggerIngest(ctx, &file); ingestion != nil {
		utils.LogWithCtx(ctx, "FileService.CreateTextMemory", "Python /ingest_file 转发成功 | id=%d queued=%v", file.Id, ingestion.Queued)
	}
	return &MemoryFileItem{
		ID:        file.Id,
		UserID:    file.UserId,
		RoleID:    file.RoleId,
		FileType:  file.FileType,
		FileName:  file.Name,
		Key:       file.Key,
		Desc:      file.Desc,
		Status:    file.Status,
		CreatedAt: file.CreatedAt,
		UpdatedAt: file.UpdatedAt,
	}, nil
}

// truncateForFilename 把文本描述截断成文件名（按字符数，不按字节；中文也按 N 个字符）
func truncateForFilename(s string, n int) string {
	if n <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}
