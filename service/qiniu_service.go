package service

import (
	"context"
	"echo-core/utils"
	"fmt"
	"strings"

	"github.com/qiniu/go-sdk/v7/auth"
	"github.com/qiniu/go-sdk/v7/storage"
)

// 七牛云对象删除能力（供回忆记忆删除链路使用）。
// 复用 getQiniuConfig（见 file_service.go）读取 AK/SK/bucket。

// newBucketManager 构造七牛 BucketManager
func newBucketManager() (*storage.BucketManager, string, error) {
	accessKey, secretKey, bucket, _, err := getQiniuConfig()
	if err != nil {
		return nil, "", err
	}
	mac := auth.New(accessKey, secretKey)
	cfg := &storage.Config{UseHTTPS: true}
	return storage.NewBucketManager(mac, cfg), bucket, nil
}

// DeleteObject 删除单个对象
func DeleteObject(ctx context.Context, key string) error {
	if strings.TrimSpace(key) == "" {
		return nil
	}
	bm, bucket, err := newBucketManager()
	if err != nil {
		utils.LogWithCtx(ctx, "Qiniu.DeleteObject", "配置检查失败 | err=%v", err)
		return err
	}
	if err := bm.Delete(bucket, key); err != nil {
		utils.LogWithCtx(ctx, "Qiniu.DeleteObject", "删除失败 | key=%s err=%v", key, err)
		return fmt.Errorf("删除对象失败: %w", err)
	}
	utils.LogWithCtx(ctx, "Qiniu.DeleteObject", "删除成功 | key=%s", key)
	return nil
}

// DeleteByPrefix 按前缀列出并批量删除对象（用于删除整个记忆主题目录）。
// 逐页拉取 marker，最多 1000/页，累计批量删除。
func DeleteByPrefix(ctx context.Context, prefix string) error {
	if strings.TrimSpace(prefix) == "" {
		return fmt.Errorf("prefix 不能为空")
	}
	bm, bucket, err := newBucketManager()
	if err != nil {
		utils.LogWithCtx(ctx, "Qiniu.DeleteByPrefix", "配置检查失败 | err=%v", err)
		return err
	}

	var keys []string
	marker := ""
	for {
		entries, _, nextMarker, hasNext, listErr := bm.ListFiles(bucket, prefix, "", marker, 1000)
		if listErr != nil {
			utils.LogWithCtx(ctx, "Qiniu.DeleteByPrefix", "列举失败 | prefix=%s err=%v", prefix, listErr)
			return fmt.Errorf("列举对象失败: %w", listErr)
		}
		for _, e := range entries {
			keys = append(keys, e.Key)
		}
		if !hasNext {
			break
		}
		marker = nextMarker
	}

	if len(keys) == 0 {
		utils.LogWithCtx(ctx, "Qiniu.DeleteByPrefix", "无对象需删除 | prefix=%s", prefix)
		return nil
	}

	// 分批 batch delete（七牛单次批量上限 1000）
	for i := 0; i < len(keys); i += 1000 {
		end := i + 1000
		if end > len(keys) {
			end = len(keys)
		}
		ops := make([]string, 0, end-i)
		for _, k := range keys[i:end] {
			ops = append(ops, storage.URIDelete(bucket, k))
		}
		if _, batchErr := bm.Batch(ops); batchErr != nil {
			utils.LogWithCtx(ctx, "Qiniu.DeleteByPrefix", "批量删除失败 | prefix=%s batch=[%d,%d) err=%v", prefix, i, end, batchErr)
			return fmt.Errorf("批量删除对象失败: %w", batchErr)
		}
	}
	utils.LogWithCtx(ctx, "Qiniu.DeleteByPrefix", "删除成功 | prefix=%s total=%d", prefix, len(keys))
	return nil
}
