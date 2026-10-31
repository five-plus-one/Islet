package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	_ "image/gif"

	_ "golang.org/x/image/webp"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"mime/multipart"

	"islet/backend/internal/model"
	"islet/backend/internal/storage"
)

var (
	ErrUploadUnsupported = errors.New("不支持的文件类型，仅允许图片")
	ErrUploadTooLarge    = errors.New("文件超过大小限制")
	ErrUploadQuota       = errors.New("存储配额不足")
	ErrUploadRateLimited = errors.New("上传过于频繁，请稍后再试")
	ErrUploadMimeBlocked = errors.New("文件类型不在分组允许的格式内")
	ErrGroupDisabled     = errors.New("所属分组已停用，无法上传")
)

const (
	thumbMaxEdge   = 480
	thumbDecodeMax = 32 << 20 // 超过 32MB 的原图跳过缩略图与宽高解析，防解码内存放大
)

// UploadService 上传链路：校验 → 去重 → 缩略图 → 写存储 → 落库。
type UploadService struct {
	db       *gorm.DB
	rdb      *redis.Client
	policies *PolicyService
	groups   *GroupService
	settings *SettingsService
}

func NewUploadService(db *gorm.DB, rdb *redis.Client, policies *PolicyService, groups *GroupService, settings *SettingsService) *UploadService {
	return &UploadService{db: db, rdb: rdb, policies: policies, groups: groups, settings: settings}
}

type UploadInput struct {
	UserID   uint64
	GroupID  uint64 // super_admin 传 0 表示不受分组限制
	IsSuper  bool
	FileName string
	Content  io.Reader
	Declared int64  // 客户端声明的大小，仅作预检
	PolicyID uint64 // 显式指定策略（须在分组白名单内）
	AlbumID  *uint64
	Tags     []string
}

type UploadResult struct {
	Image     model.Image
	Duplicate bool
}

// Upload 单文件完整链路。
func (s *UploadService) Upload(ctx context.Context, in UploadInput) (UploadResult, error) {
	var result UploadResult
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var owner model.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&owner, in.UserID).Error; err != nil {
			return err
		}
		if owner.Status != model.StatusActive {
			return ErrAccountUnavailable
		}
		in.GroupID = owner.GroupID
		in.IsSuper = owner.Role == model.UserRoleSuperAdmin
		copy := *s
		copy.db = tx
		var err error
		result, err = copy.upload(ctx, in)
		return err
	})
	return result, err
}

