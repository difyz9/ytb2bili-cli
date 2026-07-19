package config

import (
	"reflect"
	"testing"
)

func TestInitLoadsServerSecurityEnvironment(t *testing.T) {
	t.Setenv("YTB2BILI_SERVER_TOKEN", "secret")
	t.Setenv("YTB2BILI_ALLOWED_ORIGINS", "chrome-extension://one, https://admin.example")
	cfg := Default()
	cfg.Init()
	if cfg.ServerToken != "secret" {
		t.Fatalf("token=%q", cfg.ServerToken)
	}
	want := []string{"chrome-extension://one", "https://admin.example"}
	if !reflect.DeepEqual(cfg.AllowedOrigins, want) {
		t.Fatalf("origins=%v", cfg.AllowedOrigins)
	}
}
