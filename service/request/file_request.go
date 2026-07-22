package request

// GetUploadTokenRequest 获取上传token的请求参数
type GetUploadTokenRequest struct {
	FileName string `json:"fileName" binding:"required"`
	FileSize int64  `json:"fileSize" binding:"required"`
	MimeType string `json:"mimeType" binding:"required"`
	BizType  string `json:"bizType" binding:"required"`
}

// RegisterFileRequest 注册文件请求参数
//
// 注：userId 不在此结构内。userId 强制从已鉴权的 session 中取
// （middleware.MustUserID(c)），不接受请求体里的 userId 字段，
// 既避免前端 authStore 还没就绪时 userId 为空串导致的 binding 校验失败，
// 也杜绝前端伪造他人 userId 的越权风险。
type RegisterFileRequest struct {
	FileName string `json:"fileName" binding:"required"`
	Key      string `json:"key" binding:"required"`
	FileType int    `json:"fileType" binding:"required"`
	BizType  int    `json:"bizType"`
	// Desc 为用户为文件填写的文字描述，会一并转发给 Python 用于生成记忆。
	Desc string `json:"desc" binding:"omitempty,max=4000"`
	// RoleId 角色标识；用于按角色隔离记忆与 RAG 索引。
	RoleId string `json:"roleId" binding:"omitempty,max=128"`
}

// UpdateFileDescRequest 修改文件描述
type UpdateFileDescRequest struct {
	Desc string `json:"desc" binding:"omitempty,max=4000"`
}

// CreateTextMemoryRequest 新建纯文本记忆（仅描述，无文件）
type CreateTextMemoryRequest struct {
	Desc   string `json:"desc" binding:"required,min=1,max=4000"`
	RoleId string `json:"roleId" binding:"omitempty,max=128"`
}