func (s *UploadService) upload(ctx context.Context, in UploadInput) (UploadResult, error) {
	if in.AlbumID != nil {
		var album model.Album
		if err := s.db.Where("id = ? AND user_id = ?", *in.AlbumID, in.UserID).First(&album).Error; err != nil {
			return UploadResult{}, ErrAlbumNotFound
		}
	}
	// 1. 全量读入（限站点上限）
	limit := s.settings.GetInt64(SettingUploadMaxBytes, 100<<20)
	if limit <= 0 {
		limit = 100 << 20
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, io.LimitReader(in.Content, limit+1)); err != nil {
		return UploadResult{}, err
	}
	data := buf.Bytes()
	if int64(len(data)) > limit {
		return UploadResult{}, fmt.Errorf("%w（站点上限 %s）", ErrUploadTooLarge, humanBytes(limit))
	}
	if len(data) == 0 {
		return UploadResult{}, errors.New("文件为空")
	}

	// 2. 真实类型检测（不信客户端 Content-Type）
	mime := http.DetectContentType(data[:min(512, len(data))])
	if !strings.HasPrefix(mime, "image/") || strings.HasPrefix(mime, "image/svg") {
		return UploadResult{}, ErrUploadUnsupported
	}

	// 3. 分组校验
	group, err := s.groups.Get(in.GroupID)
	if err != nil {
		return UploadResult{}, err
	}
	if group.Status != model.StatusActive {
		return UploadResult{}, ErrGroupDisabled
	}
	if !in.IsSuper {
		if group.MaxFileBytes > 0 && int64(len(data)) > int64(group.MaxFileBytes) {
			return UploadResult{}, fmt.Errorf("%w（分组上限 %s）", ErrUploadTooLarge, humanBytes(int64(group.MaxFileBytes)))
		}
		if !mimeAllowed(group.AllowedMimes, mime) {
			return UploadResult{}, ErrUploadMimeBlocked
		}
		if group.UploadsPerHour > 0 {
			if err := s.checkRate(ctx, in.UserID, group.UploadsPerHour); err != nil {
				return UploadResult{}, err
			}
		}
	}

	// 4. 去重（同用户同内容）
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	var existing model.Image
	if err := s.db.Where("user_id = ? AND sha256 = ?", in.UserID, digest).First(&existing).Error; err == nil {
		return UploadResult{Image: existing, Duplicate: true}, nil
	}
	if !in.IsSuper && group.QuotaBytes > 0 {
		used, err := s.usedBytes(in.UserID)
		if err != nil {
			return UploadResult{}, err
		}
		if used+uint64(len(data)) > group.QuotaBytes {
			return UploadResult{}, ErrUploadQuota
		}
	}

	// 5. 选策略
	policyID, err := s.pickPolicyID(in)
	if err != nil {
		return UploadResult{}, err
	}
	driver, policy, err := s.policies.Driver(policyID)
	if err != nil {
		return UploadResult{}, err
	}

	// 6. 生成 key
	ext := strings.ToLower(filepath.Ext(in.FileName))
	if !validImageExt(ext) {
		ext = mimeExt(mime)
	}
	key, err := s.buildKey(ctx, policy.Naming, in.FileName, ext)
	if err != nil {
		return UploadResult{}, err
	}

	// 7. 宽高 + 缩略图（大图跳过）
	width, height := 0, 0
	thumbKey := ""
	var thumbData []byte
	if len(data) <= thumbDecodeMax {
		if cfg, _, err := image.DecodeConfig(bytes.NewReader(data)); err == nil {
			if int64(cfg.Width)*int64(cfg.Height) > 40_000_000 {
				return UploadResult{}, errors.New("图片像素过大，请缩小至 4000 万像素以内")
			}
			width, height = cfg.Width, cfg.Height
		}
		if thumb, ok := buildThumb(data); ok {
			tk := key + ".thumb"
			if ext == ".png" {
				tk = key + ".thumb.png"
			} else {
				tk = key + ".thumb.jpg"
			}
			thumbData = thumb
			if len(thumb) >= 8 && bytes.Equal(thumb[:8], []byte{137, 80, 78, 71, 13, 10, 26, 10}) {
				tk = key + ".thumb.png"
			} else {
				tk = key + ".thumb.jpg"
			}
			thumbKey = tk
		}
	}

	// 8. 写存储
	url, err := driver.Put(ctx, key, bytes.NewReader(data), int64(len(data)), mime)
	if err != nil {
		return UploadResult{}, err
	}
	thumbURL := ""
	if thumbKey != "" {
		tu, err := driver.Put(ctx, thumbKey, bytes.NewReader(thumbData), int64(len(thumbData)), thumbMime(thumbKey))
		if err != nil {
			_ = driver.Delete(ctx, key)
			return UploadResult{}, err
		}
		thumbURL = tu
	}

	// 9. 落库
	base, _ := s.policies.distribution(policy.ID)
	url = base + "/i/" + key
	if thumbKey != "" {
		thumbURL = base + "/t/" + key
	}
	record := model.Image{
		UserID: in.UserID, PolicyID: policyID, AlbumID: in.AlbumID,
		StorageKey: key, URL: url, OriginalName: sanitizeName(in.FileName),
		Mime: mime, Size: uint64(len(data)), Width: width, Height: height,
		SHA256: digest, Tags: normalizeTags(in.Tags),
	}
	if thumbKey != "" {
		record.ThumbKey = &thumbKey
		record.ThumbURL = &thumbURL
	}
	if err := s.db.Create(&record).Error; err != nil {
		_ = driver.Delete(ctx, key)
		if thumbKey != "" {
			_ = driver.Delete(ctx, thumbKey)
		}
		return UploadResult{}, err
	}
	return UploadResult{Image: record}, nil
}

