package router

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"islet/backend/internal/config"
	"islet/backend/internal/service"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOIDCCallbackIdentityAndRedirect(t *testing.T) {
	gin.SetMode(gin.TestMode)
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	mux := http.NewServeMux()
	issuer := httptest.NewServer(mux)
	defer issuer.Close()
	encode := base64.RawURLEncoding.EncodeToString
	claims, _ := json.Marshal(map[string]any{"iss": issuer.URL, "aud": "client", "sub": "owner", "exp": time.Now().Add(time.Hour).Unix()})
	signing := encode([]byte(`{"alg":"RS256","kid":"test"}`)) + "." + encode(claims)
	digest := sha256.Sum256([]byte(signing))
	signature, _ := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	token := signing + "." + encode(signature)
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"issuer": issuer.URL, "token_endpoint": issuer.URL + "/token", "authorization_endpoint": issuer.URL + "/auth", "jwks_uri": issuer.URL + "/jwks"})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{"kty": "RSA", "kid": "test", "alg": "RS256", "n": encode(key.PublicKey.N.Bytes()), "e": "AQAB"}}})
	})
	missingID := false
	calls := 0
	t.Setenv("OIDC_CLIENT_ID", "client")
	t.Setenv("OIDC_CLIENT_SECRET", "test-client-secret")
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		calls++
		r.ParseForm()
		if r.Form.Get("client_secret") != "test-client-secret" || r.Form.Get("code_verifier") != "verifier" {
			t.Error("confidential PKCE exchange fields missing")
		}
		if missingID {
			json.NewEncoder(w).Encode(map[string]string{"access_token": token})
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"access_token": "opaque-access-token", "id_token": token})
	})
	engine := gin.New()
	registerAuthRoutes(engine.Group("/api/v1"), &Deps{Cfg: config.Config{OIDCIssuer: issuer.URL, OIDCClientID: "client", AdminBaseURL: "https://images.example"}, OIDC: service.NewOIDCService()})
	invoke := func(redirect string) int {
		body, _ := json.Marshal(map[string]string{"code": "test-code", "code_verifier": "verifier", "redirect_uri": redirect})
		request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/oidc/callback", bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		return response.Code
	}
	if invoke("https://images.example/auth/callback") != 200 {
		t.Fatal("valid ID token with opaque access token must succeed")
	}
	missingID = true
	if invoke("https://images.example/auth/callback") != 401 {
		t.Fatal("access token must not replace missing identity token")
	}
	before := calls
	if invoke("https://untrusted.example/auth/callback") != 400 || calls != before {
		t.Fatal("foreign callback must be rejected before token exchange")
	}
}
