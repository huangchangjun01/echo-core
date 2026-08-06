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
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/qiniu/go-sdk/v7/auth"
	"github.com/qiniu/go-sdk/v7/storage"
	"gorm.io/gorm"
)

// 业务错误
var (
	ErrTopicExists    = errors.New("记忆主题已存在")
	ErrMemoryNotFound = errors.New("记忆不存在")
)

// RecallMemoryService 回忆记忆（文档即记忆）业务编排。
//
// 职责：
//  1. 申请 memoryId / 主题唯一性校验 / 记忆内上传 token
//  2. 记忆主题 + 源文件落库（事务）
//  3. 落库成功后异步转调 echo-ai /memory/* 完成解析/更新/删除
//  4. 删除时同步清理对象存储（源文件 / 整目录）
//
// AI 链路失败仅记日志，不阻塞主流程（与 FileService.triggerIngest 一致）。
type RecallMemoryService struct {
	repo      *repository.RecallMemoryRepository
	fileSvc   *FileService
	memClient *remote.PythonMemoryClient
}

// NewRecallMemoryService 构造
func NewRecallMemoryService() (*RecallMemoryService, error) {
	fileSvc, err := NewFileService()
	if err != nil {
		return nil, err
	}
	return &RecallMemoryService{
		repo:      repository.NewRecallMemoryRepository(),
		fileSvc:   fileSvc,
		memClient: remote.NewPythonMemoryClient(),
	}, nil
}

// newRecallMemoryServiceForTest 注入依赖的测试构造（仅 *_test.go 使用）。
//
// 用途：让 triggerParseAsync 的状态推进/重试/FAILED 路径可在不连真实 DB /
// echo-ai 的情况下被单元测试覆盖。
func newRecallMemoryServiceForTest(
	repo *repository.RecallMemoryRepository,
	fileSvc *FileService,
	memClient *remote.PythonMemoryClient,
) *RecallMemoryService {
	return &RecallMemoryService{
		repo:      repo,
		fileSvc:   fileSvc,
		memClient: memClient,
	}
}

// normalizeRoleId 统一空角色为 default
func normalizeRoleId(roleId string) string {
	if strings.TrimSpace(roleId) == "" {
		return "default"
	}
	return roleId
}

// memoryDir 记忆目录：memory/{userId}/{roleId}/{memoryId}/
func memoryDir(userId, roleId, memoryId string) string {
	return fmt.Sprintf("memory/%s/%s/%s/", userId, roleId, memoryId)
}

// memoryMdKey {memoryId}.md 的 key（路径确定，可提前计算）
func memoryMdKey(userId, roleId, memoryId string) string {
	return memoryDir(userId, roleId, memoryId) + memoryId + ".md"
}

// ===== 申请记忆 =====

// ApplyMemoryId 生成去横线的 UUID 作为 memoryId
func (s *RecallMemoryService) ApplyMemoryId(ctx context.Context) string {
	id := strings.ReplaceAll(uuid.New().String(), "-", "")
	utils.LogWithCtx(ctx, "RecallMemoryService.ApplyMemoryId", "生成 memoryId=%s", id)
	return id
}

// CheckTopicExists 校验主题唯一性
func (s *RecallMemoryService) CheckTopicExists(ctx context.Context, userId, roleId, topic string) (bool, error) {
	if userId == "" {
		return false, errors.New("userId is required")
	}
	return s.repo.ExistsTopic(ctx, userId, normalizeRoleId(roleId), topic)
}

// ===== 记忆内上传 token =====

// GetMemoryUploadToken 生成"记忆目录内"的上传 token。
//
// 与通用 GetUploadToken 不同：key 强制落在 memory/{userId}/{roleId}/{memoryId}/ 目录下，
// userId 由 session 提供，保证目录归属可信。
//   - 源文件：key = 目录 + {uuid}{ext}，InsertOnly（不可覆盖）
//   - md 文件（isMd）：key = 目录 + {memoryId}.md，允许覆盖（在线编辑重传）
func (s *RecallMemoryService) GetMemoryUploadToken(ctx context.Context, userId, roleId, memoryId, fileName string, isMd bool) (*GetUploadTokenResult, error) {
	if userId == "" || memoryId == "" {
		return nil, errors.New("userId/memoryId is required")
	}
	accessKey, secretKey, bucket, domain, err := getQiniuConfig()
	if err != nil {
		utils.LogWithCtx(ctx, "RecallMemoryService.GetMemoryUploadToken", "配置检查失败 | err=%v", err)
		return nil, err
	}
	roleId = normalizeRoleId(roleId)
	dir := memoryDir(userId, roleId, memoryId)

	var key string
	putPolicy := storage.PutPolicy{Expires: 3600}
	if isMd {
		key = memoryMdKey(userId, roleId, memoryId)
		// 允许覆盖：scope=bucket:key + 不设 InsertOnly
		putPolicy.Scope = bucket + ":" + key
	} else {
		key = dir + uuid.New().String() + path.Ext(fileName)
		putPolicy.Scope = bucket
		putPolicy.InsertOnly = 1
	}

	mac := auth.New(accessKey, secretKey)
	upToken := putPolicy.UploadToken(mac)
	if upToken == "" {
		return nil, errors.New("生成七牛云上传 token 失败")
	}
	utils.LogWithCtx(ctx, "RecallMemoryService.GetMemoryUploadToken", "token 生成成功 | key=%s isMd=%v", key, isMd)
	return &GetUploadTokenResult{
		Token:     upToken,
		UploadURL: "https://up-z2.qiniup.com",
		Key:       key,
		Domain:    domain,
	}, nil
}

