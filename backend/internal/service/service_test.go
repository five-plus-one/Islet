package service

import (
	"path/filepath"
	"strings"
	"testing"

	"islet/backend/internal/storage"
)

func TestSecretBoxRoundTrip(t *testing.T) {
	dir := t.TempDir()
	box := NewSecretBox(dir)
	plain := `{"endpoint":"https://s3.example.com","secret":"abc123"}`
	sealed, err := box.Seal(plain)
	if err != nil {
		t.Fatal(err)
	}
	if sealed == plain || strings.Contains(sealed, "abc123") {
		t.Fatal("ciphertext must not leak plaintext")
	}
	opened, err := box.Open(sealed)
	if err != nil || opened != plain {
		t.Fatalf("round trip failed: %v", err)
	}
	// 篡改后失败
	if _, err := box.Open(sealed[:len(sealed)-4] + "AAAA"); err == nil {
		t.Fatal("tampered ciphertext should fail")
	}
	// 新 box 实例（同目录）可解
	box2 := NewSecretBox(filepath.Join(dir, "sub", ".."))
	if _, err := box2.Open(sealed); err != nil {
		t.Fatalf("same key dir should decrypt: %v", err)
	}
	// 不同目录的 key 解不开
	other := NewSecretBox(t.TempDir())
	if _, err := other.Open(sealed); err == nil {
		t.Fatal("different key should not decrypt")
	}
}

func TestMimeAllowed(t *testing.T) {
	if !mimeAllowed(nil, "image/png") {
		t.Fatal("empty whitelist allows all images")
	}
	if mimeAllowed(nil, "text/plain") {
		t.Fatal("non-image must be rejected")
	}
	if !mimeAllowed([]string{"image/*"}, "image/jpeg") {
		t.Fatal("wildcard should match")
	}
	if !mimeAllowed([]string{"image/png", "image/webp"}, "image/webp") {
		t.Fatal("explicit match")
	}
	if mimeAllowed([]string{"image/png"}, "image/jpeg") {
		t.Fatal("non-listed should be rejected")
	}
}

func TestSanitizeBaseName(t *testing.T) {
	if got := sanitizeBaseName("我的照片.png"); got != "我的照片" {
		t.Fatalf("unicode keep: %q", got)
	}
	if got := sanitizeBaseName("hello world.jpg"); got != "hello world" {
		t.Fatalf("ascii keep: %q", got)
	}
	if got := sanitizeBaseName("..\\evil"); got != "evil" {
		t.Fatalf("path stripped: %q", got)
	}
	if got := sanitizeBaseName(""); got != "image" {
		t.Fatalf("fallback: %q", got)
	}
}

func TestNormalizeTags(t *testing.T) {
	tags := normalizeTags([]string{" b ", "a", "b", "", "a"})
	if len(tags) != 2 || tags[0] != "a" || tags[1] != "b" {
		t.Fatalf("unexpected tags: %v", tags)
	}
}

func TestValidateS3(t *testing.T) {
	mk := func(endpoint, region, bucket, ak, sk string) storage.S3Config {
		return storage.S3Config{Endpoint: endpoint, Region: region, Bucket: bucket, AccessKey: ak, SecretKey: sk}
	}
	// OSS endpoint 归一化 + region 校验
	if err := storage.ValidateS3(mk("https://oss-cn-hangzhou.aliyuncs.com", "cn-hangzhou", "bkt", "ak", "sk")); err != nil {
		t.Fatalf("oss config should pass: %v", err)
	}
	if err := storage.ValidateS3(mk("https://oss-cn-hangzhou.aliyuncs.com", "cn-beijing", "bkt", "ak", "sk")); err == nil {
		t.Fatal("region mismatch should fail")
	}
	if err := storage.ValidateS3(mk("https://s3.us-east-1.amazonaws.com", "us-east-1", "bad bucket", "ak", "sk")); err == nil {
		t.Fatal("bad bucket should fail")
	}
	if err := storage.ValidateS3(mk("https://s3.example.com", "r1", "bkt", "ak", "")); err == nil {
		t.Fatal("missing secret should fail")
	}
}
