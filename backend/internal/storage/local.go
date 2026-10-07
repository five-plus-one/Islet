package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Local 本地磁盘驱动：写 {root}/{key}，经公网端口 /i/{key} 流式访问。
type Local struct {
	root     string
	baseURL  func() string // site.public_base_url，运行时可变
	fallback string        // baseURL 为空时的占位（访问由 /i 动态处理）
}

func NewLocal(root string, baseURL func() string) *Local {
	return &Local{root: root, baseURL: baseURL}
}

func (d *Local) file(key string) (string, error) {
	if !SafeKey(key) {
		return "", ErrStorage
	}
	target := filepath.Join(d.root, filepath.FromSlash(key))
	absRoot, err := filepath.Abs(d.root)
	if err != nil {
		return "", err
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	if absTarget != absRoot && !strings.HasPrefix(absTarget, absRoot+string(os.PathSeparator)) {
		return "", ErrStorage
	}
	return absTarget, nil
}

func (d *Local) Put(_ context.Context, key string, r io.Reader, _ int64, _ string) (string, error) {
	target, err := d.file(key)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return "", err
	}
	f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(f, r); err != nil {
		return "", err
	}
	return d.imageURL(key), nil
}

func (d *Local) Delete(_ context.Context, key string) error {
	target, err := d.file(key)
	if err != nil {
		return err
	}
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (d *Local) Access() AccessMode { return AccessLocal }

func (d *Local) Read(_ context.Context, key string) (io.ReadCloser, int64, error) {
	target, err := d.file(key)
	if err != nil {
		return nil, 0, err
	}
	f, err := os.Open(target)
	if err != nil {
		return nil, 0, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, 0, err
	}
	return f, info.Size(), nil
}

func (d *Local) RedirectURL(key string) string { return d.imageURL(key) }

func (d *Local) Check(_ context.Context) error {
	return os.MkdirAll(d.root, 0700)
}

// imageURL 本地图片的对外 URL。baseURL 未配置时返回相对路径（访问端动态补全）。
func (d *Local) imageURL(key string) string {
	base := ""
	if d.baseURL != nil {
		base = strings.TrimRight(strings.TrimSpace(d.baseURL()), "/")
	}
	if base == "" {
		return "/i/" + key
	}
	if _, err := url.Parse(base); err != nil {
		return "/i/" + key
	}
	return fmt.Sprintf("%s/i/%s", base, key)
}

// RandomName 生成 {n 位随机 hex}{ext}。
func RandomName(ext string, n int) string {
	raw := make([]byte, n/2)
	if _, err := rand.Read(raw); err != nil {
		return ""
	}
	return hex.EncodeToString(raw) + ext
}
