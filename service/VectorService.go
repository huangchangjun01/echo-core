package service

import (
	"go-start/remote"
)

type VectorService struct {
	fileService *FileService
	echoRemote  *remote.EchoRemote
}

func NewVectorService() *VectorService {
	return &VectorService{
		echoRemote: remote.NewEchoRemote(),
	}
}

// GetVectorFromEcho 从 Echo 获取向量数据
func (s *VectorService) GetVectorFromEcho(imageData []byte) ([]float32, error) {
	return s.echoRemote.GetImageEmbedding(imageData)
}
