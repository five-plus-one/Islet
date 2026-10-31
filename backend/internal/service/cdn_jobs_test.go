package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"islet/backend/internal/model"
	"islet/backend/internal/storage"
)

type testCDN struct {
	fail, done  bool
	submissions int
}

func (p *testCDN) refresh(context.Context, []string) (string, error) {
	p.submissions++
	if p.fail {
		return "", cdnError("Forbidden.RAM")
	}
	return "123", nil
}
func (p *testCDN) complete(context.Context, string, []string) (bool, error) { return p.done, nil }

func TestCDNDurableLifecycle(t *testing.T) {
	dsn := os.Getenv("ISLET_CDN_TEST_DSN")
	if dsn == "" {
		t.Skip("requires isolated MySQL test database")
	}
	if !strings.Contains(dsn, "/islet_cdn_jobs_test?") {
		t.Fatal("requires dedicated islet_cdn_jobs_test database")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(model.AllModels()...); err != nil {
		t.Fatal(err)
	}
	db.FirstOrCreate(&model.User{ID: 1, Username: "cdn-test-owner", Role: model.UserRoleSuperAdmin, Status: model.StatusActive}, model.User{ID: 1})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer server.Close()
	box := NewSecretBox(t.TempDir())
	payload := S3Payload{S3Config: storage.S3Config{Endpoint: server.URL, Region: "us-east-1", Bucket: "test-bucket", AccessKey: "test-key", SecretKey: "test-secret", CDN: "https://cdn.example.com", PathStyle: true}, CDNRefresh: true}
	plain, _ := json.Marshal(payload)
	sealed, err := box.Seal(string(plain))
	if err != nil {
		t.Fatal(err)
	}
	policy := model.StoragePolicy{Name: fmt.Sprintf("cdn-test-%d", time.Now().UnixNano()), Driver: model.PolicyDriverS3, Config: sealed, URLMode: model.PolicyURLProxy, Naming: model.NamingDate, Status: model.StatusActive}
	if err = db.Create(&policy).Error; err != nil {
		t.Fatal(err)
	}
	image := model.Image{UserID: 1, PolicyID: policy.ID, StorageKey: fmt.Sprintf("cdn-test/%d.png", time.Now().UnixNano()), URL: "https://cdn.example.com/historical.png", OriginalName: "CDN test", Mime: "image/png", SHA256: "cdn-test"}
	if err = db.Create(&image).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Unscoped().Delete(&model.Image{}, image.ID)
		db.Where("image_id = ?", image.ID).Delete(&model.CDNCacheJob{})
		db.Where("target_type = ? AND target_id = ?", "image", fmt.Sprint(image.ID)).Delete(&model.AuditLog{})
		db.Unscoped().Delete(&model.StoragePolicy{}, policy.ID)
	})
	policies := NewPolicyService(db, box, func() string { return "https://app.example.com" }, t.TempDir())
	provider := &testCDN{fail: true}
	makeService := func() *ImageService {
		s := NewImageService(db, policies)
		s.cdnClient = func(string, string) cdnProvider { return provider }
		return s
	}
	s := makeService()
	if err = db.Exec("CREATE TRIGGER cdn_job_insert_probe BEFORE INSERT ON cdn_cache_jobs FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='isolated CDN job persistence failure'").Error; err != nil {
		t.Fatal(err)
	}
	_, rollbackErr := s.Moderate(image.ID, 1, true, "rollback check", "")
	db.Exec("DROP TRIGGER cdn_job_insert_probe")
	if rollbackErr == nil {
		t.Fatal("task persistence failure must reject moderation")
	}
	var unchanged model.Image
	db.First(&unchanged, image.ID)
	if unchanged.Blocked || unchanged.ModeratedAt != nil {
		t.Fatal("task failure must roll back image moderation")
	}
	var audits int64
	db.Model(&model.AuditLog{}).Where("target_type = ? AND target_id = ?", "image", fmt.Sprint(image.ID)).Count(&audits)
	if audits != 0 {
		t.Fatal("task failure must roll back audit")
	}
	if _, err = s.Moderate(image.ID, 1, true, "policy review", ""); err != nil {
		t.Fatal(err)
	}
	var job model.CDNCacheJob
	if err = db.Where("image_id = ?", image.ID).First(&job).Error; err != nil {
		t.Fatal(err)
	}
	if len(job.URLs) < 6 || s.ItemView(image).URL != "https://cdn.example.com/i/"+image.StorageKey {
		t.Fatal("managed and historical CDN URLs required")
	}
	due := func() {
		db.Model(&model.CDNCacheJob{}).Where("image_id = ? AND status <> ?", image.ID, "complete").Update("next_attempt_at", time.Now().Add(-time.Minute))
	}
	if err = s.SyncCDN(context.Background()); err != nil {
		t.Fatal(err)
	}
	db.First(&job, job.ID)
	if job.Attempts != 1 || job.LastError != "Forbidden.RAM" || job.NextAttemptAt.Before(time.Now()) {
		t.Fatal("failed submission must persist a delayed retry")
	}
	provider.fail = false
	s = makeService()
	due()
	if err = s.SyncCDN(context.Background()); err != nil {
		t.Fatal(err)
	}
	db.First(&job, job.ID)
	if job.Status != "submitted" || job.TaskIDs != "123" || job.SubmittedAt == nil {
		t.Fatal("restart must resume persisted submission")
	}
	due()
	_ = s.SyncCDN(context.Background())
	if s.CDNStatus(image.ID) != "pending" {
		t.Fatal("provider pending must remain pending")
	}
	provider.done = true
	due()
	_ = s.SyncCDN(context.Background())
	if s.CDNStatus(image.ID) != "complete" {
		t.Fatal("provider completion required")
	}
	if err = s.Recycle(image.ID, 1, true); err != nil {
		t.Fatal(err)
	}
	if _, err = s.GetByKey(image.StorageKey); err != ErrImageNotFound {
		t.Fatal("recycled image must be inaccessible")
	}
	if err = s.Restore(image.ID, 1, true, nil); err != nil {
		t.Fatal(err)
	}
	restored, _ := s.Get(image.ID)
	if !restored.Blocked {
		t.Fatal("recycle restore must preserve moderation")
	}
	if err = s.Recycle(image.ID, 1, true); err != nil {
		t.Fatal(err)
	}
	if err = s.Purge(context.Background(), image.ID, 1, true); err != nil {
		t.Fatal(err)
	}
	var count int64
	db.Model(&model.CDNCacheJob{}).Where("image_id = ?", image.ID).Count(&count)
	if count != 5 {
		t.Fatalf("expected durable jobs after image purge, got %d", count)
	}
	if _, err = s.Get(image.ID); err != ErrImageNotFound {
		t.Fatal("image purge did not remove record")
	}
	if err = policies.Delete(policy.ID); err == nil {
		t.Fatal("pending jobs must protect their policy")
	}
	due()
	_ = s.SyncCDN(context.Background())
	due()
	_ = s.SyncCDN(context.Background())
	due()
	_ = s.SyncCDN(context.Background())
	due()
	_ = s.SyncCDN(context.Background())
	if s.CDNStatus(image.ID) != "complete" {
		t.Fatal("purged image cache jobs did not complete")
	}
}
