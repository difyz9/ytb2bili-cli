package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
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

func TestIndexTTSAPIKeyFromEnvFallback(t *testing.T) {
	t.Setenv("INDEX_TTS_API_KEY", "env-secret-key")
	cfg := Default()
	cfg.Init() // Default() 的 TTS.Index.APIKey 为空 → 应回退环境变量
	if got := cfg.TTS.Index.APIKey; got != "env-secret-key" {
		t.Fatalf("env fallback failed: got %q", got)
	}
}

func TestIndexTTSAPIKeyConfigWinsOverEnv(t *testing.T) {
	t.Setenv("INDEX_TTS_API_KEY", "env-secret-key")
	cfg := Default()
	cfg.TTS.Index.APIKey = "config-key"
	cfg.Init()
	if got := cfg.TTS.Index.APIKey; got != "config-key" {
		t.Fatalf("config value should win: got %q", got)
	}
}

// --- EffectiveCookiesPath: data/cookies 最新文件选择 ---

// writeCookies 写一个 Netscape 格式的有效 cookies 文件，mtime 设为指定时间。
func writeCookies(t *testing.T, path string, mtime time.Time) {
	t.Helper()
	valid := "# Netscape HTTP Cookie File\n.youtube.com\tTRUE\t/\tTRUE\t1728000000\tSID\tvalue\n"
	if err := os.WriteFile(path, []byte(valid), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

func TestEffectiveCookiesPathPicksNewestFile(t *testing.T) {
	dir := t.TempDir()
	cfg := Default()
	cfg.DataDir = dir

	base := time.Now()
	older := filepath.Join(dir, "cookies", "old_cookies.txt")
	newer := filepath.Join(dir, "cookies", "fresh_cookies.txt")
	os.MkdirAll(filepath.Join(dir, "cookies"), 0o755)
	writeCookies(t, older, base.Add(-2*time.Hour))
	writeCookies(t, newer, base)

	if got := cfg.EffectiveCookiesPath(); got != newer {
		t.Fatalf("EffectiveCookiesPath() = %q, want newest %q", got, newer)
	}
}

func TestEffectiveCookiesPathSkipsInvalidNewestFile(t *testing.T) {
	dir := t.TempDir()
	cfg := Default()
	cfg.DataDir = dir
	cookiesDir := filepath.Join(dir, "cookies")
	os.MkdirAll(cookiesDir, 0o755)

	base := time.Now()
	// 最新的文件内容无效（无有效 cookie 行），应跳过取次新的有效文件
	badNewest := filepath.Join(cookiesDir, "broken.txt")
	os.WriteFile(badNewest, []byte("# empty\n"), 0o600)
	os.Chtimes(badNewest, base, base)
	goodOlder := filepath.Join(cookiesDir, "youtube_cookies_from_meta.txt")
	writeCookies(t, goodOlder, base.Add(-1*time.Hour))

	if got := cfg.EffectiveCookiesPath(); got != goodOlder {
		t.Fatalf("EffectiveCookiesPath() = %q, want valid older %q", got, goodOlder)
	}
}

func TestEffectiveCookiesPathIgnoresHiddenAndNonTxtFiles(t *testing.T) {
	dir := t.TempDir()
	cfg := Default()
	cfg.DataDir = dir
	cookiesDir := filepath.Join(dir, "cookies")
	os.MkdirAll(cookiesDir, 0o755)

	base := time.Now()
	// .DS_Store / .json 不应参与候选
	os.WriteFile(filepath.Join(cookiesDir, ".DS_Store"), []byte("junk"), 0o600)
	os.Chtimes(filepath.Join(cookiesDir, ".DS_Store"), base.Add(time.Hour), base.Add(time.Hour))
	os.WriteFile(filepath.Join(cookiesDir, "notes.md"), []byte("junk"), 0o600)
	os.Chtimes(filepath.Join(cookiesDir, "notes.md"), base.Add(time.Hour), base.Add(time.Hour))
	want := filepath.Join(cookiesDir, "cookies.txt")
	writeCookies(t, want, base)

	if got := cfg.EffectiveCookiesPath(); got != want {
		t.Fatalf("EffectiveCookiesPath() = %q, want %q", got, want)
	}
}

func TestEffectiveCookiesPathExplicitConfigWins(t *testing.T) {
	dir := t.TempDir()
	cfg := Default()
	cfg.DataDir = dir
	explicit := filepath.Join(dir, "explicit_cookies.txt")
	writeCookies(t, explicit, time.Now().Add(-24*time.Hour)) // 故意更旧
	cfg.YouTubeCookies = explicit
	// data/cookies 里放一个更新的文件，但显式配置优先
	cookiesDir := filepath.Join(dir, "cookies")
	os.MkdirAll(cookiesDir, 0o755)
	writeCookies(t, filepath.Join(cookiesDir, "newer.txt"), time.Now())

	if got := cfg.EffectiveCookiesPath(); got != explicit {
		t.Fatalf("EffectiveCookiesPath() = %q, want explicit %q", got, explicit)
	}
}

func TestEffectiveCookiesPathFallsBackToDefaultName(t *testing.T) {
	dir := t.TempDir()
	cfg := Default()
	cfg.DataDir = dir // 无任何 cookies 文件

	want := filepath.Join(dir, "cookies", DefaultCookiesFile)
	if got := cfg.EffectiveCookiesPath(); got != want {
		t.Fatalf("EffectiveCookiesPath() = %q, want %q", got, want)
	}
}
