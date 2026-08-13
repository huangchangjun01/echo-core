package service

import (
	"context"
	"echo-core/models"
	"echo-core/repository"
	"echo-core/utils"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// DownloadService 统一下载授权服务。
//
// 职责：
//   - 校验调用方对资源的访问权（按 resourceType 派发：file / memory_source）
//   - 生成 60s 短期 Qiniu 私有 URL + 应用层 HMAC(ip_sig)
//   - 落审计日志（best-effort）
//
// 改造范围：替换既有 GET /api/file/:id/download 直连代理为托管式下载授权。
// 旧代理保留作为兼容，handler 层加 Deprecated 注释。
type DownloadService struct {
	fileRepo    *repository.FileRepository
	recallRepo  *repository.RecallMemoryRepository
	auditRepo   *repository.AuditLogRepository
	fileService *FileService
}

// NewDownloadService 构造
func NewDownloadService() *DownloadService {
	return &DownloadService{
		fileRepo:    repository.NewFileRepository(),
		recallRepo:  repository.NewRecallMemoryRepository(),
		auditRepo:   repository.NewAuditLogRepository(),
		fileService: &FileService{}, // 仅复用 GetPrivateURL，不需要 repo；FileService{} 零值可用
	}
}

// AuthorizeDownload 统一下载授权入口。
//
// 参数：
//   - userId:       从 session 取
//   - clientIP:     c.ClientIP()
//   - resourceType: "file" | "memory_source" | "memory_md"
//   - resourceId:   file.id（resourceType=file 时必填）
//   - fileKey:      object storage key（resourceType=memory_source 时必填）
//   - memoryId:     recall_memory.memory_id（resourceType=memory_source 必填）
//
// 返回：
//   - signedURL:    带 ip_sig 的 Qiniu 私有 URL
//   - fileName:     浏览器下载时的展示名
//   - status:       ok / not_found / forbidden / invalid_request / internal_error
//   - err:          业务错误；调用方据此映射 HTTP 状态码
//
// 错误语义：
//   - 参数不全 / 类型不支持 → invalid_request（400）
//   - 资源不存在 / 归属错误 → not_found（404，不区分不存在与越权，避免泄露存在性）
//   - Qiniu 配置缺失 / 签名失败 → internal_error（500）
func (s *DownloadService) AuthorizeDownload(
	ctx context.Context,
	userId, clientIP, resourceType, resourceId, fileKey, memoryId string,
) (signedURL, fileName, status string, err error) {
	start := time.Now()
	status = models.AuditStatusOK
	defer func() {
		// 审计：best-effort 写入；写失败仅记日志，不影响业务返回
		s.writeAudit(ctx, userId, clientIP, resourceType, resourceId, fileKey, memoryId, status, time.Since(start).Milliseconds(), err)
	}()

	// 0. 公共前置校验
	if userId == "" {
		status = models.AuditStatusInvalidRequest
		err = errors.New("userId is required")
		return
	}
	if clientIP == "" {
		// ClientIP 拿不到时退化为空串（HMAC 仍可生成），但日志层面应警示
		utils.LogWithCtx(ctx, "DownloadService.AuthorizeDownload", "警告 clientIP 为空 | userId=%s resourceType=%s", userId, resourceType)
		clientIP = "unknown"
	}

	switch resourceType {
	case "file":
		signedURL, fileName, status, err = s.authorizeFile(ctx, userId, clientIP, resourceId)
	case "memory_source":
		signedURL, fileName, status, err = s.authorizeMemorySource(ctx, userId, clientIP, memoryId, fileKey)
	case "memory_md":
		signedURL, fileName, status, err = s.authorizeMemoryMd(ctx, userId, clientIP, memoryId)
	default:
		status = models.AuditStatusInvalidRequest
		err = fmt.Errorf("unsupported resourceType: %q", resourceType)
	}
	return
}

// authorizeFile 处理 resourceType=file 授权。
//
// 校验：file.UserId == userId && file.Status == 1
// 失败统一映射为 not_found（与既有 DownloadFile 行为一致）。
func (s *DownloadService) authorizeFile(ctx context.Context, userId, clientIP, resourceId string) (string, string, string, error) {
	if resourceId == "" {
		return "", "", models.AuditStatusInvalidRequest, errors.New("resourceId is required")
	}
	id64, parseErr := strconv.ParseUint(resourceId, 10, 64)
	if parseErr != nil || id64 == 0 {
		return "", "", models.AuditStatusInvalidRequest, errors.New("resourceId 非法的文件 ID")
	}
	id := uint(id64)

	file, err := s.fileRepo.GetByID(ctx, id)
	if err != nil || file.UserId != userId || file.Status != 1 {
		// 不区分"不存在"与"无权访问"，避免泄露他人文件 ID
		utils.LogWithCtx(ctx, "DownloadService.authorizeFile", "未授权 | id=%d userId=%s err=%v", id, userId, err)
		return "", "", models.AuditStatusNotFound, errors.New("文件不存在")
	}
	if file.Key == "" {
		// 纯文本记忆无 key：下载语义不适用，应走"纯文本下载"分支
		return "", "", models.AuditStatusInvalidRequest, errors.New("该文件无媒体内容，不可下载")
	}

	// 复用 FileService.GetPrivateURL（使用配置的 QINIU_DOMAIN），60s 过期
	privateURL, err := s.fileService.GetPrivateURL(ctx, file.Key, utils.DownloadExpirySeconds)
	if err != nil {
		utils.LogWithCtx(ctx, "DownloadService.authorizeFile", "生成私有 URL 失败 | id=%d err=%v", id, err)
		return "", "", models.AuditStatusInternalError, fmt.Errorf("生成下载链接失败: %w", err)
	}

	signedURL, err := s.signURL(ctx, privateURL, file.Key, clientIP)
	if err != nil {
		return "", "", models.AuditStatusInternalError, err
	}
	return signedURL, file.Name, models.AuditStatusOK, nil
}

// authorizeMemorySource 处理 resourceType=memory_source 授权。
//
// 校验：
//  1. memoryId 必须存在且归属当前用户
//  2. fileKey 必须是该记忆下的活跃源文件
//
// 任意一步失败统一映射为 not_found。
func (s *DownloadService) authorizeMemorySource(ctx context.Context, userId, clientIP, memoryId, fileKey string) (string, string, string, error) {
	if memoryId == "" || fileKey == "" {
		return "", "", models.AuditStatusInvalidRequest, errors.New("memoryId 与 fileKey 必填")
	}
	m, err := s.recallRepo.GetByMemoryId(ctx, memoryId)
	if err != nil || m.UserId != userId {
		utils.LogWithCtx(ctx, "DownloadService.authorizeMemorySource", "记忆归属校验失败 | memoryId=%s userId=%s err=%v", memoryId, userId, err)
		return "", "", models.AuditStatusNotFound, errors.New("记忆不存在")
	}
	// 二次校验：fileKey 必须真的在该记忆下
	keys, err := s.recallRepo.ListActiveSourceFileKeys(ctx, memoryId)
	if err != nil {
		return "", "", models.AuditStatusInternalError, fmt.Errorf("查询源文件失败: %w", err)
	}
	matched := false
	for _, k := range keys {
		if k == fileKey {
			matched = true
			break
		}
	}
	if !matched {
		utils.LogWithCtx(ctx, "DownloadService.authorizeMemorySource", "fileKey 不在该记忆下 | memoryId=%s fileKey=%s", memoryId, fileKey)
		return "", "", models.AuditStatusNotFound, errors.New("该源文件不属于此记忆")
	}

	// 查 fileName（用于浏览器下载展示名）
	var displayName string
	sources, err := s.recallRepo.ListSourceByMemoryId(ctx, memoryId)
	if err == nil {
		for _, sf := range sources {
			if sf.FileKey == fileKey {
				displayName = sf.FileName
				break
			}
		}
	}
	if displayName == "" {
		// 兜底：fileKey 最后一段
		displayName = trimLastSegment(fileKey)
	}

	privateURL, err := s.fileService.GetPrivateURL(ctx, fileKey, utils.DownloadExpirySeconds)
	if err != nil {
		utils.LogWithCtx(ctx, "DownloadService.authorizeMemorySource", "生成私有 URL 失败 | memoryId=%s fileKey=%s err=%v", memoryId, fileKey, err)
		return "", "", models.AuditStatusInternalError, fmt.Errorf("生成下载链接失败: %w", err)
	}
	signedURL, err := s.signURL(ctx, privateURL, fileKey, clientIP)
	if err != nil {
		return "", "", models.AuditStatusInternalError, err
	}
	return signedURL, displayName, models.AuditStatusOK, nil
}

// authorizeMemoryMd 处理 resourceType=memory_md 授权。
//
// md 文本已经在 DB 缓存（recall_memory.md_content），不走 CDN。
// 鉴权后返回一段"代理下载 URL"：`/api/memory/{memoryId}/md-file?ticket=<HMAC>`。
// ticket 是 HMAC(memoryId|deadline|ip)，由独立 handler 校验后流式返回 md 内容
// 并设置 Content-Disposition: attachment 触发浏览器下载。
//
// 与 file/memory_source 走 CDN 的差异：
//   - file/memory_source: 浏览器直连 Qiniu CDN（快、可断点续传）
//   - memory_md:           浏览器走自家后端代理（必须，因为 md 不在对象存储）
//   - 两者都在 audit_log 落一条 ok，且 ticket 60s 过期 + IP 锁
func (s *DownloadService) authorizeMemoryMd(ctx context.Context, userId, clientIP, memoryId string) (string, string, string, error) {
	if memoryId == "" {
		return "", "", models.AuditStatusInvalidRequest, errors.New("memoryId is required")
	}
	m, err := s.recallRepo.GetByMemoryId(ctx, memoryId)
	if err != nil || m.UserId != userId {
		utils.LogWithCtx(ctx, "DownloadService.authorizeMemoryMd", "记忆归属校验失败 | memoryId=%s userId=%s err=%v", memoryId, userId, err)
		return "", "", models.AuditStatusNotFound, errors.New("记忆不存在")
	}
	if m.MdContent == "" {
		return "", "", models.AuditStatusInvalidRequest, errors.New("该记忆暂未缓存 md 内容")
	}

	// 拼 ticket：HMAC(memoryId|deadline|ip)，deadline 与 ip_sig 共用同一签名工具
	deadline := utils.NowDeadline()
	secret, secErr := utils.DownloadSignSecret()
	if secErr != nil {
		utils.LogWithCtx(ctx, "DownloadService.authorizeMemoryMd", "签名密钥未配置 | err=%v", secErr)
		return "", "", models.AuditStatusInternalError, secErr
	}
	ticket := utils.MakeDownloadSign(secret, memoryId, clientIP, deadline)
	proxyURL := fmt.Sprintf("/api/memory/%s/md-file?ticket=%s&e=%d", memoryId, ticket, deadline)
	utils.LogWithCtx(ctx, "DownloadService.authorizeMemoryMd",
		"md 下载 ticket 签发成功 | memoryId=%s deadline=%d ticketLen=%d", memoryId, deadline, len(ticket))
	fileName := memoryId + ".md"
	return proxyURL, fileName, models.AuditStatusOK, nil
}

// signURL 把七牛私有 URL 追加 ip_sig=<hmac>。
//
// 计算：HMAC-SHA256( key | deadline | ip )
// deadline 取自 privateURL 中的 ?e= 参数，保证 HMAC 与 Qiniu 的过期时间一致。
func (s *DownloadService) signURL(ctx context.Context, privateURL, key, clientIP string) (string, error) {
	deadline, err := parseDeadlineFromQiniuURL(privateURL)
	if err != nil {
		utils.LogWithCtx(ctx, "DownloadService.signURL", "解析 deadline 失败 | err=%v url=%s", err, truncateURL(privateURL))
		return "", fmt.Errorf("解析过期时间失败: %w", err)
	}

	secret, err := utils.DownloadSignSecret()
	if err != nil {
		utils.LogWithCtx(ctx, "DownloadService.signURL", "签名密钥未配置 | err=%v", err)
		return "", err
	}
	sig := utils.MakeDownloadSign(secret, key, clientIP, deadline)
	signedURL, err := utils.AppendIPSignature(privateURL, sig)
	if err != nil {
		utils.LogWithCtx(ctx, "DownloadService.signURL", "URL 拼接失败 | err=%v", err)
		return "", err
	}
	return signedURL, nil
}

// writeAudit 写入一条审计日志（best-effort，失败仅记日志不影响业务返回）。
func (s *DownloadService) writeAudit(
	ctx context.Context, userId, clientIP, resourceType, resourceId, fileKey, memoryId,
	status string, latencyMs int64, bizErr error,
) {
	targetID := buildTargetID(resourceType, resourceId, fileKey, memoryId)
	entry := &models.AuditLog{
		UserId:     userId,
		Action:     models.AuditActionDownloadAuthorize,
		TargetType: resourceType,
		TargetId:   targetID,
		Ip:         clientIP,
		Status:     status,
		LatencyMs:  int(latencyMs),
	}
	if err := s.auditRepo.Create(ctx, entry); err != nil {
		utils.LogWithCtx(ctx, "DownloadService.writeAudit", "审计写入失败 | target=%s status=%s err=%v bizErr=%v",
			targetID, status, err, bizErr)
	}
}

// buildTargetID 构造审计 target_id。
//   - file:          <resourceId>
//   - memory_source: <memoryId>:<fileKey>
//   - memory_md:     <memoryId>
//   - 其他:          <resourceType>:<resourceId>
func buildTargetID(resourceType, resourceId, fileKey, memoryId string) string {
	switch resourceType {
	case "file":
		return resourceId
	case "memory_source":
		if memoryId != "" && fileKey != "" {
			return memoryId + ":" + fileKey
		}
	case "memory_md":
		if memoryId != "" {
			return memoryId
		}
	}
	return resourceType + ":" + resourceId
}

// trimLastSegment 取 fileKey 末段作为下载展示名兜底。
func trimLastSegment(key string) string {
	idx := strings.LastIndex(key, "/")
	if idx < 0 || idx == len(key)-1 {
		return key
	}
	return key[idx+1:]
}

// truncateURL 截断 URL 用于日志（避免完整 URL 落日志泄露）。
func truncateURL(u string) string {
	const max = 80
	if len(u) <= max {
		return u
	}
	return u[:max] + "..."
}