// ===== 保存记忆 =====

// RecallMemoryItem 列表/详情返回项
type RecallMemoryItem struct {
	ID             uint                     `json:"id"`
	MemoryID       string                   `json:"memoryId"`
	UserID         string                   `json:"userId"`
	RoleID         string                   `json:"roleId"`
	Topic          string                   `json:"topic"`
	SubjectiveDesc string                   `json:"subjectiveDesc"`
	MdKey          string                   `json:"mdKey"`
	MdURL          string                   `json:"mdUrl,omitempty"`
	MdContent      string                   `json:"mdContent,omitempty"` // {memoryId}.md 内容(DB缓存)，详情直接下发供前端渲染，免去浏览器再拉对象存储
	ParseStatus    int                      `json:"parseStatus"`
	EditStatus     int                      `json:"editStatus"`
	Status         int                      `json:"status"`
	CreatedAt      time.Time                `json:"createdAt"`
	UpdatedAt      time.Time                `json:"updatedAt"`
	SourceFiles    []RecallSourceFileDetail `json:"sourceFiles"`
}

// RecallSourceFileDetail 源文件详情项
type RecallSourceFileDetail struct {
	FileKey  string `json:"fileKey"`
	FileName string `json:"fileName"`
	FileType int    `json:"fileType"`
	URL      string `json:"url,omitempty"`
}

// SaveMemory 保存记忆（事务）：写主题 + 批量源文件；提交后异步触发 AI 解析。
func (s *RecallMemoryService) SaveMemory(ctx context.Context, userId string, req *request.SaveMemoryRequest) (*RecallMemoryItem, error) {
	if userId == "" {
		return nil, errors.New("userId is required")
	}
	roleId := normalizeRoleId(req.RoleId)
	utils.LogWithCtx(ctx, "RecallMemoryService.SaveMemory", "入参 | userId=%s roleId=%s memoryId=%s topic=%s files=%d",
		userId, roleId, req.MemoryId, req.Topic, len(req.SourceFiles))

	// 主题唯一性强校验
	exists, err := s.repo.ExistsTopic(ctx, userId, roleId, req.Topic)
	if err != nil {
		return nil, fmt.Errorf("主题校验失败: %v", err)
	}
	if exists {
		return nil, ErrTopicExists
	}

	tx := config.DB.Begin()
	if tx.Error != nil {
		return nil, fmt.Errorf("开启事务失败: %v", tx.Error)
	}

	mdKey := memoryMdKey(userId, roleId, req.MemoryId) // 路径确定，提前落库
	m := models.RecallMemory{
		MemoryId:       req.MemoryId,
		UserId:         userId,
		RoleId:         roleId,
		Topic:          req.Topic,
		SubjectiveDesc: req.SubjectiveDesc,
		MdKey:          mdKey,
		ParseStatus:    models.ParseStatusPending,
		EditStatus:     models.EditStatusIdle,
		Status:         models.RecallStatusActive,
	}
	if err := s.repo.CreateWithTx(ctx, tx, &m); err != nil {
		tx.Rollback()
		utils.LogWithCtx(ctx, "RecallMemoryService.SaveMemory", "主题入库失败，回滚 | err=%v", err)
		return nil, fmt.Errorf("记忆主题入库失败: %v", err)
	}

	files := make([]models.RecallSourceFile, 0, len(req.SourceFiles))
	for i, f := range req.SourceFiles {
		// 权威 fileType：扩展名 > 请求值（修复 mp4 被前端误标为文本的 bug）
		f.FileType = resolveSourceFileType(f.FileName, f.FileType, 0)
		req.SourceFiles[i].FileType = f.FileType
		files = append(files, models.RecallSourceFile{
			MemoryId: req.MemoryId,
			FileKey:  f.FileKey,
			FileName: f.FileName,
			FileType: f.FileType,
			Status:   models.RecallStatusActive,
		})
	}
	if err := s.repo.BatchCreateSourceWithTx(ctx, tx, files); err != nil {
		tx.Rollback()
		utils.LogWithCtx(ctx, "RecallMemoryService.SaveMemory", "源文件入库失败，回滚 | err=%v", err)
		return nil, fmt.Errorf("源文件入库失败: %v", err)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("提交事务失败: %v", err)
	}
	utils.LogWithCtx(ctx, "RecallMemoryService.SaveMemory", "落库成功 | memoryId=%s files=%d", req.MemoryId, len(files))

	// 异步触发 AI 解析（fire-and-forget，失败仅记日志）
	s.triggerParseAsync(userId, roleId, &m, req.SourceFiles)

	return &RecallMemoryItem{
		ID:             m.Id,
		MemoryID:       m.MemoryId,
		UserID:         m.UserId,
		RoleID:         m.RoleId,
		Topic:          m.Topic,
		SubjectiveDesc: m.SubjectiveDesc,
		MdKey:          m.MdKey,
		ParseStatus:    m.ParseStatus,
		EditStatus:     m.EditStatus,
		Status:         m.Status,
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
	}, nil
}