func (s *UploadService) pickPolicyID(in UploadInput) (uint64, error) {
	if in.PolicyID != 0 {
		if !in.IsSuper {
			group, err := s.groups.Get(in.GroupID)
			if err != nil {
				return 0, err
			}
			if len(group.PolicyIDs) > 0 && !containsUint64(group.PolicyIDs, in.PolicyID) {
				return 0, errors.New("该存储策略不在分组可用范围内")
			}
		}
		if _, _, err := s.policies.Driver(in.PolicyID); err != nil {
			return 0, err
		}
		return in.PolicyID, nil
	}
	if in.IsSuper {
		return s.policies.DefaultPolicyID()
	}
	group, err := s.groups.Get(in.GroupID)
	if err != nil {
		return 0, err
	}
	if len(group.PolicyIDs) == 0 {
		return s.policies.DefaultPolicyID()
	}
	// 未指定时取分组白名单中的默认策略，否则取白名单第一个
	def, err := s.policies.DefaultPolicyID()
	if err != nil {
		return 0, err
	}
	for _, id := range group.PolicyIDs {
		if id == def {
			return def, nil
		}
	}
	for _, id := range group.PolicyIDs {
		if _, _, err := s.policies.Driver(id); err == nil {
			return id, nil
		}
	}
	return 0, errors.New("分组可用的存储策略均不可用，请联系管理员")
}

func (s *UploadService) buildKey(ctx context.Context, naming, fileName, ext string) (string, error) {
	switch naming {
	case model.NamingKeep:
		base := sanitizeBaseName(fileName)
		key := base + ext
		var count int64
		if err := s.db.Model(&model.Image{}).Unscoped().Where("storage_key = ?", key).Count(&count).Error; err != nil {
			return "", err
		}
		if count == 0 {
			return key, nil
		}
		for i := 0; i < 5; i++ {
			candidate := fmt.Sprintf("%s-%s%s", base, storage.RandomName("", 8)[:8], ext)
			var n int64
			if err := s.db.Model(&model.Image{}).Unscoped().Where("storage_key = ?", candidate).Count(&n).Error; err != nil {
				return "", err
			}
			if n == 0 {
				return candidate, nil
			}
		}
		return "", errors.New("无法生成不冲突的文件名，请改用随机命名")
	default:
		if naming == model.NamingDate {
			return time.Now().UTC().Format("2006/01/02/") + storage.RandomName(ext, 16), nil
		}
		return storage.RandomName(ext, 16), nil
	}
}

func (s *UploadService) checkRate(ctx context.Context, userID uint64, perHour int) error {
	if s.rdb == nil {
		return nil
	}
	bucket := time.Now().UTC().Format("2006010215")
	key := fmt.Sprintf("islet:rate:upload:%d:%s", userID, bucket)
	n, err := s.rdb.Incr(ctx, key).Result()
	if err != nil {
		return nil // Redis 故障不阻塞上传
	}
	if n == 1 {
		s.rdb.Expire(ctx, key, 2*time.Hour)
	}
	if int(n) > perHour {
		return ErrUploadRateLimited
	}
	return nil
}

func (s *UploadService) usedBytes(userID uint64) (uint64, error) {
	var used uint64
	err := s.db.Model(&model.Image{}).Where("user_id = ? AND deleted_at IS NULL", userID).
		Select("COALESCE(SUM(size), 0)").Scan(&used).Error
	return used, err
}

// UsedBytes 供配额展示。
func (s *UploadService) UsedBytes(userID uint64) (uint64, error) { return s.usedBytes(userID) }

// buildThumb 生成最长边 480px 的缩略图；不适用时返回 false。
func BuildThumbnail(data []byte) ([]byte, bool) { return buildThumb(data) }

