package translator

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/zolagz/ytb2bili-go/internal/config"
)

// TestTencentProviderLive 真实调用腾讯翻译（需要 config.yaml 的 tencent_cloud 凭证）。
// 无凭证时跳过。运行: go test -run TestTencentProviderLive -v
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

// TestRouterWithTencentFallback 验证 Router 配置：DeepSeek primary + Tencent fallback。
// 通过 config.yaml 加载（含 translation 段）。
func TestRouterWithTencentFallback(t *testing.T) {
	cfg, err := config.LoadYAML("../../config.yaml")
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if cfg.Translation == nil {
		t.Skip("未配置 translation 段")
	}

	router := buildRouter(cfg.Translation, cfg.LLMAPIKey, cfg.LLMBaseURL, cfg.LLMModel, cfg.TencentCloud)
	if router == nil {
		t.Fatal("buildRouter 返回 nil")
	}
	stats := router.Stats()
	if len(stats) == 0 {
		t.Fatal("Router 无 provider")
	}
	for name := range stats {
		t.Logf("  provider: %s", name)
	}
	// 至少应有 deepseek + tencent
	if _, ok := stats["deepseek"]; !ok {
		t.Error("缺少 deepseek provider")
	}
	if _, ok := stats["tencent"]; !ok {
		t.Error("缺少 tencent provider（fallback 未注册）")
	}
}

var _ = os.Getenv // 保持 os import
