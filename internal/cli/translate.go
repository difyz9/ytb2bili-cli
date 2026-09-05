package cli

// // 翻译: translate（字幕翻译单步，含服务商自检）

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/zolagz/ytb2bili-go/internal/config"
	"github.com/zolagz/ytb2bili-go/internal/pipeline"
	"github.com/zolagz/ytb2bili-go/internal/translator"
)

// ─── Translate ─────────────────────────────────────────────────────────────

func newTranslateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "translate <input.srt>",
		Short: "翻译 SRT 字幕文件",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()

			// --test: 测试所有已配置的翻译服务商连通性
			if testMode, _ := cmd.Flags().GetBool("test"); testMode {
				return runTranslateTest(cfg)
			}

			if len(args) == 0 {
				return fmt.Errorf("请输入 SRT 字幕文件路径")
			}
			asJSON, _ := cmd.Flags().GetBool("json")
			jsonMode = asJSON
			defer func() { jsonMode = false }()

			inputPath, err := pipeline.ResolveInput(cfg, args[0], "srt")
			if err != nil {
				return err
			}

			sourceLang, _ := cmd.Flags().GetString("source-lang")
			targetLang, _ := cmd.Flags().GetString("target-lang")
			if targetLang == "" {
				targetLang = cfg.EffectiveTranslationTargetLang()
			}

			outf("🌐 翻译字幕: %s\n", inputPath)
			outf("   %s → %s\n", sourceLang, targetLang)
			outf("\n")

			start := time.Now()
			result, err := translator.SRTContext(context.Background(), inputPath, sourceLang, targetLang, cfg)
			if err != nil {
				return fmt.Errorf("翻译失败: %w", err)
			}

			outf("✅ 翻译完成 (耗时: %v)\n", time.Since(start).Round(time.Second))
			outf("📄 %s\n", result)
			if asJSON {
				return emitJSON(struct {
					OK         bool   `json:"ok"`
					Step       string `json:"step"`
					Input      string `json:"input"`
					Output     string `json:"output"`
					SourceLang string `json:"source_lang"`
					TargetLang string `json:"target_lang"`
				}{true, "translate", inputPath, result, sourceLang, targetLang})
			}
			return nil
		},
	}
	cmd.Flags().String("source-lang", "en", "源语言")
	cmd.Flags().String("target-lang", "", "目标语言（默认读取配置）")
	cmd.Flags().Bool("test", false, "测试所有已配置翻译服务商的连通性（不翻译文件）")
	cmd.Flags().Bool("json", false, "以 JSON 输出结果（stdout 仅含 JSON）")
	return cmd
}

// runTranslateTest 测试配置的所有翻译服务商连通性。
func runTranslateTest(cfg *config.Config) error {
	fmt.Println("🔍 翻译服务商连通性测试...")
	fmt.Println()

	primary := "deepseek（默认）"
	fallbacks := "无"
	if cfg.Translation != nil {
		if cfg.Translation.Primary != "" {
			primary = cfg.Translation.Primary
		}
		if len(cfg.Translation.Fallbacks) > 0 {
			fallbacks = strings.Join(cfg.Translation.Fallbacks, " → ")
		}
	}
	fmt.Printf("配置: primary=%s, fallbacks=%s\n\n", primary, fallbacks)

	results := translator.TestProviders(cfg)
	if len(results) == 0 {
		return fmt.Errorf("没有可测试的翻译服务商（检查 config.yaml 的 translation 段）")
	}

	allOK := true
	for _, r := range results {
		status := "❌"
		if r.OK {
			status = "✅"
		} else {
			allOK = false
		}
		latency := r.Latency.Round(time.Millisecond)
		line := fmt.Sprintf("  %s %-10s %8s", status, r.Name, latency)
		if r.OK && r.Note != "" {
			line += "  " + r.Note
		}
		if !r.OK && r.Error != "" {
			line += "  " + r.Error
		}
		fmt.Println(line)
	}
	fmt.Println()
	if allOK {
		fmt.Println("🎉 所有翻译服务商均可用")
	} else {
		fmt.Println("⚠️ 部分服务商不可用（主服务失败时会自动降级）")
	}
	return nil
}

// ─── Tencent TTS ───────────────────────────────────────────────────────────