// triggerParseAsync 后台调用 echo-ai /memory/parse
//
// 状态生命周期（修复 #5/#6：之前 status 永远停在 0）：
//  1. 先把 parse_status 置为 RUNNING(1)，让用户从列表立即看到"解析中"，而不是无限"待解析"
//  2. 调 echo-ai HTTP；失败时按指数退避重试最多 3 次（覆盖 echo-ai 启动延迟 / 短暂网络抖动）
//  3. 最终成功 → 由 echo-ai 在 parse_memory() 末尾调 mark_done(parse_status=2) 写回 DB
//  4. 重试耗尽 → 把 parse_status 置为 FAILED(3)，用户能立刻看到失败而非卡死
func (s *RecallMemoryService) triggerParseAsync(userId, roleId string, m *models.RecallMemory, srcReq []request.RecallSourceFileItem) {
	go func() {
		ctx := utils.WithUID(context.Background(), userId)
		sources := make([]dto.RecallSourceFileInfo, 0, len(srcReq))
		for _, f := range srcReq {
			url, err := s.fileSvc.GetPrivateURL(ctx, f.FileKey, 24*3600)
			if err != nil {
				utils.LogWithCtx(ctx, "RecallMemoryService.triggerParseAsync", "生成下载URL失败（仍透传key） | key=%s err=%v", f.FileKey, err)
			}
			sources = append(sources, dto.RecallSourceFileInfo{
				FileKey:  f.FileKey,
				FileName: f.FileName,
				FileType: f.FileType,
				URL:      url,
			})
		}

		// 1) 先把状态推到 RUNNING（修复 #5 核心：避免 status 一直停在 PENDING）
		if err := s.repo.UpdateParseStatus(ctx, m.MemoryId, models.ParseStatusRunning); err != nil {
			utils.LogWithCtx(ctx, "RecallMemoryService.triggerParseAsync", "置 RUNNING 失败 | memoryId=%s err=%v", m.MemoryId, err)
		}

		// 2) 调 echo-ai，带重试
		req := dto.ParseMemoryRequest{
			UserID:         userId,
			RoleID:         roleId,
			MemoryID:       m.MemoryId,
			Topic:          m.Topic,
			SubjectiveDesc: m.SubjectiveDesc,
			SourceFiles:    sources,
		}
		const maxAttempts = 3
		var lastErr error
		for attempt := 1; attempt <= maxAttempts; attempt++ {
			if _, err := s.memClient.ParseMemory(ctx, req); err != nil {
				lastErr = err
				utils.LogWithCtx(ctx, "RecallMemoryService.triggerParseAsync",
					"AI 解析触发失败（将重试） | attempt=%d/%d memoryId=%s err=%v",
					attempt, maxAttempts, m.MemoryId, err)
				if attempt < maxAttempts {
					// 指数退避：1s, 2s, 4s ...
					time.Sleep(time.Duration(1<<(attempt-1)) * time.Second)
				}
				continue
			}
			// 成功：echo-ai 已接单，最终状态由它在 parse_memory() 末尾 mark_done 写回
			utils.LogWithCtx(ctx, "RecallMemoryService.triggerParseAsync",
				"AI 解析已派发 | memoryId=%s attempt=%d files=%d", m.MemoryId, attempt, len(sources))
			return
		}

		// 3) 重试耗尽：标 FAILED（修复 #6：echo-ai 不可达时不再无限停留在 RUNNING）
		if err := s.repo.UpdateParseStatus(ctx, m.MemoryId, models.ParseStatusFailed); err != nil {
			utils.LogWithCtx(ctx, "RecallMemoryService.triggerParseAsync", "置 FAILED 失败 | memoryId=%s err=%v", m.MemoryId, err)
		}
		utils.LogWithCtx(ctx, "RecallMemoryService.triggerParseAsync",
			"AI 解析触发最终失败 | memoryId=%s attempts=%d lastErr=%v", m.MemoryId, maxAttempts, lastErr)
	}()
}

// ===== 列表 / 详情 =====

