package config

import (
	"os"
	"path/filepath"
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

func TestTTSProviderConfigLoads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte("tts:\n  provider: index\n"), 0644)
	cfg, err := LoadYAML(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TTS == nil || cfg.TTS.Provider != "index" {
		t.Fatalf("provider=%v, want index", cfg.TTS)
	}
}

func TestDefaultTTSProviderIsEmpty(t *testing.T) {
	cfg := Default()
	if cfg.TTS == nil {
		t.Fatal("default TTS config is nil")
	}
	if cfg.TTS.Provider != "" {
		t.Fatalf("default provider=%q, want empty (auto)", cfg.TTS.Provider)
	}
}
