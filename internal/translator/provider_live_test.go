//go:build live

// Package translator 真实外部服务测试：仅在显式启用 live 标签时编译运行。
// 运行方式: go test -tags live ./internal/translator/
// 需要 config.yaml 中存在有效的腾讯云/其他 provider 凭证。
package translator

import (
	"context"
	"strings"
	"testing"

	"github.com/zolagz/ytb2bili-go/internal/config"
)

// TestTencentProviderLive 真实调用腾讯翻译（需要 config.yaml 的 tencent_cloud 凭证）。
func TestTencentProviderLive(t *testing.T) {
	cfg, err := config.LoadYAML("../../config.yaml")
	if err != nil || cfg.TencentCloud == nil || cfg.TencentCloud.SecretID == "" {
		t.Skip("无腾讯云凭证，跳过真实调用测试")
	}

	tp := NewTencentProvider(TencentConfig{
		SecretID:  cfg.TencentCloud.SecretID,
		SecretKey: cfg.TencentCloud.SecretKey,
		Region:    cfg.TencentCloud.Region,
	})

	texts := []string{
		"Hello world, this is a test.",
		"Python is a popular programming language.",
		"Machine learning is changing the world.",
	}
	out, err := tp.TranslateBatch(context.Background(), texts, "en", "zh-Hans")
	if err != nil {
		t.Fatalf("腾讯翻译批量失败: %v", err)
	}
	if len(out) != 3 {
		t.Fatalf("len(out) = %d, want 3", len(out))
	}
	for i, line := range out {
		t.Logf("  [%d] %s", i+1, line)
		if strings.TrimSpace(line) == "" {
			t.Errorf("第 %d 条翻译为空", i+1)
		}
	}
}
