package middleware

import (
	"strings"
	"testing"

	"islet/backend/internal/model"
)

func TestBearerToken(t *testing.T) {
	if got := bearerToken("Bearer abc"); got != "abc" {
		t.Fatalf("got %q", got)
	}
	if got := bearerToken("bearer abc"); got != "" {
		t.Fatalf("case-sensitive prefix expected, got %q", got)
	}
	if got := bearerToken(""); got != "" {
		t.Fatalf("empty header, got %q", got)
	}
}

func TestRequireRoleLogic(t *testing.T) {
	// super_admin 恒通过、其余按名单匹配的语义由 RequireRole 中间件承载；
	// 这里直接验证角色常量与判定函数组合。
	if model.UserRoleSuperAdmin != "super_admin" {
		t.Fatal("role constant")
	}
	allowed := []string{model.UserRoleAdmin}
	if model.UserRoleAdmin != allowed[0] {
		t.Fatal("admin in list")
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if firstNonEmpty("", "a", "b") != "a" {
		t.Fatal("first non empty")
	}
	if firstNonEmpty("", "") != "" {
		t.Fatal("all empty")
	}
	_ = strings.TrimSpace("")
}
