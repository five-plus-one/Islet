package storage

import (
	"context"
	"errors"
	"io"
	"path"
	"strings"
)

var ErrStorage = errors.New("图片存储操作失败，请检查配置和存储权限")

// AccessMode 图片访问方式：决定 /i/{key} 的行为。
type AccessMode int

const (
	AccessLocal    AccessMode = iota // 从应用本地磁盘流式返回
	AccessRedirect                   // 302 到对象存储直链
	AccessProxy                      // 应用回源读取并转发
)

// Driver 存储驱动接口。策略（storage_policies）是驱动的具名实例。
type Driver interface {
	// Put 写入对象并返回固化到图片记录的最终 URL。
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) (string, error)
	Delete(ctx context.Context, key string) error
	Access() AccessMode
	// Read AccessLocal / AccessProxy 时读取对象内容。
	Read(ctx context.Context, key string) (io.ReadCloser, int64, error)
	// RedirectURL AccessRedirect 时的 302 目标。
	RedirectURL(key string) string
	// Check 连通性检测：上传探测对象并回读校验。
	Check(ctx context.Context) error
}

// SafeKey 校验存储 key：禁止路径穿越与绝对路径成分。
func SafeKey(key string) bool {
	if key == "" || strings.Contains(key, "\\") || strings.Contains(key, "..") {
		return false
	}
	if strings.HasPrefix(key, "/") {
		return false
	}
	clean := path.Clean(key)
	return clean == key && len(key) <= 512
}
