package service

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

var ErrTokenInvalid = errors.New("token invalid")

// Identity OIDC 身份声明。
type Identity struct {
	Subject  string
	Username string
	Email    string
}

// Discovery 标准 OIDC Discovery 文档（只保留用到的字段）。
type Discovery struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
	EndSessionEndpoint    string `json:"end_session_endpoint"`
}

type jwkKey struct {
	KeyID     string `json:"kid"`
	KeyType   string `json:"kty"`
	Algorithm string `json:"alg"`
	Use       string `json:"use"`
	Modulus   string `json:"n"`
	Exponent  string `json:"e"`
	Curve     string `json:"crv"`
	X         string `json:"x"`
	Y         string `json:"y"`
}

type jwksDocument struct {
	Keys []jwkKey `json:"keys"`
}

type jwksEntry struct {
	keys       map[string]crypto.PublicKey
	fetchedAt  time.Time
	fetching   sync.Mutex
	lastFailed bool
}

// OIDCService 提供 Discovery 缓存与 JWT 验签（RS256/384/512、ES256/384/521）。
type OIDCService struct {
	client *http.Client

	mu      sync.Mutex
	disco   map[string]Discovery
	jwks    map[string]*jwksEntry
	discoAt map[string]time.Time
}

func NewOIDCService() *OIDCService {
	return &OIDCService{
		client:  &http.Client{Timeout: 10 * time.Second},
		disco:   map[string]Discovery{},
		jwks:    map[string]*jwksEntry{},
		discoAt: map[string]time.Time{},
	}
}

// Discover 拉取并校验 {issuer}/.well-known/openid-configuration，结果缓存 10 分钟。
// 校验文档 issuer 与请求一致（RFC 校验，允许完全相等即可）。
func (s *OIDCService) Discover(ctx context.Context, issuer string) (Discovery, error) {
	issuer = strings.TrimRight(strings.TrimSpace(issuer), "/")
	if issuer == "" || !strings.HasPrefix(issuer, "http://") && !strings.HasPrefix(issuer, "https://") {
		return Discovery{}, errors.New("Issuer 需以 http(s):// 开头")
	}
	s.mu.Lock()
	if meta, ok := s.disco[issuer]; ok && time.Since(s.discoAt[issuer]) < 10*time.Minute {
		s.mu.Unlock()
		return meta, nil
	}
	s.mu.Unlock()

	url := issuer + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Discovery{}, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return Discovery{}, fmt.Errorf("无法访问 Discovery 地址：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Discovery{}, fmt.Errorf("Discovery 返回 HTTP %d", resp.StatusCode)
	}
	var meta Discovery
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return Discovery{}, fmt.Errorf("Discovery 文档解析失败：%w", err)
	}
	if strings.TrimRight(meta.Issuer, "/") != issuer {
		return Discovery{}, errors.New("Discovery 文档的 issuer 与填写的 Issuer 不一致")
	}
	if meta.AuthorizationEndpoint == "" || meta.TokenEndpoint == "" || meta.JWKSURI == "" {
		return Discovery{}, errors.New("Discovery 文档缺少 authorization_endpoint / token_endpoint / jwks_uri")
	}

	s.mu.Lock()
	s.disco[issuer] = meta
	s.discoAt[issuer] = time.Now()
	s.mu.Unlock()
	return meta, nil
}

func (s *OIDCService) jwksEntry(jwksURI string) *jwksEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.jwks[jwksURI]
	if !ok {
		entry = &jwksEntry{keys: map[string]crypto.PublicKey{}}
		s.jwks[jwksURI] = entry
	}
	return entry
}