// ListMemories 列出当前用户当前角色下的记忆主题
func (s *RecallMemoryService) ListMemories(ctx context.Context, userId, roleId string) ([]RecallMemoryItem, error) {
	if userId == "" {
		return nil, errors.New("userId is required")
	}
	list, err := s.repo.ListByUserRole(ctx, userId, roleId)
	if err != nil {
		return nil, err
	}
	out := make([]RecallMemoryItem, 0, len(list))
	for i := range list {
		m := list[i]
		// 列表也返回源文件数（轻量，不含 url）
		files, _ := s.repo.ListSourceByMemoryId(ctx, m.MemoryId)
		out = append(out, RecallMemoryItem{
			ID:             m.Id,
			MemoryID:       m.MemoryId,
			UserID:         m.UserId,
			RoleID:         m.RoleId,
			Topic:          m.Topic,
			SubjectiveDesc: m.SubjectiveDesc,
			MdKey:          m.MdKey,
			ParseStatus:    m.ParseStatus,
			EditStatus:     m.EditStatus,
			Status:         m.Status,
			CreatedAt:      m.CreatedAt,
			UpdatedAt:      m.UpdatedAt,
			SourceFiles:    toFileDetails(files),
		})
	}
	utils.LogWithCtx(ctx, "RecallMemoryService.ListMemories", "完成 | count=%d", len(out))
	return out, nil
}

// toFileDetails 仓储层 model → 详情 DTO（列表场景 url 为空，减少带宽）
func toFileDetails(files []models.RecallSourceFile) []RecallSourceFileDetail {
	out := make([]RecallSourceFileDetail, 0, len(files))
	for _, f := range files {
		out = append(out, RecallSourceFileDetail{
			FileKey:  f.FileKey,
			FileName: f.FileName,
			FileType: f.FileType,
		})
	}
	return out
}

// UpdateMdContent 单独更新 md_content 字段（DB 缓存，避免 Qiniu CDN 下载）
func (s *RecallMemoryService) UpdateMdContent(ctx context.Context, memoryId, mdContent string) error {
	if memoryId == "" {
		return errors.New("memoryId is required")
	}
	utils.LogWithCtx(ctx, "RecallMemoryService.UpdateMdContent", "memoryId=%s bytes=%d", memoryId, len(mdContent))
	return s.repo.UpdateMdContent(ctx, memoryId, mdContent)
}

// UpdateParseStatus 由内部回调或运维修正使用（POST /api/memory/internal/parse-status）。
//
// 若 memoryId 不存在（被软删/从未入库），返回 ErrMemoryNotFound，方便 echo-ai 排查。
//   - 该方法不校验 userId（内部接口自带 token 鉴权，避免越权）
func (s *RecallMemoryService) UpdateParseStatus(ctx context.Context, memoryId string, status int) error {
	if memoryId == "" {
		return errors.New("memoryId is required")
	}
	// 先确认记录存在（排除被软删/误传的 memoryId）
	if _, err := s.repo.GetByMemoryId(ctx, memoryId); err != nil {
		return ErrMemoryNotFound
	}
	utils.LogWithCtx(ctx, "RecallMemoryService.UpdateParseStatus", "memoryId=%s status=%d", memoryId, status)
	return s.repo.UpdateParseStatus(ctx, memoryId, status)
}

// GetMemoryDetail 记忆详情（含源文件下载链接 + md 下载链接）
func (s *RecallMemoryService) GetMemoryDetail(ctx context.Context, userId, memoryId string) (*RecallMemoryItem, error) {
	if userId == "" {
		return nil, errors.New("userId is required")
	}
	m, err := s.repo.GetByMemoryId(ctx, memoryId)
	if err != nil {
		return nil, ErrMemoryNotFound
	}
	if m.UserId != userId {
		return nil, ErrMemoryNotFound
	}
	files, err := s.repo.ListSourceByMemoryId(ctx, memoryId)
	if err != nil {
		return nil, err
	}
	details := make([]RecallSourceFileDetail, 0, len(files))
	for _, f := range files {
		url, _ := s.fileSvc.GetPrivateURL(ctx, f.FileKey, 3600)
		details = append(details, RecallSourceFileDetail{
			FileKey:  f.FileKey,
			FileName: f.FileName,
			FileType: f.FileType,
			URL:      url,
		})
	}
	item := &RecallMemoryItem{
		ID:             m.Id,
		MemoryID:       m.MemoryId,
		UserID:         m.UserId,
		RoleID:         m.RoleId,
		Topic:          m.Topic,
		SubjectiveDesc: m.SubjectiveDesc,
		MdKey:          m.MdKey,
		MdContent:      m.MdContent, // 直接下发 DB 缓存的 md 正文，前端优先渲染它（对象存储可能 404/CDN 421/跨域）
		ParseStatus:    m.ParseStatus,
		EditStatus:     m.EditStatus,
		Status:         m.Status,
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
		SourceFiles:    details,
	}
	// md 下载链接（仅解析完成后有意义）
	if m.MdKey != "" {
		if u, err := s.fileSvc.GetPrivateURL(ctx, m.MdKey, 3600); err == nil {
			item.MdURL = u
		}
	}
	return item, nil
}

// ===== 编辑（更新）已有记忆 =====

