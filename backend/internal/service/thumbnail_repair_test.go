package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"image"
	"image/jpeg"
	"io"
	"islet/backend/internal/model"
	"islet/backend/internal/storage"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCDNOrientationRepair(t *testing.T) {
	dsn := os.Getenv("ISLET_CDN_TEST_DSN")
	if dsn == "" {
		t.Skip("requires isolated MySQL test database")
	}
	if !strings.Contains(dsn, "/islet_cdn_jobs_test?") {
		t.Fatal("dedicated database required")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(model.AllModels()...); err != nil {
		t.Fatal(err)
	}
	var encoded bytes.Buffer
	jpeg.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 80, 48)), nil)
	original := orientationJPEG(encoded.Bytes(), 6, binary.LittleEndian)
	sum := sha256.Sum256(original)
	var mu sync.Mutex
	objects := map[string][]byte{"/test-bucket/original.jpg": original, "/test-bucket/old.jpg": []byte("preserved preview")}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Method == "PUT" {
			b, _ := io.ReadAll(r.Body)
			objects[r.URL.Path] = b
			w.Header().Set("ETag", `"test"`)
			return
		}
		b, ok := objects[r.URL.Path]
		if !ok {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(404)
			io.WriteString(w, "<Error><Code>NoSuchKey</Code></Error>")
			return
		}
		w.Header().Set("Content-Length", fmt.Sprint(len(b)))
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		w.Header().Set("ETag", `"test"`)
		if r.Method != "HEAD" {
			w.Write(b)
		}
	}))
	defer server.Close()
	box := NewSecretBox(t.TempDir())
	payload := S3Payload{S3Config: storage.S3Config{Endpoint: server.URL, Region: "us-east-1", Bucket: "test-bucket", AccessKey: "test", SecretKey: "test", CDN: "https://cdn.example.com", PathStyle: true}, CDNRefresh: true}
	plain, _ := json.Marshal(payload)
	sealed, _ := box.Seal(string(plain))
	policy := model.StoragePolicy{Name: fmt.Sprintf("orientation-%d", time.Now().UnixNano()), Driver: model.PolicyDriverS3, Config: sealed, Status: model.StatusActive, URLMode: model.PolicyURLProxy}
	if err = db.Create(&policy).Error; err != nil {
		t.Fatal(err)
	}
	old := "old.jpg"
	oldURL := "https://cdn.example.com/old.jpg"
	item := model.Image{UserID: 1, PolicyID: policy.ID, StorageKey: "original.jpg", OriginalName: "camera.jpg", SHA256: hex.EncodeToString(sum[:]), Width: 80, Height: 48, ThumbKey: &old, ThumbURL: &oldURL, Blocked: true, BlockReason: "policy"}
	if err = db.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Unscoped().Delete(&model.Image{}, item.ID)
		db.Where("image_id = ?", item.ID).Delete(&model.CDNCacheJob{})
		db.Unscoped().Delete(&model.StoragePolicy{}, policy.ID)
	})
	s := NewImageService(db, NewPolicyService(db, box, func() string { return "https://app.example.com" }, t.TempDir()))
	if needed, e := s.RepairThumbnailOrientation(context.Background(), item.ID, false); e != nil || !needed {
		t.Fatal("preflight", needed, e)
	}
	if len(objects) != 2 {
		t.Fatal("preflight wrote storage")
	}
	if err = db.Exec("CREATE TRIGGER orientation_job_probe BEFORE INSERT ON cdn_cache_jobs FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='orientation test'").Error; err != nil {
		t.Fatal(err)
	}
	_, failed := s.RepairThumbnailOrientation(context.Background(), item.ID, true)
	db.Exec("DROP TRIGGER orientation_job_probe")
	if failed == nil {
		t.Fatal("expected transactional failure")
	}
	unchanged, _ := s.Get(item.ID)
	if stringValue(unchanged.ThumbKey) != old || unchanged.Width != 80 {
		t.Fatal("failed cache enqueue changed image")
	}
	if changed, e := s.RepairThumbnailOrientation(context.Background(), item.ID, true); e != nil || !changed {
		t.Fatal("retry", changed, e)
	}
	repaired, _ := s.Get(item.ID)
	if repaired.Width != 48 || repaired.Height != 80 || !repaired.Blocked || repaired.BlockReason != "policy" || !repaired.CreatedAt.Equal(item.CreatedAt) || repaired.SHA256 != item.SHA256 {
		t.Fatal("repair changed protected fields")
	}
	if changed, e := s.RepairThumbnailOrientation(context.Background(), item.ID, true); e != nil || changed {
		t.Fatal("idempotence", changed, e)
	}
	// Newly uploaded photographs already have corrected conventional previews.
	mu.Lock()
	objects["/test-bucket/current.jpg"] = bytes.Clone(objects["/test-bucket/"+*repaired.ThumbKey])
	mu.Unlock()
	if e := db.Model(&repaired).Update("thumb_key", "current.jpg").Error; e != nil {
		t.Fatal(e)
	}
	if changed, e := s.RepairThumbnailOrientation(context.Background(), item.ID, false); e != nil || changed {
		t.Fatal("correct preview must be preserved", changed, e)
	}
	var jobs []model.CDNCacheJob
	db.Where("image_id = ?", item.ID).Find(&jobs)
	if len(jobs) != 1 || !strings.Contains(strings.Join(jobs[0].URLs, " "), "/legacy/old.jpg") {
		t.Fatal("previous and managed paths must be refreshed")
	}
	if !bytes.Equal(objects["/test-bucket/original.jpg"], original) || string(objects["/test-bucket/old.jpg"]) != "preserved preview" {
		t.Fatal("original or previous preview changed")
	}
}
