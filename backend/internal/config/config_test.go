package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadPriority(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "islet.json")
	if err := os.WriteFile(file, []byte(`{"mysql_host":"file-host","redis_host":"file-redis","auth_mode":"oidc","oidc_issuer":"https://idp.example.com"}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONFIG_FILE", file)
	t.Setenv("MYSQL_HOST", "env-host")
	cfg := Load()
	if cfg.MySQLHost != "env-host" {
		t.Fatalf("env should override file, got %s", cfg.MySQLHost)
	}
	if cfg.RedisHost != "file-redis" {
		t.Fatalf("file should override default, got %s", cfg.RedisHost)
	}
	if cfg.OIDCIssuer != "https://idp.example.com" {
		t.Fatalf("issuer from file, got %s", cfg.OIDCIssuer)
	}
}

func TestCompleteState(t *testing.T) {
	cfg := Config{InstallationComplete: true}
	if !cfg.Complete() {
		t.Fatal("expected complete")
	}
	cfg.ResetPending = true
	if cfg.Complete() {
		t.Fatal("reset pending should not be complete")
	}
}

func TestAuthReady(t *testing.T) {
	oidc := Config{AuthMode: "oidc", OIDCIssuer: "https://idp", OIDCClientID: "app"}
	if !oidc.AuthReady() {
		t.Fatal("oidc ready")
	}
	oidcMissing := Config{AuthMode: "oidc"}
	if oidcMissing.AuthReady() {
		t.Fatal("issuer missing should not be ready")
	}
	local := Config{AuthMode: "local"}
	if !local.AuthReady() {
		t.Fatal("local always ready")
	}
}

func TestSaveAndReload(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "nested", "islet.json")
	cfg := Config{
		MySQLHost: "db", MySQLUser: "u", MySQLPassword: "p",
		AuthMode: "local", BootstrapVerified: true, BootstrapUsername: "admin",
		InstallationComplete: true,
	}
	if err := SaveFile(file, cfg); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONFIG_FILE", file)
	loaded := Load()
	if loaded.MySQLHost != "db" || !loaded.Complete() || !loaded.BootstrapVerified {
		t.Fatalf("round trip mismatch: %+v", loaded)
	}
}

func TestResetMarker(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "islet.json")
	_ = SaveFile(file, Config{InstallationComplete: true})
	t.Setenv("CONFIG_FILE", file)
	_ = os.Setenv("MYSQL_HOST", "")
	if err := ResetMarker(file); err != nil {
		t.Fatal(err)
	}
	cfg := Load()
	if cfg.Complete() {
		t.Fatal("reset marker must force setup mode")
	}
	if err := ClearResetMarker(file); err != nil {
		t.Fatal(err)
	}
	cfg = Load()
	if !cfg.Complete() {
		t.Fatal("marker cleared should restore complete")
	}
	_ = time.Now
}
