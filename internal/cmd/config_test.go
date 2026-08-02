package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveConfigPath(t *testing.T) {
	t.Run("flag takes precedence", func(t *testing.T) {
		t.Setenv("YTB2BILI_CONFIG", "/env/config.yaml")
		t.Chdir(t.TempDir())
		os.WriteFile("config.yaml", []byte("data_dir: ./data\n"), 0644)
		if got := resolveConfigPath("/flag/config.yaml"); got != "/flag/config.yaml" {
			t.Fatalf("got %q, want /flag/config.yaml", got)
		}
	})

	t.Run("env beats cwd config", func(t *testing.T) {
		t.Setenv("YTB2BILI_CONFIG", "/env/config.yaml")
		t.Chdir(t.TempDir())
		os.WriteFile("config.yaml", []byte("data_dir: ./data\n"), 0644)
		if got := resolveConfigPath(""); got != "/env/config.yaml" {
			t.Fatalf("got %q, want /env/config.yaml", got)
		}
	})

	t.Run("cwd config.yaml when present", func(t *testing.T) {
		t.Chdir(t.TempDir())
		os.WriteFile("config.yaml", []byte("data_dir: ./data\n"), 0644)
		if got := resolveConfigPath(""); got != "config.yaml" {
			t.Fatalf("got %q, want config.yaml", got)
		}
	})

	t.Run("empty when nothing found", func(t *testing.T) {
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
