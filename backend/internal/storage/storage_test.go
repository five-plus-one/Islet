package storage

import (
	"strings"
	"testing"
)

func TestSafeKey(t *testing.T) {
	for _, ok := range []string{"abc.png", "2026/01/02/abc.png", "a-b_c.png", "x.thumb.jpg"} {
		if !SafeKey(ok) {
			t.Fatalf("expected valid: %s", ok)
		}
	}
	for _, bad := range []string{"", "..", "a/../b", "/abs", "a\\b", "a//b", strings.Repeat("k", 600)} {
		if SafeKey(bad) {
			t.Fatalf("expected invalid: %q", bad)
		}
	}
}

func TestLocalDriverRoundTrip(t *testing.T) {
	dir := t.TempDir()
	base := "http://cdn.example.com"
	driver := NewLocal(dir, func() string { return base })
	url, err := driver.Put(nil, "2026/01/02/test.png", strings.NewReader("hello"), 5, "image/png")
	if err != nil {
		t.Fatal(err)
	}
	if url != base+"/i/2026/01/02/test.png" {
		t.Fatalf("unexpected url: %s", url)
	}
	if _, err := driver.Put(nil, "2026/01/02/test.png", strings.NewReader("replacement"), 11, "image/png"); err == nil {
		t.Fatal("an existing image must not be overwritten")
	}
	rc, size, err := driver.Read(nil, "2026/01/02/test.png")
	if err != nil || size != 5 {
		t.Fatalf("read failed: %v %d", err, size)
	}
	_ = rc.Close()
	if err := driver.Delete(nil, "2026/01/02/test.png"); err != nil {
		t.Fatal(err)
	}
	// 路径穿越拒绝
	if _, _, err := driver.Read(nil, "../../etc/passwd"); err == nil {
		t.Fatal("traversal should be rejected")
	}
}

func TestNormalizeS3(t *testing.T) {
	cfg := normalizeS3(S3Config{Endpoint: "https://oss-cn-hangzhou.aliyuncs.com/", Region: "oss-cn-hangzhou", Bucket: "b", PathStyle: true})
	if !strings.HasPrefix(cfg.Endpoint, "https://s3.oss-cn-hangzhou") {
		t.Fatalf("endpoint not normalized: %s", cfg.Endpoint)
	}
	if cfg.PathStyle {
		t.Fatal("oss should use virtual-host style")
	}
	if cfg.Region != "cn-hangzhou" {
		t.Fatalf("region not normalized: %s", cfg.Region)
	}
}

func TestS3PublicURL(t *testing.T) {
	driver := NewS3(S3Config{
		Endpoint: "https://s3.oss-cn-hangzhou.aliyuncs.com", Region: "cn-hangzhou",
		Bucket: "bkt", Prefix: "islet", CDN: "https://cdn.example.com",
	}, func() string { return "" })
	if got := driver.RedirectURL("a/b.png"); got != "https://cdn.example.com/islet/a/b.png" {
		t.Fatalf("cdn url: %s", got)
	}
	driver2 := NewS3(S3Config{
		Endpoint: "https://s3.oss-cn-hangzhou.aliyuncs.com", Region: "cn-hangzhou",
		Bucket: "bkt", Prefix: "",
	}, func() string { return "" })
	if got := driver2.RedirectURL("a.png"); got != "https://bkt.s3.oss-cn-hangzhou.aliyuncs.com/a.png" {
		t.Fatalf("virtual-host url: %s", got)
	}
}
