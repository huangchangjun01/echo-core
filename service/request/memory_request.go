package request

// ===== 回忆记忆（文档即记忆）接口入参 =====
//
// 统一约定：userId 一律从已鉴权 session 取（middleware.MustUserID），
// 不接受请求体中的 userId，防越权与空串校验失败。

// CheckTopicRequest 记忆主题唯一性校验
type CheckTopicRequest struct {
	RoleId string `json:"roleId" binding:"omitempty,max=128"`
	Topic  string `json:"topic" binding:"required,min=1,max=255"`
}

// MemoryUploadTokenRequest 申请记忆目录内的上传 token
type MemoryUploadTokenRequest struct {
	RoleId   string `json:"roleId" binding:"omitempty,max=128"`
	MemoryId string `json:"memoryId" binding:"required,len=32"`
	FileName string `json:"fileName" binding:"required,max=255"`
	// IsMd 为 true 时申请 {memoryId}.md 的可覆盖 token（在线编辑用）
	IsMd bool `json:"isMd"`
}

// RecallSourceFileItem 保存记忆时的单个源文件
type RecallSourceFileItem struct {
	FileKey  string `json:"fileKey" binding:"required,max=512"`
	FileName string `json:"fileName" binding:"required,max=255"`
	FileType int    `json:"fileType" binding:"required,oneof=1 2 3 4"`
}

// SaveMemoryRequest 保存记忆（落库 + 异步触发 AI 解析）
type SaveMemoryRequest struct {
	MemoryId       string                 `json:"memoryId" binding:"required,len=32"`
	RoleId         string                 `json:"roleId" binding:"omitempty,max=128"`
	Topic          string                 `json:"topic" binding:"required,min=1,max=255"`
	SubjectiveDesc string                 `json:"subjectiveDesc" binding:"omitempty,max=1000"`
	SourceFiles    []RecallSourceFileItem `json:"sourceFiles" binding:"required,min=1,dive"`
}

// UpdateMemoryRequest 编辑已有记忆（事务内更新主题/描述 + 增量同步源文件）。
//
// 关键差异（与 SaveMemoryRequest）：
//  1. 服务端不再校验主题唯一性 —— 编辑是更新同一条记录，不会与自身冲突。
//  2. 源文件按 delta 增量同步：请求中的列表全量提交，新增条目 INSERT，
//     请求中缺席但 DB 已存在的条目软删（与服务 DeleteSourceFile 一致的语义）。
//  3. NeedReparse 由前端基于"源文件增删 + subjectiveDesc 是否改动"算出：
//     - true  → 提交后异步触发 AI 重新解析（删向量 → 重建）。
//     - false → 仅落库，不触发 AI；适用于"无任何改动却点了保存"的场景。
type UpdateMemoryRequest struct {
	MemoryId       string                 `json:"memoryId" binding:"required,len=32"`
	RoleId         string                 `json:"roleId" binding:"omitempty,max=128"`
	Topic          string                 `json:"topic" binding:"required,min=1,max=255"`
	SubjectiveDesc string                 `json:"subjectiveDesc" binding:"omitempty,max=1000"`
	SourceFiles    []RecallSourceFileItem `json:"sourceFiles" binding:"required,min=1,dive"`
	NeedReparse    bool                   `json:"needReparse"`
}

// DeleteSourceFileReq 删除单个源文件
type DeleteSourceFileReq struct {
	MemoryId string `json:"memoryId" binding:"required,len=32"`
	FileKey  string `json:"fileKey" binding:"required,max=512"`
}

// DeleteMemoryThemeReq 删除整个记忆主题
type DeleteMemoryThemeReq struct {
	MemoryId string `json:"memoryId" binding:"required,len=32"`
}