func buildThumb(data []byte) ([]byte, bool) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, false
	}
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= 0 || h <= 0 {
		return nil, false
	}
	scale := 1.0
	if w > thumbMaxEdge || h > thumbMaxEdge {
		if w >= h {
			scale = float64(thumbMaxEdge) / float64(w)
		} else {
			scale = float64(thumbMaxEdge) / float64(h)
		}
	}
	tw, th := max(1, int(float64(w)*scale)), max(1, int(float64(h)*scale))

	// 检查源图是否有透明通道（PNG 常见）；有则输出 PNG，否则 JPEG q82
	hasAlpha := false
	switch src.(type) {
	case *image.NRGBA, *image.NRGBA64:
		hasAlpha = true
	case *image.RGBA, *image.RGBA64:
		hasAlpha = true
	}

	// 最近邻采样足够缩略图场景（避免引入外部长边依赖；x/image draw 亦可，但简单实现更可控）
	dst := image.NewRGBA(image.Rect(0, 0, tw, th))
	for y := 0; y < th; y++ {
		sy := bounds.Min.Y + y*bounds.Dy()/th
		for x := 0; x < tw; x++ {
			sx := bounds.Min.X + x*bounds.Dx()/tw
			dst.Set(x, y, src.At(sx, sy))
		}
	}

	var out bytes.Buffer
	if hasAlpha {
		if err := png.Encode(&out, dst); err != nil {
			return nil, false
		}
		return out.Bytes(), true
	}
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: 82}); err != nil {
		return nil, false
	}
	return out.Bytes(), true
}

func thumbMime(key string) string {
	if strings.HasSuffix(key, ".png") {
		return "image/png"
	}
	return "image/jpeg"
}

func mimeExt(mime string) string {
	switch mime {
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/bmp":
		return ".bmp"
	default:
		return ".jpg"
	}
}

func validImageExt(ext string) bool {
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".avif":
		return true
	}
	return false
}

func sanitizeBaseName(name string) string {
	base := filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	base = strings.TrimSpace(base)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	if base == "" || base == "." || base == "/" {
		return "image"
	}
	// 净化文件名：保留 Unicode 字母数字与安全符号，其余转 '-'
	var b strings.Builder
	for _, r := range base {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r), r == '-', r == '_', r == '.', r == ' ':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	result := strings.Trim(b.String(), "-. ")
	if result == "" {
		return "image"
	}
	if len([]rune(result)) > 120 {
		result = string([]rune(result)[:120])
	}
	return result
}

func sanitizeName(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	if name == "" || name == "." || name == "/" {
		return "image"
	}
	if len([]rune(name)) > 255 {
		name = string([]rune(name)[:255])
	}
	return name
}

func normalizeTags(tags []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" || len([]rune(t)) > 32 {
			continue
		}
		if seen[t] {
			continue
		}
		seen[t] = true
		result = append(result, t)
	}
	sort.Strings(result)
	if len(result) > 16 {
		result = result[:16]
	}
	return result
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	value := float64(n)
	units := []string{"KB", "MB", "GB", "TB"}
	i := -1
	for value >= unit && i < len(units)-1 {
		value /= unit
		i++
	}
	return fmt.Sprintf("%.1f %s", value, units[i])
}

func containsUint64(list []uint64, target uint64) bool {
	for _, v := range list {
		if v == target {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ParseUploadForm 从 multipart 表单提取上传文件（files 字段，兼容 file）。
func ParseUploadForm(c *gin.Context, maxBytes int64) ([]*multipart.FileHeader, error) {
	form, err := c.MultipartForm()
	if err != nil {
		return nil, errors.New("请使用 multipart/form-data 上传")
	}
	files := form.File["files"]
	if len(files) == 0 {
		files = form.File["file"]
	}
	if len(files) == 0 {
		return nil, errors.New("未找到上传文件（字段名 files）")
	}
	if len(files) > 30 {
		return nil, errors.New("单次最多上传 30 个文件")
	}
	for _, f := range files {
		if f.Size > maxBytes {
			return nil, fmt.Errorf("%w：%s（站点上限 %s）", ErrUploadTooLarge, f.Filename, humanBytes(maxBytes))
		}
	}
	return files, nil
}
