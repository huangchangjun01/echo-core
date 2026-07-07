package dto

// IngestFileInfo /ingest_file 请求体中的 file 子对象
// 对齐 Python 接口文档。
type IngestFileInfo struct {
	FileID   string `json:"fileId" binding:"required"`
	FileName string `json:"fileName" binding:"required"`
	FileKey  string `json:"fileKey" binding:"required"`
	URL      string `json:"url" binding:"required"`
}

// IngestFileRequest 调用 Python /ingest_file 的请求体
type IngestFileRequest struct {
	UserID string         `json:"userId" binding:"required"`
	File   IngestFileInfo `json:"file" binding:"required"`
}

// IngestFileResponse Python /ingest_file 的响应
type IngestFileResponse struct {
	OK     bool   `json:"ok"`
	Queued bool   `json:"queued"`
	FileID string `json:"fileId"`
}
