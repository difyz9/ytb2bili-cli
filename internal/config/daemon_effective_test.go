package config

import (
	"os"
	"testing"
)

// boolPtr/intPtr 构造指针，便于表测试。
func bptr(v bool) *bool { return &v }
func iptr(v int) *int   { return &v }

func TestDaemonEffectiveCleanupDefaults(t *testing.T) {
	var nilCfg *DaemonConfig
	if !nilCfg.EffectiveCleanupAfterUpload() {
		t.Error("nil 配置默认应投稿后清理产物")
	}
	if !nilCfg.EffectiveCleanupKeepSubtitles() {
		t.Error("nil 配置默认应保留字幕/封面")
	}
	if got := nilCfg.EffectiveMinFreeGB(); got != 20 {
		t.Errorf("nil 配置磁盘水位默认应为 20，实际 %d", got)
	}
	if got := nilCfg.EffectiveKeywordsPerBatch(); got != 5 {
		t.Errorf("nil 配置每批关键词默认应为 5，实际 %d", got)
	}
	if got := nilCfg.EffectiveSearchFailBackoffMaxSec(); got != 1800 {
		t.Errorf("nil 配置搜索退避上限默认应为 1800，实际 %d", got)
	}
}

func TestDaemonEffectiveCleanupOverrides(t *testing.T) {
	c := &DaemonConfig{
		CleanupAfterUpload:      bptr(false),
		CleanupKeepSubtitles:    bptr(false),
		MinFreeGB:               iptr(0),
		KeywordsPerBatch:        iptr(0),
		SearchFailBackoffMaxSec: iptr(0),
	}
	if c.EffectiveCleanupAfterUpload() {
		t.Error("显式关闭后不应清理产物")
	}
	if c.EffectiveCleanupKeepSubtitles() {
		t.Error("显式关闭后不应保留字幕")
	}
	if got := c.EffectiveMinFreeGB(); got != 0 {
		t.Errorf("水位 0 = 禁用，实际 %d", got)
	}
	if got := c.EffectiveKeywordsPerBatch(); got != 0 {
		t.Errorf("每批关键词 0 = 全部，实际 %d", got)
	}
	if got := c.EffectiveSearchFailBackoffMaxSec(); got != 0 {
		t.Errorf("退避 0 = 禁用退避，实际 %d", got)
	}
}

func TestDaemonEffectiveNegativeValues(t *testing.T) {
	c := &DaemonConfig{MinFreeGB: iptr(-5), KeywordsPerBatch: iptr(-3), SearchFailBackoffMaxSec: iptr(-1)}
	if got := c.EffectiveMinFreeGB(); got != 0 {
		t.Errorf("负数水位应视为禁用，实际 %d", got)
	}
	if got := c.EffectiveKeywordsPerBatch(); got != 0 {
		t.Errorf("负数关键词数应视为全部，实际 %d", got)
	}
	if got := c.EffectiveSearchFailBackoffMaxSec(); got != 0 {
		t.Errorf("负数退避应视为禁用，实际 %d", got)
	}
}

func TestDaemonYAMLParsesCleanupKeys(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/config.yaml"
	yaml := []byte(`
daemon:
  cleanup_after_upload: false
  cleanup_keep_subtitles: false
  min_free_gb: 35
  keywords_per_batch: 8
  search_fail_backoff_max_sec: 600
`)
	if err := os.WriteFile(path, yaml, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadYAML(path)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if cfg.Daemon == nil {
		t.Fatal("daemon 段未解析")
	}
	if cfg.Daemon.EffectiveCleanupAfterUpload() {
		t.Error("cleanup_after_upload: false 未生效")
	}
	if cfg.Daemon.EffectiveCleanupKeepSubtitles() {
		t.Error("cleanup_keep_subtitles: false 未生效")
	}
	if got := cfg.Daemon.EffectiveMinFreeGB(); got != 35 {
		t.Errorf("min_free_gb 应为 35，实际 %d", got)
	}
	if got := cfg.Daemon.EffectiveKeywordsPerBatch(); got != 8 {
		t.Errorf("keywords_per_batch 应为 8，实际 %d", got)
	}
	if got := cfg.Daemon.EffectiveSearchFailBackoffMaxSec(); got != 600 {
		t.Errorf("search_fail_backoff_max_sec 应为 600，实际 %d", got)
	}
}
