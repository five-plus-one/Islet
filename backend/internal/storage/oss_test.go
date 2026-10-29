package storage

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

func TestOSSCreateOnlyRequest(t *testing.T) {
	cfg := S3Config{Endpoint: "https://s3.oss-cn-shanghai.aliyuncs.com", Bucket: "images", AccessKey: "example", SecretKey: "test-secret"}
	req, err := ossRequest(context.Background(), cfg, "photos/海岸 1.png", []byte("hello"), "image/png", time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if req.URL.Host != "images.oss-cn-shanghai.aliyuncs.com" || req.URL.Path != "/photos/海岸 1.png" {
		t.Fatal("incorrect native object address")
	}
	if req.Header.Get("x-oss-forbid-overwrite") != "true" || req.Header.Get("If-None-Match") != "" {
		t.Fatal("native create-only protection missing")
	}
	if req.Header.Get("Content-MD5") != "XUFAKrxLKna5cZ2REBfFkg==" {
		t.Fatal("invalid body checksum")
	}
	if !strings.HasPrefix(req.Header.Get("Authorization"), "OSS example:") {
		t.Fatal("missing native authorization")
	}
	data, _ := io.ReadAll(req.Body)
	if string(data) != "hello" || req.ContentLength != 5 {
		t.Fatal("body changed during signing")
	}
}
