package cli

// // 任务链：chain run/plan/list（可组合单步流水线）

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zolagz/ytb2bili-go/internal/pipeline"
	"github.com/zolagz/ytb2bili-go/internal/workflow"
)

// ─── Chain ─────────────────────────────────────────────────────────────────

func newChainCmd() *cobra.Command {
	chain := &cobra.Command{
		Use:   "chain",
		Short: "任务链管理",
	}

	runCmd := &cobra.Command{
		Use:   "run <step1,step2,...> <YouTube URL>",
		Short: "运行自定义任务链",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 2 {
				return fmt.Errorf("用法: ytb chain run <step1,step2,...> <YouTube URL>")
			}
			asJSON, _ := cmd.Flags().GetBool("json")
			jsonMode = asJSON
			defer func() { jsonMode = false }()

			cfg := loadConfig()
			chainSteps := workflow.ParseChain(args[0])
			url := args[1]

			outf("🔗 任务链: %s\n", strings.Join(chainSteps, " → "))
			outf("📺 %s\n\n", url)

			dryRun, _ := cmd.Flags().GetBool("dry-run")
			skipTrans, _ := cmd.Flags().GetBool("skip-translate")
			tid, _ := cmd.Flags().GetInt("tid")
			if tid == 0 {
				tid = cfg.BiliTid
			}
			// 语言参数：默认 en → 配置目标语言。修复历史 bug：chain run 未传
			// source-lang 导致语言判定短路，英文原文被当"翻译结果"（BV1GKMr6fEZy）。
			sourceLang, _ := cmd.Flags().GetString("source-lang")
			if sourceLang == "" {
				sourceLang = "en"
			}
			targetLang, _ := cmd.Flags().GetString("target-lang")
			if targetLang == "" {
				targetLang = cfg.EffectiveTranslationTargetLang()
			}

			processor := &pipeline.Processor{Config: cfg, Reporter: pipelineReporter()}
			result, err := processor.Process(context.Background(), pipeline.Request{
				URL: url, Chain: chainSteps, DryRun: dryRun,
				SkipTranslate: skipTrans, Tid: tid, Source: "manual",
				SourceLang: sourceLang, TargetLang: targetLang,
			})
			if result != nil && result.BVID != "" {
				outf("\n📺 https://www.bilibili.com/video/%s\n", result.BVID)
			}
			if err != nil {
				return err
			}
			if asJSON && result != nil {
				return emitJSON(struct {
					OK      bool     `json:"ok"`
					Step    string   `json:"step"`
					TaskID  string   `json:"task_id"`
					VideoID string   `json:"video_id"`
					BVID    string   `json:"bvid"`
					Plan    []string `json:"plan"`
				}{true, "chain", result.TaskID, result.VideoID, result.BVID, result.Plan})
			}
			return nil
		},
	}
	runCmd.Flags().Bool("dry-run", false, "仅处理不上传")
	runCmd.Flags().Bool("skip-translate", false, "跳过翻译")
	runCmd.Flags().Int("tid", 0, "B站分区ID")
	runCmd.Flags().String("source-lang", "", "源语言（默认 en）")
	runCmd.Flags().String("target-lang", "", "目标语言（默认读取配置）")
	runCmd.Flags().Bool("json", false, "以 JSON 输出结果（stdout 仅含 JSON）")

	planCmd := &cobra.Command{
		Use:   "plan <step1,step2,...> <YouTube URL>",
		Short: "查看任务链规划（不执行）",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 2 {
				return fmt.Errorf("用法: ytb chain plan <step1,step2,...> <YouTube URL>")
			}
			cfg := loadConfig()
			chainSteps := workflow.ParseChain(args[0])
			url := args[1]

			processor := &pipeline.Processor{Config: cfg}
			result, err := processor.Process(context.Background(), pipeline.Request{
				URL: url, Chain: chainSteps, PlanOnly: true, Source: "manual",
			})
			if err != nil {
				return err
			}
			fmt.Printf("🔗 任务链规划:\n")
			for i, step := range result.Plan {
				fmt.Printf("  %d. %s\n", i+1, step)
			}
			return nil
		},
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "列出所有可用步骤",
		RunE: func(cmd *cobra.Command, args []string) error {
			steps := []struct{ name, desc string }{
				{"download", "下载视频"},
				{"transcribe", "BCut ASR 语音转字幕"},
				{"translate", "LLM 翻译字幕"},
				{"tts", "IndexTTS 合成分段配音"},
				{"audio-sync", "对齐配音并替换音轨"},
				{"metadata", "AI 生成标题、简介和标签"},
				{"upload", "投稿到 B站"},
			}
			fmt.Println("📋 可用步骤:")
			for _, s := range steps {
				fmt.Printf("  %-14s %s\n", s.name, s.desc)
			}
			return nil
		},
	}

	chain.AddCommand(runCmd, planCmd, listCmd)
	return chain
}

// ─── Audio-Sync ─────────────────────────────────────────────────────────────
