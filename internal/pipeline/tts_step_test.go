package pipeline

import (
	"testing"

	"github.com/zolagz/ytb2bili-go/internal/config"
)

func TestTencentConfigured(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		if tencentConfigured(nil) {
			t.Fatal("expected false for nil config")
		}
	})

	t.Run("empty config", func(t *testing.T) {
		if tencentConfigured(&config.Config{}) {
			t.Fatal("expected false for empty config")
		}
	})

	t.Run("config has both secrets", func(t *testing.T) {
		cfg := &config.Config{
			TencentCloud: &config.TencentCloudConfig{SecretID: "id", SecretKey: "key"},
		}
		if !tencentConfigured(cfg) {
			t.Fatal("expected true when both secrets in config")
		}
	})

	t.Run("config missing secret key", func(t *testing.T) {
		cfg := &config.Config{
			TencentCloud: &config.TencentCloudConfig{SecretID: "id"},
		}
		if tencentConfigured(cfg) {
			t.Fatal("expected false when key missing in config")
		}
	})

	t.Run("env provides secrets", func(t *testing.T) {
		t.Setenv("TENCENTCLOUD_SECRET_ID", "env-id")
		t.Setenv("TENCENTCLOUD_SECRET_KEY", "env-key")
		if !tencentConfigured(&config.Config{}) {
			t.Fatal("expected true when env provides secrets")
		}
	})

	t.Run("env partial secrets", func(t *testing.T) {
		t.Setenv("TENCENTCLOUD_SECRET_ID", "env-id")
		t.Setenv("TENCENTCLOUD_SECRET_KEY", "")
		if tencentConfigured(&config.Config{}) {
			t.Fatal("expected false when env partial")
		}
	})
}

func TestSelectTTSProvider(t *testing.T) {
	withCreds := &config.Config{
		TencentCloud: &config.TencentCloudConfig{SecretID: "id", SecretKey: "key"},
	}

	t.Run("explicit tencent", func(t *testing.T) {
		cfg := &config.Config{TTS: &config.TTSConfig{Provider: "tencent"}}
		if got := SelectTTSProvider(cfg); got != "tencent" {
			t.Fatalf("got %q, want tencent", got)
		}
	})

	t.Run("explicit index", func(t *testing.T) {
		cfg := &config.Config{TTS: &config.TTSConfig{Provider: "index"}}
		if got := SelectTTSProvider(cfg); got != "index" {
			t.Fatalf("got %q, want index", got)
		}
	})

	t.Run("explicit tencent wins over missing creds", func(t *testing.T) {
		cfg := &config.Config{TTS: &config.TTSConfig{Provider: "tencent"}}
		if got := SelectTTSProvider(cfg); got != "tencent" {
			t.Fatalf("got %q, want tencent", got)
		}
	})

	t.Run("auto with creds picks tencent", func(t *testing.T) {
		if got := SelectTTSProvider(withCreds); got != "tencent" {
			t.Fatalf("got %q, want tencent", got)
		}
	})

	t.Run("auto without creds picks index", func(t *testing.T) {
		cfg := &config.Config{TTS: &config.TTSConfig{}}
		if got := SelectTTSProvider(cfg); got != "index" {
			t.Fatalf("got %q, want index", got)
		}
	})

	t.Run("auto alias", func(t *testing.T) {
		cfg := &config.Config{TTS: &config.TTSConfig{Provider: "auto"}}
		if got := SelectTTSProvider(cfg); got != "index" {
			t.Fatalf("got %q, want index (no creds)", got)
		}
	})

	t.Run("case and space insensitive", func(t *testing.T) {
		cfg := &config.Config{TTS: &config.TTSConfig{Provider: " Tencent "}}
		if got := SelectTTSProvider(cfg); got != "tencent" {
			t.Fatalf("got %q, want tencent", got)
		}
	})

	t.Run("nil config defaults to index", func(t *testing.T) {
		if got := SelectTTSProvider(nil); got != "index" {
			t.Fatalf("got %q, want index", got)
		}
	})
}