// UpdateMemory 编辑已有记忆（事务内更新主题/主观描述 + 源文件增量同步）。
//
// 与 SaveMemory 的关键差异：
//  1. **不校验主题唯一性** —— 编辑是更新同一条记录，与自身 (userId, roleId, topic) 匹配是预期行为。
//  2. **源文件按 delta 同步**：请求列表"全量提交"，服务端 diff 出新增 / 待删：
//     - 新增（fileKey 不在 DB 现有）→ INSERT；新文件必须在 upload-token 阶段就已传到对象存储。
//     - 待删（DB 现有但不在请求列表中）→ 软删（级联在事务提交后的后台真实删对象 + AI 更新）。
//  3. **按需异步重新解析**：服务端以"权威 diff"为准——
//     即使前端传 needReparse=true，也必须经过 DB 旧值 vs 请求新值的对比，确有改动才触发 AI。
//     这样可以彻底避免"前端 dirty 状态错乱/竞态/重复点击"导致的无谓 LLM 调用与向量重建
//     （典型症状：md 内容从"形象描述"退化成"光圈/ISO 技术参数"）。
func (s *RecallMemoryService) UpdateMemory(ctx context.Context, userId string, req *request.UpdateMemoryRequest) (*RecallMemoryItem, error) {
	if userId == "" {
		return nil, errors.New("userId is required")
	}
	roleId := normalizeRoleId(req.RoleId)
	utils.LogWithCtx(ctx, "RecallMemoryService.UpdateMemory",
		"入参 | userId=%s roleId=%s memoryId=%s topic=%s files=%d clientNeedReparse=%v",
		userId, roleId, req.MemoryId, req.Topic, len(req.SourceFiles), req.NeedReparse)

	// 1. 鉴权 + 存在性校验
	m, err := s.repo.GetByMemoryId(ctx, req.MemoryId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrMemoryNotFound
		}
		return nil, fmt.Errorf("查询记忆失败: %w", err)
	}
	if m.UserId != userId {
		// 不暴露存在性差异，统一按"不存在"返回
		return nil, ErrMemoryNotFound
	}

	// 2. 计算权威 diff（DB 旧值 vs 请求新值）—— 与事务提交无关，只读操作
	existingKeys, err := s.repo.ListActiveSourceFileKeys(ctx, req.MemoryId)
	if err != nil {
		return nil, fmt.Errorf("查询源文件失败: %w", err)
	}
	// 编辑场景：拉 DB 旧 fileType 用于权威归一（修复 mp4 被前端误标为文本的 bug）
	existingDBTypes := s.repo.ListActiveSourceFileTypes(ctx, req.MemoryId)
	requestedSet := make(map[string]struct{}, len(req.SourceFiles))
	for i, f := range req.SourceFiles {
		requestedSet[f.FileKey] = struct{}{}
		// 即使是已存在文件，扩展名兜底仍要算一次（首次 DB 落错/扩展名后被改名时纠正）
		f.FileType = resolveSourceFileType(f.FileName, f.FileType, existingDBTypes[f.FileKey])
		req.SourceFiles[i].FileType = f.FileType
	}
	requestedKeys := make([]string, 0, len(req.SourceFiles))
	for k := range requestedSet {
		requestedKeys = append(requestedKeys, k)
	}
	toInsert, toSoftDelete, _ := diffSourceFiles(existingKeys, requestedKeys)

	topicChanged := req.Topic != m.Topic
	// desc 比较用 TrimSpace 双向规范化：避免 DB 中的尾随空格/换行被前端 trim 后误判为"改动"。
	// 这类小细节是触发"无改动却又被重解析"的常见原因（用户在 UI 看上去啥都没改就 save）。
	descChanged := strings.TrimSpace(req.SubjectiveDesc) != strings.TrimSpace(m.SubjectiveDesc)
	filesChangedAuthoritative := len(toInsert) > 0 || len(toSoftDelete) > 0
	authoritativeNeedReparse := topicChanged || descChanged || filesChangedAuthoritative

	// 当客户端值与权威值不一致时记录告警——典型是 UI 层 dirty 状态计算错误
	if req.NeedReparse != authoritativeNeedReparse {
		utils.LogWithCtx(ctx, "RecallMemoryService.UpdateMemory",
			"needReparse 不一致，以权威 diff 为准 | clientNeedReparse=%v authoritative=%v topicChanged=%v descChanged=%v filesChanged=%v",
			req.NeedReparse, authoritativeNeedReparse, topicChanged, descChanged, filesChangedAuthoritative)
	}

	// 3. 事务：更新主题/描述 + 源文件 diff
	tx := config.DB.Begin()
	if tx.Error != nil {
		return nil, fmt.Errorf("开启事务失败: %v", tx.Error)
	}

	// 3.1 编辑时，parse_status 归零；权威值决定后续是否推到 RUNNING。
	if err := s.repo.UpdateMemoryFieldsWithTx(ctx, tx, req.MemoryId, req.Topic, req.SubjectiveDesc, models.ParseStatusPending); err != nil {
		tx.Rollback()
		utils.LogWithCtx(ctx, "RecallMemoryService.UpdateMemory", "更新主题/描述失败 | err=%v", err)
		return nil, fmt.Errorf("更新记忆失败: %w", err)
	}

	// 3.2 源文件 delta 同步
	if len(toInsert) > 0 {
		files := make([]models.RecallSourceFile, 0, len(toInsert))
		for _, f := range toInsert {
			files = append(files, models.RecallSourceFile{
				MemoryId: req.MemoryId,
				FileKey:  f,
				FileName: lookupFileName(req.SourceFiles, f),
				FileType: lookupFileType(req.SourceFiles, f), // 上方已权威归一过
				Status:   models.RecallStatusActive,
			})
		}
		if err := s.repo.BatchCreateSourceWithTx(ctx, tx, files); err != nil {
			tx.Rollback()
			utils.LogWithCtx(ctx, "RecallMemoryService.UpdateMemory", "新增源文件入库失败 | err=%v", err)
			return nil, fmt.Errorf("新增源文件失败: %w", err)
		}
	}
	for _, k := range toSoftDelete {
		if _, err := s.repo.SoftDeleteSourceWithTx(ctx, tx, req.MemoryId, k); err != nil {
			tx.Rollback()
			utils.LogWithCtx(ctx, "RecallMemoryService.UpdateMemory", "软删源文件失败 | key=%s err=%v", k, err)
			return nil, fmt.Errorf("删除源文件失败: %w", err)
		}
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("提交事务失败: %v", err)
	}
	utils.LogWithCtx(ctx, "RecallMemoryService.UpdateMemory",
		"落库成功 | memoryId=%s insert=%d softDelete=%d authoritativeNeedReparse=%v",
		req.MemoryId, len(toInsert), len(toSoftDelete), authoritativeNeedReparse)

	// 4. 仅在权威 diff 非空时，才异步触发 AI 重新解析
	if authoritativeNeedReparse {
		s.triggerUpdateParseAsync(userId, roleId, req.MemoryId, req.Topic, req.SubjectiveDesc, req.SourceFiles, toSoftDelete)
	}

	// 5. 回查并返回最新记忆（让前端拿到 parse_status/edit_status 等状态字段）
	return s.GetMemoryDetail(ctx, userId, req.MemoryId)
}

