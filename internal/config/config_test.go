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

func TestDefaultTranslationTargetIsSimplifiedChinese(t *testing.T) {
	cfg := Default()
	if got := cfg.EffectiveTranslationTargetLang(); got != "zh-Hans" {
		t.Fatalf("target language=%q", got)
	}
}

func TestTranslationTargetEnvironmentOverridesConfig(t *testing.T) {
	t.Setenv("YTB2BILI_TRANSLATION_TARGET_LANG", "ja")
	cfg := Default()
	cfg.Init()
	if got := cfg.EffectiveTranslationTargetLang(); got != "ja" {
		t.Fatalf("target language=%q", got)
	}
}
