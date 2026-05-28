package request

// IngestFileRequest 调用 Python /ingest_file 接口的请求结构
type IngestFileRequest struct {
	UserID string `json:"userId"`
	File   struct {
		FileID   string `json:"fileId"`
		FileName string `json:"fileName"`
		FileKey  string `json:"fileKey"`
		Url      string `json:"url"`
	} `json:"file"`
}