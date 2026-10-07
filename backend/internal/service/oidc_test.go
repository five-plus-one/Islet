package service

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// newTestIssuer 搭一个本地 OIDC Discovery + JWKS 假 IdP。
func newTestIssuer(t *testing.T, key *rsa.PrivateKey, kid string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	base := server.URL
	_ = pem.EncodeToMemory(&pem.Block{Type: "RSA PUBLIC KEY", Bytes: x509.MarshalPKCS1PublicKey(&key.PublicKey)})
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer":                 base,
			"authorization_endpoint": base + "/auth",
			"token_endpoint":         base + "/token",
			"jwks_uri":               base + "/jwks",
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]string{{
				"kty": "RSA", "kid": kid, "alg": "RS256", "use": "sig",
				"n": base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()),
				"e": "AQAB",
			}},
		})
	})
	return server
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func signJWT(t *testing.T, key *rsa.PrivateKey, kid string, payload map[string]any) string {
	t.Helper()
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": kid})
	body, _ := json.Marshal(payload)
	signing := b64(header) + "." + b64(body)
	digest := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return signing + "." + b64(sig)
}

func TestOIDCVerifyOK(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	server := newTestIssuer(t, key, "test-kid")
	oidc := NewOIDCService()
	token := signJWT(t, key, "test-kid", map[string]any{
		"iss": server.URL, "aud": "client", "sub": "user-1",
		"exp": float64(time.Now().Add(time.Hour).Unix()),
		"preferred_username": "alice", "email": "a@b.c",
	})
	identity, err := oidc.Verify(context.Background(), token, VerifyOptions{Issuer: server.URL, Audience: "client"})
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	if identity.Subject != "user-1" || identity.Username != "alice" || identity.Email != "a@b.c" {
		t.Fatalf("identity mismatch: %+v", identity)
	}
}

func TestOIDCVerifyRejects(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	server := newTestIssuer(t, key, "kid")
	oidc := NewOIDCService()
	expired := signJWT(t, key, "kid", map[string]any{
		"iss": server.URL, "aud": "client", "sub": "u",
		"exp": float64(time.Now().Add(-time.Hour).Unix()),
	})
	if _, err := oidc.Verify(context.Background(), expired, VerifyOptions{Issuer: server.URL, Audience: "client"}); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("expired token should be rejected, got %v", err)
	}
	wrongIssuer := signJWT(t, key, "kid", map[string]any{
		"iss": "https://other", "aud": "client", "sub": "u",
		"exp": float64(time.Now().Add(time.Hour).Unix()),
	})
	if _, err := oidc.Verify(context.Background(), wrongIssuer, VerifyOptions{Issuer: server.URL, Audience: "client"}); err == nil {
		t.Fatal("wrong issuer should be rejected")
	}
	wrongKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	forged := signJWT(t, wrongKey, "kid", map[string]any{
		"iss": server.URL, "aud": "client", "sub": "u",
		"exp": float64(time.Now().Add(time.Hour).Unix()),
	})
	if _, err := oidc.Verify(context.Background(), forged, VerifyOptions{Issuer: server.URL, Audience: "client"}); err == nil {
		t.Fatal("forged signature should be rejected")
	}
}

func TestDiscoveryValidation(t *testing.T) {
	oidc := NewOIDCService()
	if _, err := oidc.Discover(context.Background(), "not-a-url"); err == nil {
		t.Fatal("invalid issuer should fail")
	}
	if _, err := oidc.Discover(context.Background(), ""); err == nil {
		t.Fatal("empty issuer should fail")
	}
}