// inferFileTypeByExt 按文件扩展名推断 fileType；无法识别返回 0。
//
// 优先级：扩展名 > 声明 fileType。修复视频编辑重解析 bug 的服务端兜底：
// 当前端因 File 占位 type="" 导致 fileType 被错算成 1(文本) 时，扩展名能识别
// 就能保证不会把 mp4 字节流当文本转发给 echo-ai。
func inferFileTypeByExt(fileName string) int {
	if fileName == "" {
		return 0
	}
	ext := strings.ToLower(strings.TrimPrefix(path.Ext(fileName), "."))
	switch ext {
	case "mp4", "mov", "m4v", "mkv", "webm", "avi", "flv", "wmv", "3gp":
		return 3
	case "mp3", "wav", "m4a", "aac", "flac", "ogg", "opus", "wma", "amr":
		return 4
	case "jpg", "jpeg", "png", "gif", "webp", "bmp", "heic", "heif", "tiff", "tif":
		return 2
	case "txt", "md", "markdown", "log", "csv", "json", "xml", "html", "htm", "yaml", "yml":
		return 1
	default:
		return 0
	}
}

// resolveSourceFileType 综合三处证据算 fileType：扩展名 > DB 旧值 > 请求值。
//
// 关键场景：
//   - 已知扩展名 + 任意请求值：按扩展名（覆盖视频误标成文本）
//   - 未知扩展名 + DB 有旧值：用 DB 旧值（信任首次落库的判定）
//   - 未知扩展名 + DB 无旧值：用请求值
//   - 三处都不可判定：兜底 1(文本)
func resolveSourceFileType(fileName string, reqType, dbType int) int {
	extType := inferFileTypeByExt(fileName)
	if extType != 0 {
		return extType
	}
	if dbType == 1 || dbType == 2 || dbType == 3 || dbType == 4 {
		return dbType
	}
	if reqType == 1 || reqType == 2 || reqType == 3 || reqType == 4 {
		return reqType
	}
	return 1
}

// normalizeSourceFileTypes 批量归一，对每个 fileName+fileType 算权威值。
func normalizeSourceFileTypes(items []request.RecallSourceFileItem) {
	for i := range items {
		items[i].FileType = resolveSourceFileType(items[i].FileName, items[i].FileType, 0)
	}
}

// normalizeWithDB 用 DB 旧值补强（编辑场景），并把"请求 fileType 越界"这种脏数据
// 一并归一。返回 (归一后的 items, key→fileType 字典) 供调用方按需取。
func normalizeWithDB(
	items []request.RecallSourceFileItem,
	dbTypes map[string]int,
) []dto.RecallSourceFileInfo {
	out := make([]dto.RecallSourceFileInfo, 0, len(items))
	for _, f := range items {
		dbType, hasDB := dbTypes[f.FileKey]
		ft := resolveSourceFileType(f.FileName, f.FileType, dbType)
		if hasDB && ft != f.FileType {
			utils.LogWithCtx(context.Background(),
				"RecallMemoryService.normalizeWithDB",
				"fileType 纠正 | key=%s fileName=%s req=%d db=%d resolved=%d",
				f.FileKey, f.FileName, f.FileType, dbType, ft)
		}
		out = append(out, dto.RecallSourceFileInfo{
			FileKey:  f.FileKey,
			FileName: f.FileName,
			FileType: ft,
		})
	}
	return out
}

