package translator

import (
	"os"
	"testing"

	"github.com/zolagz/ytb2bili-go/internal/config"
)

// TestRouterWithTencentFallback 验证 Router 配置：DeepSeek primary + Tencent fallback。
// 依赖本地 config.yaml（含 translation 段）；无配置时跳过，不依赖真实 API。
func TestRouterWithTencentFallback(t *testing.T) {
	cfg, err := config.LoadYAML("../../config.yaml")
	if err != nil {
		if os.IsNotExist(err) {
			t.Skip("无 config.yaml（CI/纯净环境），跳过")
		}
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
