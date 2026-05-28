package request

// GetUploadTokenRequest 获取上传token的请求参数
type GetUploadTokenRequest struct {
	FileName string `json:"fileName" binding:"required"`
	FileSize int64  `json:"fileSize" binding:"required"`
	MimeType string `json:"mimeType" binding:"required"`
	BizType  string `json:"bizType" binding:"required"`
}

// RegisterFileRequest 注册文件请求参数
type RegisterFileRequest struct {
	UserId   string `json:"userId" binding:"required"`
	FileName string `json:"fileName" binding:"required"`
	Key      string `json:"key" binding:"required"`
	FileType int    `json:"fileType" binding:"required"`
	BizType  int    `json:"bizType"`
}
