package service

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
)

// parseDeadlineFromQiniuURL 从七牛私有 URL 中提取 ?e=<deadline> 参数。
//
// 七牛私有 URL 形如：
//
//	https://<domain>/<encoded-key>?e=<deadline>&token=<hmac>
//
// 返回值为 Unix 秒级时间戳。
func parseDeadlineFromQiniuURL(rawURL string) (int64, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return 0, err
	}
	eStr := strings.TrimSpace(u.Query().Get("e"))
	if eStr == "" {
		return 0, errors.New("URL 中未找到 ?e= 参数")
	}
	deadline, err := strconv.ParseInt(eStr, 10, 64)
	if err != nil {
		return 0, err
	}
	if deadline <= 0 {
		return 0, errors.New("deadline 必须 > 0")
	}
	return deadline, nil
}