// diffSourceFiles 计算源文件 diff：
//   - toInsert：请求中有但 DB 中没有的 fileKey
//   - toSoftDelete：DB 中有但请求中没有的 fileKey
//   - unchanged：DB 与请求都存在的 fileKey（仅校验用）
func diffSourceFiles(existingKeys []string, requestedKeys []string) (toInsert []string, toSoftDelete []string, unchanged []string) {
	existingSet := make(map[string]struct{}, len(existingKeys))
	for _, k := range existingKeys {
		existingSet[k] = struct{}{}
	}
	requestedSet := make(map[string]struct{}, len(requestedKeys))
	for _, k := range requestedKeys {
		requestedSet[k] = struct{}{}
	}
	for k := range requestedSet {
		if _, ok := existingSet[k]; !ok {
			toInsert = append(toInsert, k)
		} else {
			unchanged = append(unchanged, k)
		}
	}
	for k := range existingSet {
		if _, ok := requestedSet[k]; !ok {
			toSoftDelete = append(toSoftDelete, k)
		}
	}
	return
}

// lookupFileName 从 SourceFileItem 列表中按 fileKey 取出 fileName（用于 INSERT）。
func lookupFileName(items []request.RecallSourceFileItem, key string) string {
	for _, x := range items {
		if x.FileKey == key {
			return x.FileName
		}
	}
	return ""
}

// lookupFileType 同上，取 fileType。
func lookupFileType(items []request.RecallSourceFileItem, key string) int {
	for _, x := range items {
		if x.FileKey == key {
			return x.FileType
		}
	}
	return 0
}

// triggerUpdateParseAsync 编辑后异步触发 AI 解析。
//
// 与 SaveMemory.triggerParseAsync 的差异：
//  1. 走 Python /memory/reparse（独立 stage/log，便于排查"编辑后再解析"链路）。
//  2. 仍按 (1,2,4) 秒指数退避重试 3 次；最终失败置 FAILED。
//  3. 透传编辑后的 Topic/SubjectiveDesc（SaveMemory 是从内存 m 读，这里直接收参），
//     保证 AI 重新生成的 md 用的是"用户最后一次看到的"主题与主观描述，而不是 DB 旧值。
func (s *RecallMemoryService) triggerUpdateParseAsync(userId, roleId, memoryId, topic, subjectiveDesc string, srcReq []request.RecallSourceFileItem, removedKeys []string) {
	go func() {
		ctx := utils.WithUID(context.Background(), userId)
		sources := make([]dto.RecallSourceFileInfo, 0, len(srcReq))
		for _, f := range srcReq {
			url, err := s.fileSvc.GetPrivateURL(ctx, f.FileKey, 24*3600)
			if err != nil {
				utils.LogWithCtx(ctx, "RecallMemoryService.triggerUpdateParseAsync",
					"生成下载URL失败（仍透传key） | key=%s err=%v", f.FileKey, err)
			}
			sources = append(sources, dto.RecallSourceFileInfo{
				FileKey:  f.FileKey,
				FileName: f.FileName,
				FileType: f.FileType,
				URL:      url,
			})
		}

		// 1) 推到 RUNNING，避免用户列表永远停在"待解析"
		if err := s.repo.UpdateParseStatus(ctx, memoryId, models.ParseStatusRunning); err != nil {
			utils.LogWithCtx(ctx, "RecallMemoryService.triggerUpdateParseAsync",
				"置 RUNNING 失败 | memoryId=%s err=%v", memoryId, err)
		}

		// 2) 调用 echo-ai /memory/reparse，带重试
		req := dto.ReparseMemoryRequest{
			UserID:          userId,
			RoleID:          roleId,
			MemoryID:        memoryId,
			Topic:           topic,
			SubjectiveDesc:  subjectiveDesc,
			SourceFiles:     sources,
			RemovedFileKeys: removedKeys,
		}
		const maxAttempts = 3
		var lastErr error
		for attempt := 1; attempt <= maxAttempts; attempt++ {
			if _, err := s.memClient.ReparseMemory(ctx, req); err != nil {
				lastErr = err
				utils.LogWithCtx(ctx, "RecallMemoryService.triggerUpdateParseAsync",
					"AI 编辑再解析触发失败（将重试） | attempt=%d/%d memoryId=%s err=%v",
					attempt, maxAttempts, memoryId, err)
				if attempt < maxAttempts {
					time.Sleep(time.Duration(1<<(attempt-1)) * time.Second)
				}
				continue
			}
			utils.LogWithCtx(ctx, "RecallMemoryService.triggerUpdateParseAsync",
				"AI 编辑再解析已派发 | memoryId=%s attempt=%d files=%d removed=%d",
				memoryId, attempt, len(sources), len(removedKeys))
			return
		}

		// 3) 重试耗尽：标 FAILED
		if err := s.repo.UpdateParseStatus(ctx, memoryId, models.ParseStatusFailed); err != nil {
			utils.LogWithCtx(ctx, "RecallMemoryService.triggerUpdateParseAsync",
				"置 FAILED 失败 | memoryId=%s err=%v", memoryId, err)
		}
		utils.LogWithCtx(ctx, "RecallMemoryService.triggerUpdateParseAsync",
			"AI 编辑再解析最终失败 | memoryId=%s attempts=%d lastErr=%v",
			memoryId, maxAttempts, lastErr)
	}()
}