// fetchKeys 返回 kid -> 公钥；TTL 5 分钟，lastFailed 时强制刷新。
func (s *OIDCService) fetchKeys(ctx context.Context, jwksURI string, force bool) (map[string]crypto.PublicKey, error) {
	entry := s.jwksEntry(jwksURI)
	entry.fetching.Lock()
	defer entry.fetching.Unlock()
	if !force && !entry.lastFailed && time.Since(entry.fetchedAt) < 5*time.Minute && len(entry.keys) > 0 {
		return entry.keys, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jwksURI, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		entry.lastFailed = true
		return nil, fmt.Errorf("无法获取 JWKS：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		entry.lastFailed = true
		return nil, fmt.Errorf("JWKS 返回 HTTP %d", resp.StatusCode)
	}
	var doc jwksDocument
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		entry.lastFailed = true
		return nil, fmt.Errorf("JWKS 解析失败：%w", err)
	}
	keys := make(map[string]crypto.PublicKey, len(doc.Keys))
	for _, k := range doc.Keys {
		if k.Use != "" && k.Use != "sig" {
			continue
		}
		switch {
		case k.KeyType == "RSA" && k.Modulus != "" && k.Exponent != "":
			if pub, err := rsaPublicKey(k); err == nil {
				keys[k.KeyID] = pub
			}
		case k.KeyType == "EC" && k.X != "" && k.Y != "":
			if pub, err := ecPublicKey(k); err == nil {
				keys[k.KeyID] = pub
			}
		}
	}
	entry.keys = keys
	entry.fetchedAt = time.Now()
	entry.lastFailed = false
	return keys, nil
}

// VerifyOptions JWT 校验参数。
type VerifyOptions struct {
	Issuer   string
	Audience string // 允许 aud；空则跳过
	JWKSURI  string // 可空，自动从 Discovery 取
}

func (s *OIDCService) Verify(ctx context.Context, token string, opt VerifyOptions) (Identity, error) {
	opt.Issuer = strings.TrimRight(strings.TrimSpace(opt.Issuer), "/")
	jwksURI := opt.JWKSURI
	if jwksURI == "" {
		meta, err := s.Discover(ctx, opt.Issuer)
		if err != nil {
			return Identity{}, err
		}
		jwksURI = meta.JWKSURI
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Identity{}, ErrTokenInvalid
	}
	var header struct {
		Algorithm string `json:"alg"`
		KeyID     string `json:"kid"`
	}
	if err := decodeJWTPart(parts[0], &header); err != nil {
		return Identity{}, ErrTokenInvalid
	}
	if header.KeyID == "" {
		return Identity{}, ErrTokenInvalid
	}
	hash, byteLen, ok := algorithmParams(header.Algorithm)
	if !ok {
		return Identity{}, ErrTokenInvalid
	}

	var payload map[string]any
	if err := decodeJWTPart(parts[1], &payload); err != nil {
		return Identity{}, ErrTokenInvalid
	}
	if opt.Issuer != "" && stringClaim(payload, "iss") != opt.Issuer {
		return Identity{}, ErrTokenInvalid
	}
	if opt.Audience != "" && !audienceContains(payload["aud"], opt.Audience) {
		return Identity{}, ErrTokenInvalid
	}
	if exp, ok := numberClaim(payload, "exp"); ok && time.Now().Unix() >= int64(exp) {
		return Identity{}, ErrTokenInvalid
	}

	signed := []byte(parts[0] + "." + parts[1])
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return Identity{}, ErrTokenInvalid
	}

	keys, err := s.fetchKeys(ctx, jwksURI, false)
	if err != nil {
		return Identity{}, err
	}
	pub, ok := keys[header.KeyID]
	if !ok {
		if keys, err = s.fetchKeys(ctx, jwksURI, true); err != nil {
			return Identity{}, err
		}
		if pub, ok = keys[header.KeyID]; !ok {
			return Identity{}, ErrTokenInvalid
		}
	}

	switch key := pub.(type) {
	case *rsa.PublicKey:
		digest := hashHash(hash, signed)
		if err := rsa.VerifyPKCS1v15(key, hash, digest, signature); err != nil {
			return Identity{}, ErrTokenInvalid
		}
	case *ecdsa.PublicKey:
		if len(signature) != 2*byteLen {
			return Identity{}, ErrTokenInvalid
		}
		r := new(big.Int).SetBytes(signature[:byteLen])
		sc := new(big.Int).SetBytes(signature[byteLen:])
		digest := hashHash(hash, signed)
		if !ecdsa.Verify(key, digest, r, sc) {
			return Identity{}, ErrTokenInvalid
		}
	default:
		return Identity{}, ErrTokenInvalid
	}

	return Identity{
		Subject:  stringClaim(payload, "sub"),
		Username: firstNonEmpty(stringClaim(payload, "preferred_username"), stringClaim(payload, "username"), stringClaim(payload, "name")),
		Email:    stringClaim(payload, "email"),
	}, nil
}

