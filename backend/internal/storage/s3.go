package storage

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// S3Config S3 兼容驱动配置（加密前的明文结构）。
type S3Config struct {
	Endpoint  string `json:"endpoint"`
	Region    string `json:"region"`
	Bucket    string `json:"bucket"`
	Prefix    string `json:"prefix"`
	CDN       string `json:"cdn"`
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key,omitempty"`
	PathStyle bool   `json:"path_style"`
}

// S3 S3 兼容对象存储驱动（阿里云 OSS / 腾讯 COS / MinIO / R2 等）。
type S3 struct {
	cfg   S3Config
	base  func() string // site.public_base_url（proxy 模式 URL 用）
	proxy bool
}

func NewS3(cfg S3Config, baseURL func() string) *S3 {
	return &S3{cfg: normalizeS3(cfg), base: baseURL}
}

func (d *S3) SetProxy(proxy bool) { d.proxy = proxy }

func normalizeS3(c S3Config) S3Config {
	c.Endpoint = strings.TrimRight(strings.TrimSpace(c.Endpoint), "/")
	c.Region = strings.TrimSpace(c.Region)
	c.Bucket = strings.TrimSpace(c.Bucket)
	c.CDN = strings.TrimRight(strings.TrimSpace(c.CDN), "/")
	c.Prefix = strings.Trim(strings.TrimSpace(c.Prefix), "/")
	u, err := url.Parse(c.Endpoint)
	if err == nil && strings.HasSuffix(u.Hostname(), ".aliyuncs.com") {
		host := u.Hostname()
		if strings.HasPrefix(host, "oss-") && !strings.HasPrefix(host, "s3.") {
			u.Host = "s3." + host
			c.Endpoint = u.String()
		}
		if strings.HasPrefix(u.Hostname(), "s3.oss-") {
			c.PathStyle = false
			c.Region = strings.TrimPrefix(c.Region, "oss-")
		}
	}
	return c
}

// Validate 校验配置；endpoint 为 OSS 时从 host 推导并校验 Region。
func ValidateS3(c S3Config) error {
	c = normalizeS3(c)
	for _, raw := range []string{c.Endpoint, c.CDN} {
		if raw == "" {
			continue
		}
		u, e := url.Parse(raw)
		if e != nil || u.Host == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") || u.RawQuery != "" || u.Fragment != "" {
			return errorsNew("Endpoint 与 CDN 需为合法的 http(s) 地址，且不含查询参数")
		}
	}
	u, _ := url.Parse(c.Endpoint)
	if u == nil || u.Host == "" {
		return errorsNew("Endpoint 不能为空")
	}
	if strings.HasPrefix(u.Hostname(), "s3.oss-") && strings.HasSuffix(u.Hostname(), ".aliyuncs.com") {
		region := strings.TrimSuffix(strings.TrimPrefix(u.Hostname(), "s3.oss-"), ".aliyuncs.com")
		region = strings.TrimSuffix(region, "-internal")
		if region != "accelerate" && c.Region != region {
			return fmt.Errorf("地域应为 %s，请检查 Region 与存储桶所在地", region)
		}
	}
	if strings.Trim(u.Path, "/") != "" {
		return errorsNew("Endpoint 不应包含路径，路径请填写到 Prefix")
	}
	if c.Bucket == "" || strings.ContainsAny(c.Bucket, "/\\ .") {
		return errorsNew("Bucket 名称不合法")
	}
	if c.Region == "" {
		return errorsNew("Region 不能为空")
	}
	if c.AccessKey == "" || c.SecretKey == "" {
		return errorsNew("Access Key 与 Secret Key 不能为空")
	}
	if strings.Contains(c.Prefix, "..") || strings.ContainsAny(c.Prefix, "\\?#") {
		return errorsNew("Prefix 不合法")
	}
	return nil
}

func (d *S3) client() (*minio.Client, error) {
	u, err := url.Parse(d.cfg.Endpoint)
	if err != nil || u.Host == "" {
		return nil, ErrStorage
	}
	lookup := minio.BucketLookupDNS
	if d.cfg.PathStyle {
		lookup = minio.BucketLookupPath
	}
	return minio.New(u.Host, &minio.Options{
		Creds:        credentials.NewStaticV4(d.cfg.AccessKey, d.cfg.SecretKey, ""),
		Secure:       u.Scheme == "https",
		Region:       d.cfg.Region,
		BucketLookup: lookup,
	})
}

func (d *S3) objectKey(key string) string {
	return path.Join(d.cfg.Prefix, key)
}

func (d *S3) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) (string, error) {
	client, err := d.client()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	options := minio.PutObjectOptions{ContentType: contentType, DisableMultipart: true, DisableContentSha256: true, SendContentMd5: true}
	options.SetMatchETagExcept("*")
	_, err = client.PutObject(ctx, d.cfg.Bucket, d.objectKey(key), r, size, options)
	if err != nil {
		return "", TranslateError(err)
	}
	return d.publicURL(key), nil
}

