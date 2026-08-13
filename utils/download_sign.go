package utils

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"time"
)

// DownloadExpirySeconds 下载签名 URL 的过期时间（秒）。
// 改造要求：60 秒。测试可通过包内变量临时改写。
var DownloadExpirySeconds int64 = 60

// DownloadSignSecret 读签名密钥。
//
// 必须从环境变量 DOWNLOAD_SIGN_SECRET 注入，缺失即返回错误（由 service 层透传 → 500）。
// 部署时多实例必须使用相同的 secret，否则跨实例的 URL 校验会失败（本次不做跨实例校验，
// 仅作为未来扩展点：若部署多实例 + 边缘层做 IP 校验，secret 必须一致）。
func DownloadSignSecret() ([]byte, error) {
	s := os.Getenv("DOWNLOAD_SIGN_SECRET")
	if s == "" {
		return nil, errors.New("DOWNLOAD_SIGN_SECRET 未配置")
	}
	return []byte(s), nil
}

// NowDeadline 统一返回过期截止的 Unix 时间戳。
func NowDeadline() int64 {
	return time.Now().Unix() + DownloadExpirySeconds
}

// MakeDownloadSign 生成 (key|deadline|ip) 的 HMAC-SHA256 十六进制字符串。
//
// payload 拼接顺序固定：key|deadline|ip；任何调整都必须同步到 MakeSignedDownloadURL 的注释。
// deadline 单位为秒（Unix 时间戳）。
func MakeDownloadSign(secret []byte, key, ip string, deadline int64) string {
	payload := key + "|" + strconv.FormatInt(deadline, 10) + "|" + ip
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

// AppendIPSignature 在七牛私有 URL 上追加 ip_sig=<hmac> 参数。
//
// 七牛私有 URL 形如：
//
//	https://<domain>/<encoded-key>?e=<deadline>&token=<qiniu-mac>
//	https://<domain>/<encoded-key>?e=<deadline>&token=<qiniu-mac>&xcode=...
//
// 我们保留原 query 顺序、值原样，仅追加 ip_sig。
//
// 注意：Qiniu CDN 不会校验 ip_sig；它只为应用层审计与未来边缘层校验留出 hook。
func AppendIPSignature(privateURL, ipSig string) (string, error) {
	u, err := url.Parse(privateURL)
	if err != nil {
		return "", fmt.Errorf("parse private url: %w", err)
	}
	q := u.Query()
	q.Set("ip_sig", ipSig)
	u.RawQuery = q.Encode()
	return u.String(), nil
}
