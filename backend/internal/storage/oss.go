package storage

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"encoding/base64"
	"encoding/xml"
	"github.com/minio/minio-go/v7"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// OSS exposes atomic create-only writes through its native API.
func ossRequest(ctx context.Context, cfg S3Config, key string, data []byte, contentType string, now time.Time) (*http.Request, error) {
	endpoint, err := url.Parse(cfg.Endpoint)
	if err != nil {
		return nil, ErrStorage
	}
	endpoint.Host = cfg.Bucket + "." + strings.TrimPrefix(endpoint.Host, "s3.")
	endpoint.Path = "/" + key
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint.String(), bytes.NewReader(data))
	if err != nil {
		return nil, ErrStorage
	}
	digest := md5.Sum(data)
	md5Value := base64.StdEncoding.EncodeToString(digest[:])
	date := now.UTC().Format(http.TimeFormat)
	canonical := "PUT\n" + md5Value + "\n" + contentType + "\n" + date + "\nx-oss-forbid-overwrite:true\n/" + cfg.Bucket + "/" + key
	signature := hmac.New(sha1.New, []byte(cfg.SecretKey))
	signature.Write([]byte(canonical))
	request.Header.Set("Content-MD5", md5Value)
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("Date", date)
	request.Header.Set("x-oss-forbid-overwrite", "true")
	request.Header.Set("Authorization", "OSS "+cfg.AccessKey+":"+base64.StdEncoding.EncodeToString(signature.Sum(nil)))
	return request, nil
}

func (d *S3) putOSS(ctx context.Context, key string, reader io.Reader, size int64, contentType string) (string, error) {
	if size < 0 {
		return "", ErrStorage
	}
	data, err := io.ReadAll(io.LimitReader(reader, size+1))
	if err != nil || int64(len(data)) != size {
		return "", ErrStorage
	}
	request, err := ossRequest(ctx, d.cfg, d.objectKey(key), data, contentType, time.Now())
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: 60 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return "", ErrStorage
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var failure struct {
			Code string `xml:"Code"`
		}
		_ = xml.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&failure)
		return "", TranslateError(minio.ErrorResponse{Code: failure.Code, StatusCode: response.StatusCode})
	}
	return d.publicURL(key), nil
}