func (d *S3) Delete(ctx context.Context, key string) error {
	client, err := d.client()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := client.RemoveObject(ctx, d.cfg.Bucket, d.objectKey(key), minio.RemoveObjectOptions{}); err != nil {
		return TranslateError(err)
	}
	return nil
}

func (d *S3) Access() AccessMode {
	if !d.proxy {
		return AccessRedirect
	}
	return AccessProxy
}

func (d *S3) Read(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	client, err := d.client()
	if err != nil {
		return nil, 0, err
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	obj, err := client.GetObject(ctx, d.cfg.Bucket, d.objectKey(key), minio.GetObjectOptions{})
	if err != nil {
		cancel()
		return nil, 0, TranslateError(err)
	}
	stat, err := obj.Stat()
	if err != nil {
		_ = obj.Close()
		cancel()
		return nil, 0, TranslateError(err)
	}
	return &cancelReader{ReadCloser: obj, cancel: cancel}, stat.Size, nil
}

type cancelReader struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (r *cancelReader) Close() error { defer r.cancel(); return r.ReadCloser.Close() }

func (d *S3) RedirectURL(key string) string {
	return d.publicURL(key)
}

// publicURL 固化到图片记录的直链：CDN 优先，否则 endpoint + bucket。
func (d *S3) publicURL(key string) string {
	if d.proxy {
		base := ""
		if d.base != nil {
			base = strings.TrimRight(d.base(), "/")
		}
		return base + "/i/" + key
	}
	if d.cfg.CDN != "" {
		return d.cfg.CDN + "/" + d.objectKey(key)
	}
	u, err := url.Parse(d.cfg.Endpoint)
	if err != nil || u.Host == "" {
		return "/i/" + key
	}
	if d.cfg.PathStyle {
		scheme := "https"
		if u.Scheme == "http" {
			scheme = "http"
		}
		return fmt.Sprintf("%s://%s/%s/%s", scheme, u.Host, d.cfg.Bucket, d.objectKey(key))
	}
	// 虚拟主机风格：{bucket}.{endpoint-host}
	scheme := "https"
	if u.Scheme == "http" {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s.%s/%s", scheme, d.cfg.Bucket, u.Host, d.objectKey(key))
}

func (d *S3) Check(ctx context.Context) error {
	client, err := d.client()
	if err != nil {
		return err
	}
	// 探测图：64x64 纯色 PNG
	png := buildProbePNG()
	name := ".islet-check/" + probeName() + ".png"
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if _, err = client.PutObject(ctx, d.cfg.Bucket, d.objectKey(name), bytes.NewReader(png), int64(len(png)),
		minio.PutObjectOptions{ContentType: "image/png", DisableMultipart: true, DisableContentSha256: true, SendContentMd5: true}); err != nil {
		return TranslateError(err)
	}
	if err := client.RemoveObject(ctx, d.cfg.Bucket, d.objectKey(name), minio.RemoveObjectOptions{}); err != nil {
		return TranslateError(err)
	}
	return nil
}

func probeName() string {
	raw := make([]byte, 8)
	if _, err := cryptoRandRead(raw); err != nil {
		return "probe"
	}
	return hex.EncodeToString(raw)
}

// TranslateError 把 minio/S3 错误翻译为可操作的中文提示。
func TranslateError(err error) error {
	r := minio.ToErrorResponse(err)
	switch r.Code {
	case "PreconditionFailed":
		return errorsNew("文件名已被使用，请重新上传")
	case "NoSuchBucket":
		return errorsNew("存储桶不存在，请检查 Bucket 名称和地域")
	case "InvalidAccessKeyId":
		return errorsNew("访问密钥 ID 无效，请检查 Access Key ID")
	case "AccessDenied":
		return errorsNew("存储桶拒绝写入，请检查访问密钥的上传/删除权限")
	case "SignatureDoesNotMatch", "InvalidSecurity":
		return errorsNew("签名校验失败，请检查密钥、地域与 Endpoint")
	case "AuthorizationHeaderMalformed", "PermanentRedirect", "IncorrectEndpoint":
		return errorsNew("存储地址或地域不匹配，请检查 Endpoint 和 Region")
	case "NoSuchKey":
		return errorsNew("对象不存在")
	case "RequestTimeTooSkewed":
		return errorsNew("服务器时间偏差过大，请同步系统时间")
	}
	var netErr net.Error
	if errorsAs(err, &netErr) {
		return errorsNew("无法连接存储服务，请检查地址与网络")
	}
	return ErrStorage
}

// BuildS3Driver 按明文配置构造驱动（PolicyService 解密后调用）。
func BuildS3Driver(cfg S3Config, baseURL func() string) (*S3, error) {
	if err := ValidateS3(cfg); err != nil {
		return nil, err
	}
	return NewS3(cfg, baseURL), nil
}
