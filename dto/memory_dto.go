package dto

// ===== echo-ai 回忆记忆客户端出入参（对齐 Python /memory/* 接口）=====

// RecallSourceFileInfo 源文件信息（transfer 给 Python）
type RecallSourceFileInfo struct {
	FileKey  string `json:"fileKey"`
	FileName string `json:"fileName"`
	FileType int    `json:"fileType"` // 1文本 2图片 3视频 4音频
	// URL 为带鉴权的可访问下载地址（私有空间临时链接），Python 下载用。
	URL string `json:"url,omitempty"`
}

// ParseMemoryRequest 调用 Python /memory/parse 的请求体
type ParseMemoryRequest struct {
	UserID         string                 `json:"userId"`
	RoleID         string                 `json:"roleId"`
	MemoryID       string                 `json:"memoryId"`
	Topic          string                 `json:"topic"`
	SubjectiveDesc string                 `json:"subjectiveDesc,omitempty"`
	SourceFiles    []RecallSourceFileInfo `json:"sourceFiles"`
}

// DeleteSourceFileRequest 调用 Python /memory/source/delete 的请求体
type DeleteSourceFileRequest struct {
	UserID   string `json:"userId"`
	RoleID   string `json:"roleId"`
	MemoryID string `json:"memoryId"`
	FileKey  string `json:"fileKey"`
}

// DeleteMemoryRequest 调用 Python /memory/delete 的请求体
type DeleteMemoryRequest struct {
	UserID   string `json:"userId"`
	RoleID   string `json:"roleId"`
	MemoryID string `json:"memoryId"`
}

// ReparseMemoryRequest 调用 Python /memory/reparse 的请求体。
//
// 编辑后调用：包含当前全量源文件 + 本次被移除的文件 key（用于 AI 端决定
// 是走"全量重建"还是"增量 md 修改"）。
//   - SourceFiles：当前记忆下保留下来的全部源文件（含已存在的与新增的）。
//   - RemovedFileKeys：本次编辑中软删的源文件 key（用于在 md 中删除对应小节、
//     与 /memory/source/delete 行为对齐）。
type ReparseMemoryRequest struct {
	UserID          string                 `json:"userId"`
	RoleID          string                 `json:"roleId"`
	MemoryID        string                 `json:"memoryId"`
	Topic           string                 `json:"topic"`
	SubjectiveDesc  string                 `json:"subjectiveDesc,omitempty"`
	SourceFiles     []RecallSourceFileInfo `json:"sourceFiles"`
	RemovedFileKeys []string               `json:"removedFileKeys,omitempty"`
}

// RecallAsyncResponse Python /memory/* 的统一异步响应
type RecallAsyncResponse struct {
	OK       bool   `json:"ok"`
	Queued   bool   `json:"queued"`
	MemoryID string `json:"memoryId"`
}
