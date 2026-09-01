package cli

// // 一键投稿：submit（下载→转录→翻译→TTS→投稿完整流水线）

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zolagz/ytb2bili-go/internal/config"
	"github.com/zolagz/ytb2bili-go/internal/pipeline"
	"github.com/zolagz/ytb2bili-go/internal/workflow"
)

// ─── Submit ────────────────────────────────────────────────────────────────

func newSubmitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "submit <YouTube URL>",
		Short: "提交搬运任务",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("请输入 YouTube URL")
			}
			asJSON, _ := cmd.Flags().GetBool("json")
			jsonMode = asJSON
			defer func() { jsonMode = false }()

			cfg := loadConfig()
			url := args[0]

			sourceLang, _ := cmd.Flags().GetString("source-lang")
			targetLang, _ := cmd.Flags().GetString("target-lang")
			if targetLang == "" {
				targetLang = cfg.EffectiveTranslationTargetLang()
			}
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			skipTrans, _ := cmd.Flags().GetBool("skip-translate")
			tid, _ := cmd.Flags().GetInt("tid")
			if tid == 0 {
				tid = cfg.BiliTid
			}
			showPlan, _ := cmd.Flags().GetBool("show-plan")
			chainStr, _ := cmd.Flags().GetString("chain")

			processor := &pipeline.Processor{Config: cfg, Reporter: pipelineReporter()}
			result, err := processor.Process(context.Background(), pipeline.Request{
				URL: url, SourceLang: sourceLang, TargetLang: targetLang,
				Tid: tid, DryRun: dryRun, SkipTranslate: skipTrans,
				Source: "manual", Chain: workflow.ParseChain(chainStr),
				PlanOnly: showPlan,
			})
			if result != nil {
				outf("🔗 任务链: %s\n", strings.Join(result.Plan, " → "))
				if result.BVID != "" {
					outf("📺 https://www.bilibili.com/video/%s\n", result.BVID)
				}
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
				}{true, "submit", result.TaskID, result.VideoID, result.BVID, result.Plan})
			}
			return nil
		},
	}
	cmd.Flags().String("source-lang", "en", "源语言")
	cmd.Flags().String("target-lang", "", "目标语言")
	cmd.Flags().Int("tid", 0, "B站分区ID")
	cmd.Flags().Bool("dry-run", false, "仅处理不上传")
	cmd.Flags().Bool("skip-translate", false, "跳过翻译")
	cmd.Flags().Bool("show-plan", false, "只显示规划不执行")
	cmd.Flags().String("chain", "", "自定义任务链")
	cmd.Flags().Bool("json", false, "以 JSON 输出结果（stdout 仅含 JSON）")
	return cmd
}

// ─── Queue ─────────────────────────────────────────────────────────────────

// processSingle 处理单个视频的完整流水线（auto --submit 内部使用）
// pipelineReporter 返回流水线进度输出的统一回调。
func pipelineReporter() pipeline.Reporter {
	return func(event pipeline.Event) {
		if event.Status == "running" {
			fmt.Printf("[%d/%d] %s... ", event.Position, event.Total, event.Step)
		} else if event.Err != nil {
			fmt.Printf("❌ %v\n", event.Err)
		} else {
			fmt.Println("✅")
		}
	}
}

func processSingle(cfg *config.Config, url string, skipTranslate bool) (*pipeline.Result, error) {
	targetLang := cfg.EffectiveTranslationTargetLang()
	if skipTranslate {
		targetLang = ""
	}
	return (&pipeline.Processor{
		Config:   cfg,
		Reporter: pipelineReporter(),
	}).Process(context.Background(), pipeline.Request{
		URL:        url,
		Source:     "auto",
		TargetLang: targetLang,
	})
}

// ─── Server ────────────────────────────────────────────────────────────────