func hashHash(h crypto.Hash, data []byte) []byte {
	switch h {
	case crypto.SHA384:
		sum := sha512.Sum384(data)
		return sum[:]
	case crypto.SHA512:
		sum := sha512.Sum512(data)
		return sum[:]
	default:
		sum := sha256.Sum256(data)
		return sum[:]
	}
}

// algorithmParams 返回 (哈希, EC 字节长, 是否支持)。
func algorithmParams(alg string) (crypto.Hash, int, bool) {
	switch alg {
	case "RS256", "ES256":
		return crypto.SHA256, 32, true
	case "RS384", "ES384":
		return crypto.SHA384, 48, true
	case "RS512", "ES512":
		return crypto.SHA512, 66, true
	}
	return 0, 0, false
}

func rsaPublicKey(key jwkKey) (*rsa.PublicKey, error) {
	modulus, err := base64.RawURLEncoding.DecodeString(key.Modulus)
	if err != nil {
		return nil, err
	}
	exponent, err := base64.RawURLEncoding.DecodeString(key.Exponent)
	if err != nil {
		return nil, err
	}
	n := new(big.Int).SetBytes(modulus)
	e := new(big.Int).SetBytes(exponent).Int64()
	if e == 0 {
		return nil, errors.New("invalid exponent")
	}
	return &rsa.PublicKey{N: n, E: int(e)}, nil
}

func ecPublicKey(key jwkKey) (*ecdsa.PublicKey, error) {
	var curve elliptic.Curve
	var byteLen int
	switch key.Curve {
	case "P-256":
		curve, byteLen = elliptic.P256(), 32
	case "P-384":
		curve, byteLen = elliptic.P384(), 48
	case "P-521":
		curve, byteLen = elliptic.P521(), 66
	default:
		return nil, errors.New("unsupported curve")
	}
	xBytes, err := base64.RawURLEncoding.DecodeString(key.X)
	if err != nil {
		return nil, err
	}
	yBytes, err := base64.RawURLEncoding.DecodeString(key.Y)
	if err != nil {
		return nil, err
	}
	if len(xBytes) != byteLen || len(yBytes) != byteLen {
		return nil, errors.New("invalid point length")
	}
	x := new(big.Int).SetBytes(xBytes)
	y := new(big.Int).SetBytes(yBytes)
	if !curve.IsOnCurve(x, y) {
		return nil, errors.New("point not on curve")
	}
	return &ecdsa.PublicKey{Curve: curve, X: x, Y: y}, nil
}

func decodeJWTPart(part string, target any) error {
	data, err := base64.RawURLEncoding.DecodeString(part)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func stringClaim(payload map[string]any, key string) string {
	if v, ok := payload[key].(string); ok {
		return v
	}
	return ""
}

func numberClaim(payload map[string]any, key string) (float64, bool) {
	v, ok := payload[key].(float64)
	return v, ok
}

func audienceContains(value any, expected string) bool {
	switch audience := value.(type) {
	case string:
		return audience == expected
	case []any:
		for _, item := range audience {
			if s, ok := item.(string); ok && s == expected {
				return true
			}
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
