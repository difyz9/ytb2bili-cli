package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveConfigPath(t *testing.T) {
	setHome := func(t *testing.T) string {
		t.Helper()
		home := t.TempDir()
		t.Setenv("HOME", home)
		return home
	}

	t.Run("flag takes precedence", func(t *testing.T) {
		setHome(t)
		t.Setenv("YTB2BILI_CONFIG", "/env/config.yaml")
		t.Chdir(t.TempDir())
		os.WriteFile("config.yaml", []byte("data_dir: ./data\n"), 0644)
		if got := resolveConfigPath("/flag/config.yaml"); got != "/flag/config.yaml" {
			t.Fatalf("got %q, want /flag/config.yaml", got)
		}
	})

	t.Run("env beats cwd config", func(t *testing.T) {
		setHome(t)
		t.Setenv("YTB2BILI_CONFIG", "/env/config.yaml")
		t.Chdir(t.TempDir())
		os.WriteFile("config.yaml", []byte("data_dir: ./data\n"), 0644)
		if got := resolveConfigPath(""); got != "/env/config.yaml" {
			t.Fatalf("got %q, want /env/config.yaml", got)
		}
	})

	t.Run("user config beats cwd config", func(t *testing.T) {
		home := setHome(t)
		t.Chdir(t.TempDir())
		os.WriteFile("config.yaml", []byte("data_dir: ./data\n"), 0644)
		userConfig := filepath.Join(home, ".ytb", "config.yaml")
		os.MkdirAll(filepath.Dir(userConfig), 0o755)
		os.WriteFile(userConfig, []byte("data_dir: ~/.ytb/data\n"), 0o600)
		if got := resolveConfigPath(""); got != userConfig {
			t.Fatalf("got %q, want %q", got, userConfig)
		}
	})

	t.Run("cwd config.yaml when present", func(t *testing.T) {
		setHome(t)
		t.Chdir(t.TempDir())
		os.WriteFile("config.yaml", []byte("data_dir: ./data\n"), 0644)
		if got := resolveConfigPath(""); got != "config.yaml" {
			t.Fatalf("got %q, want config.yaml", got)
		}
	})

	t.Run("empty when nothing found", func(t *testing.T) {
		setHome(t)
		t.Setenv("YTB2BILI_CONFIG", "")
		t.Chdir(t.TempDir())
		if got := resolveConfigPath(""); got != "" {
			t.Fatalf("got %q, want empty", got)
		}
	})
}

func TestLoadConfigAt(t *testing.T) {
	t.Run("empty path returns defaults with env init", func(t *testing.T) {
		t.Setenv("YTB2BILI_TRANSLATION_TARGET_LANG", "ja")
		cfg, err := loadConfigAt("")
		if err != nil {
			t.Fatal(err)
		}
		if cfg.EffectiveTranslationTargetLang() != "ja" {
			t.Fatalf("target lang=%q, want ja", cfg.EffectiveTranslationTargetLang())
		}
	})

	t.Run("valid yaml loads", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.yaml")
		os.WriteFile(path, []byte("data_dir: /tmp/data\nbili_tid: 17\n"), 0644)
		cfg, err := loadConfigAt(path)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.DataDir != "/tmp/data" || cfg.BiliTid != 17 {
			t.Fatalf("got %+v", cfg)
		}
	})

	t.Run("malformed yaml returns error", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "bad.yaml")
		os.WriteFile(path, []byte("data_dir: [broken\n"), 0644)
		if _, err := loadConfigAt(path); err == nil {
			t.Fatal("expected error for malformed yaml")
		}
	})
}

func TestLoadConfigSearchDaemonSections(t *testing.T) {
	t.Run("search and daemon sections parse", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.yaml")
		os.WriteFile(path, []byte(`
search:
  keywords:
    - "AI tutorial"
    - "ChatGPT"
  scorer: fresh
  upload_date: this_week
  max_duration: 25
  max_videos: 3
  min_views: 500
daemon:
  interval_sec: 30
  consume_per_batch: 4
  max_retries: 5
  step_timeout_sec:
    download: 1800
    tts: 3600
  heartbeat_file: daemon/hb.json
  alert_webhook: "https://open.feishu.cn/open-apis/bot/v2/hook/xxx"
`), 0644)
		cfg, err := loadConfigAt(path)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Search == nil || len(cfg.Search.Keywords) != 2 || cfg.Search.Keywords[0] != "AI tutorial" {
			t.Fatalf("search keywords: %+v", cfg.Search)
		}
		if cfg.Search.Scorer != "fresh" || cfg.Search.UploadDate != "this_week" ||
			cfg.Search.MaxDuration != 25 || cfg.Search.MaxVideos != 3 || cfg.Search.MinViews != 500 {
			t.Fatalf("search params: %+v", cfg.Search)
		}
		if cfg.Daemon == nil || cfg.Daemon.EffectiveInterval() != 30 ||
			cfg.Daemon.EffectiveConsumePerBatch() != 4 || cfg.Daemon.EffectiveMaxRetries() != 5 {
			t.Fatalf("daemon params: %+v", cfg.Daemon)
		}
		if cfg.Daemon.StepTimeout("download") != 1800 || cfg.Daemon.StepTimeout("tts") != 3600 {
			t.Fatalf("step timeouts: %+v", cfg.Daemon.StepTimeoutSec)
		}
		if cfg.Daemon.StepTimeout("unknown") != 0 {
			t.Fatal("unknown step should have 0 timeout")
		}
		if cfg.Daemon.EffectiveHeartbeatFile() != "daemon/hb.json" {
			t.Fatalf("heartbeat file: %s", cfg.Daemon.EffectiveHeartbeatFile())
		}
	})

	t.Run("defaults when sections missing", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.yaml")
		os.WriteFile(path, []byte("data_dir: ./data\n"), 0644)
		cfg, err := loadConfigAt(path)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Search == nil || cfg.Search.Scorer != "nowcast" || cfg.Search.MaxVideos != 5 {
			t.Fatalf("search defaults: %+v", cfg.Search)
		}
		if cfg.Daemon == nil || cfg.Daemon.EffectiveInterval() != 60 ||
			cfg.Daemon.EffectiveConsumePerBatch() != 2 || cfg.Daemon.EffectiveMaxRetries() != 3 {
			t.Fatalf("daemon defaults: %+v", cfg.Daemon)
		}
		if cfg.Daemon.StepTimeout("download") != 1800 || cfg.Daemon.StepTimeout("tts") != 3600 {
			t.Fatalf("default step timeouts: %+v", cfg.Daemon.StepTimeoutSec)
		}
		if cfg.Daemon.EffectiveHeartbeatFile() != filepath.Join("daemon", "heartbeat.json") {
			t.Fatalf("default heartbeat: %s", cfg.Daemon.EffectiveHeartbeatFile())
		}
	})

	t.Run("env webhook fallback", func(t *testing.T) {
		t.Setenv("YTB2BILI_ALERT_WEBHOOK", "https://env.example/hook")
		cfg, err := loadConfigAt("")
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Daemon.AlertWebhook != "https://env.example/hook" {
			t.Fatalf("env webhook not applied: %q", cfg.Daemon.AlertWebhook)
		}
	})
}