// contains 小工具：O(n) 切片包含判定；n 为源文件数（一般 < 100）。
// 保留以备其它场景；UpdateMemory 内的 diff 改用 diffSourceFiles（更快、可读）。
func contains(set []string, v string) bool {
	for _, x := range set {
		if x == v {
			return true
		}
	}
	return false
}

// DeleteSourceFile 硬删源文件记录（事务）；提交后异步删对象 + 更新 md/向量
func (s *RecallMemoryService) DeleteSourceFile(ctx context.Context, userId string, req *request.DeleteSourceFileReq) error {
	if userId == "" {
		return errors.New("userId is required")
	}
	m, err := s.repo.GetByMemoryId(ctx, req.MemoryId)
	if err != nil || m.UserId != userId {
		return ErrMemoryNotFound
	}

	tx := config.DB.Begin()
	if tx.Error != nil {
		return fmt.Errorf("开启事务失败: %v", tx.Error)
	}
	affected, err := s.repo.HardDeleteSourceWithTx(ctx, tx, req.MemoryId, req.FileKey)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("删除源文件失败: %v", err)
	}
	if affected == 0 {
		tx.Rollback()
		return errors.New("源文件不存在")
	}
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("提交事务失败: %v", err)
	}
	utils.LogWithCtx(ctx, "RecallMemoryService.DeleteSourceFile", "记录硬删成功 | memoryId=%s fileKey=%s", req.MemoryId, req.FileKey)

	// 异步：删对象存储文件 + 通知 AI 更新 md/向量
	go func() {
		bg := utils.WithUID(context.Background(), userId)
		if err := DeleteObject(bg, req.FileKey); err != nil {
			utils.LogWithCtx(bg, "RecallMemoryService.DeleteSourceFile", "对象删除失败 | key=%s err=%v", req.FileKey, err)
		}
		if _, err := s.memClient.DeleteSourceFile(bg, dto.DeleteSourceFileRequest{
			UserID:   userId,
			RoleID:   m.RoleId,
			MemoryID: req.MemoryId,
			FileKey:  req.FileKey,
		}); err != nil {
			utils.LogWithCtx(bg, "RecallMemoryService.DeleteSourceFile", "AI 更新触发失败 | memoryId=%s err=%v", req.MemoryId, err)
		}
	}()
	return nil
}

// ===== 删除整个主题 =====

// DeleteMemoryTheme 硬删主题 + 全部源文件（事务）；提交后异步删目录 + 删向量
func (s *RecallMemoryService) DeleteMemoryTheme(ctx context.Context, userId string, req *request.DeleteMemoryThemeReq) error {
	if userId == "" {
		return errors.New("userId is required")
	}
	m, err := s.repo.GetByMemoryId(ctx, req.MemoryId)
	if err != nil || m.UserId != userId {
		return ErrMemoryNotFound
	}

	tx := config.DB.Begin()
	if tx.Error != nil {
		return fmt.Errorf("开启事务失败: %v", tx.Error)
	}
	if err := s.repo.HardDeleteAllSourceWithTx(ctx, tx, req.MemoryId); err != nil {
		tx.Rollback()
		return fmt.Errorf("删除源文件失败: %v", err)
	}
	if err := s.repo.HardDeleteWithTx(ctx, tx, req.MemoryId); err != nil {
		tx.Rollback()
		return fmt.Errorf("删除主题失败: %v", err)
	}
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("提交事务失败: %v", err)
	}
	utils.LogWithCtx(ctx, "RecallMemoryService.DeleteMemoryTheme", "记录硬删成功 | memoryId=%s", req.MemoryId)

	// 异步：删整个目录 + 通知 AI 删向量
	go func() {
		bg := utils.WithUID(context.Background(), userId)
		if err := DeleteByPrefix(bg, memoryDir(userId, m.RoleId, req.MemoryId)); err != nil {
			utils.LogWithCtx(bg, "RecallMemoryService.DeleteMemoryTheme", "目录删除失败 | memoryId=%s err=%v", req.MemoryId, err)
		}
		if _, err := s.memClient.DeleteMemory(bg, dto.DeleteMemoryRequest{
			UserID:   userId,
			RoleID:   m.RoleId,
			MemoryID: req.MemoryId,
		}); err != nil {
			utils.LogWithCtx(bg, "RecallMemoryService.DeleteMemoryTheme", "AI 删除触发失败 | memoryId=%s err=%v", req.MemoryId, err)
		}
	}()
	return nil
}
