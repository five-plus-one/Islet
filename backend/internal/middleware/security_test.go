package middleware

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestProtection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		origin string
		want   int
	}{{"", 200}, {"https://islet.example", 200}, {"https://evil.example", 403}, {"null", 403}} {
		t.Run(tc.origin, func(t *testing.T) {
			e := gin.New()
			e.Use(RequestProtection())
			e.POST("/api/save", func(c *gin.Context) { c.Status(200) })
			r := httptest.NewRequest(http.MethodPost, "https://islet.example/api/save", nil)
			r.Header.Set("Origin", tc.origin)
			w := httptest.NewRecorder()
			e.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("status %d, want %d", w.Code, tc.want)
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("account responses must not be cached")
			}
		})
	}
}

func TestUploadCredentialCannotManage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, viaToken := range []bool{false, true} {
		e := gin.New()
		e.Use(func(c *gin.Context) { c.Set(ContextUserKey, AuthUser{ID: 1, Role: "super_admin", ViaToken: viaToken}) }, RequireInteractive())
		e.GET("/", func(c *gin.Context) { c.Status(200) })
		w := httptest.NewRecorder()
		e.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
		want := 200
		if viaToken {
			want = 403
		}
		if w.Code != want {
			t.Fatalf("upload credential=%v: status %d", viaToken, w.Code)
		}
	}
}
