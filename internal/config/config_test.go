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

func TestTTSIndexConfigLoads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte(`tts:
  provider: index
  index:
    api_url: "http://127.0.0.1:19999"
    emotion: "happy"
    emotion_alpha: 0.8
    ref_audio: "/data/ref.wav"
    use_emo_text: true
    emo_text: "cheerful"
    concurrency: 2
    retries: 5
    timeout: 120
`), 0644)
	cfg, err := LoadYAML(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TTS == nil || cfg.TTS.Index == nil {
		t.Fatal("TTS.Index is nil")
	}
	idx := cfg.TTS.Index
	if idx.APIURL != "http://127.0.0.1:19999" {
		t.Fatalf("api_url=%q", idx.APIURL)
	}
	if idx.Emotion != "happy" {
		t.Fatalf("emotion=%q", idx.Emotion)
	}
	if idx.EmotionAlpha != 0.8 {
		t.Fatalf("emotion_alpha=%v", idx.EmotionAlpha)
	}
	if idx.RefAudio != "/data/ref.wav" {
		t.Fatalf("ref_audio=%q", idx.RefAudio)
	}
	if !idx.UseEmoText || idx.EmoText != "cheerful" {
		t.Fatalf("use_emo_text=%v emo_text=%q", idx.UseEmoText, idx.EmoText)
	}
	if idx.Concurrency != 2 || idx.Retries != 5 || idx.Timeout != 120 {
		t.Fatalf("concurrency=%d retries=%d timeout=%v", idx.Concurrency, idx.Retries, idx.Timeout)
	}
}

func TestDefaultTTSIndexConfig(t *testing.T) {
	cfg := Default()
	if cfg.TTS == nil || cfg.TTS.Index == nil {
		t.Fatal("default TTS.Index is nil")
	}
	if cfg.TTS.Index.APIURL != "http://localhost:18765" {
		t.Fatalf("default api_url=%q", cfg.TTS.Index.APIURL)
	}
	if cfg.TTS.Index.Emotion != "default" {
		t.Fatalf("default emotion=%q", cfg.TTS.Index.Emotion)
	}
	if cfg.TTS.Index.Concurrency != 1 || cfg.TTS.Index.Retries != 3 || cfg.TTS.Index.Timeout != 180 {
		t.Fatalf("default concurrency=%d retries=%d timeout=%v", cfg.TTS.Index.Concurrency, cfg.TTS.Index.Retries, cfg.TTS.Index.Timeout)
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
