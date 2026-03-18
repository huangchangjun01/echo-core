package service

import (
	"context"
	"errors"
	"fmt"
	"github.com/qiniu/go-sdk/v7/auth"
	"github.com/qiniu/go-sdk/v7/storage"
	"io"
	"log"
	"net/http"
	"runtime/debug"
	"time"
)

var (
	accessKey = "L1PcZ8nUNX8XdeGSH0VcdlB5GsjsLfpf3qZ5-4iU"
	secretKey = "U_5FvdBhV-LCKdvBy0T8tSkfErZamQNiHSZA_eHv"
	bucket    = "huangchangjun"
	domain    = "tc155y2lr.hn-bkt.clouddn.com"
)

type FileService struct {
	vectorService *VectorService
}

func NewFileService() *FileService {
	return &FileService{vectorService: NewVectorService()}
}

// checkConfig 检查七牛云配置是否完整
func checkConfig() error {
	if accessKey == "" || secretKey == "" || bucket == "" || domain == "" {
		return errors.New("七牛云配置不完整，请检查 accessKey、secretKey、bucket 和 domain")
	}
	log.Println("七牛云配置检查通过")
	return nil
}

func (h *FileService) UploadToQiniu(file io.Reader, key string) (string, error) {
	// 检查配置
	if err := checkConfig(); err != nil {
		log.Println(err)
		return "", err
	}
	// 创建凭证
	putPolicy := storage.PutPolicy{
		Scope: bucket + ":" + key,
	}
	mac := auth.New(accessKey, secretKey)
	upToken := putPolicy.UploadToken(mac)

	// 配置存储区域
	cfg := storage.Config{}
	// 根据bucket所在区域设置，这里假设华东
	cfg.Zone = &storage.ZoneHuanan
	// 是否使用https
	cfg.UseHTTPS = false
	// 构建表单上传的对象
	formUploader := storage.NewFormUploader(&cfg)
	ret := storage.PutRet{}
	// 可选配置
	putExtra := storage.PutExtra{}

	err := formUploader.Put(context.Background(), &ret, upToken, key, file, -1, &putExtra)
	if err != nil {
		log.Println("上传七牛云失败：", ret, err)
		return "", err
	}
	log.Println("上传七牛云成功：", ret)
	// 返回完整的访问URL
	return domain + "/" + ret.Key, nil
}

func (h *FileService) Upload(file io.Reader, key string) (string, error) {
	// 1. 上传文件到七牛云
	url, err := h.UploadToQiniu(file, key)
	if err != nil {
		log.Println("上传文件失败", err)
		return "", err
	}

	// 2. 将文件转换为 base64
	baseData, err := convertFileForBase64(url)
	if err != nil {
		log.Println("文件转换base64失败", err)
		return "", err
	}

	// 3. 获取向量数据
	vector, err := h.vectorService.GetVectorFromEcho(baseData)
	if err != nil {
		log.Println("获取向量数据失败", err)
		return "", err
	}
	log.Println("获取向量数据成功", vector)
	// todo 将向量数据存储到数据库中，关联文件URL等信息
	return url, nil
}

// getPrivateURL 生成七牛云私有空间临时访问链接
func (h *FileService) GetPrivateURL(key string, expiresInSeconds int64) (string, error) {
	// 检查配置
	if err := checkConfig(); err != nil {
		log.Println(err)
		return "", err
	}

	mac := auth.New(accessKey, secretKey)
	// 构建私有空间访问 URL
	// 注意：如果使用了 CDN 域名且 CDN 开启了防盗链，可能需要额外处理
	// 这里使用最简单的私有空间签名方法
	//urlToSign := fmt.Sprintf("%s,%s", domain, key)

	deadline := time.Now().Add(time.Duration(expiresInSeconds) * time.Second).Unix()
	privateURL := storage.MakePrivateURL(mac, domain, key, deadline)
	log.Println("七牛云文件url获取成功：", privateURL)
	return privateURL, nil
}

// getPublicURL 生成七牛云公开空间访问链接
func (h *FileService) GetPublicURL(key string) (string, error) {
	// 检查配置
	if err := checkConfig(); err != nil {
		log.Println(err)
		return "", err
	}

	publicURL := storage.MakePublicURL(domain, key)
	log.Println("七牛云文件url获取成功：", publicURL)
	return publicURL + ".png", nil
}

func convertFileForBase64(url string) ([]byte, error) {
	// 发起 HTTP GET 请求
	resp, err := http.Get("http://" + url)
	if err != nil {
		stack := debug.Stack()
		log.Printf("Error: %v\nStack Trace:\n%s", err, stack)
		return nil, err
	}
	defer resp.Body.Close() // 确保响应体被关闭

	// 检查 HTTP 状态码
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("下载失败：HTTP %s", resp.Status)
	}

	// 读取响应体到字节切片
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return data, nil
}
