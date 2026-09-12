package cli

// 产物清理：clean —— 清理已投稿视频的本地产物，释放磁盘空间

import (
	"github.com/spf13/cobra"

	"github.com/zolagz/ytb2bili-go/internal/diskspace"
	"github.com/zolagz/ytb2bili-go/internal/pipeline"
)

// ─── Clean ─────────────────────────────────────────────────────────────────

func newCleanCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clean [videoId...]",
		Short: "清理本地产物释放磁盘（默认保留字幕与封面）",
		Long: `清理下载目录中已处理视频的重型产物（视频、音画同步产物、配音中间音频）。

默认保留字幕(.srt/<id>.zh-Hans.srt)与封面(cover.jpg)——字幕审核通过后仍需异步上传。
不带参数 = 清理下载目录下所有视频；带 videoId = 只清理指定视频（幂等，目录不存在则跳过）。

示例:
  ytb clean                      # 清理全部视频的大文件（保留字幕/封面）
  ytb clean --dry-run            # 只统计可释放空间，不删除
  ytb clean FwOTs4UxQS4          # 只清理指定视频
  ytb clean --include-subtitles  # 连字幕/封面也删（彻底释放）
  ytb clean --json               # 机器可读输出`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			includeSubs, _ := cmd.Flags().GetBool("include-subtitles")
			jsonOut, _ := cmd.Flags().GetBool("json")
			if jsonOut {
				jsonMode = true
				defer func() { jsonMode = false }()
			}

			root := cfg.EffectiveDownloadDir()
			before, _ := diskspace.FreeBytes(root)

			results, err := pipeline.CleanupAllArtifacts(cfg, args, !includeSubs, dryRun)
			if err != nil {
				return err
			}
			videos, files, freed := pipeline.SummarizeCleanup(results)

			for _, r := range results {
				if r.DeletedFiles > 0 {
					if jsonOut {
						continue
					}
					outf("%s\n", pipeline.CleanupLogLine(r))
				}
			}

			after, _ := diskspace.FreeBytes(root)
			if jsonOut {
				return emitJSON(map[string]interface{}{
					"ok": true, "step": "clean", "dry_run": dryRun,
					"videos": videos, "deleted_files": files,
					"freed_bytes": freed, "freed": diskspace.FormatBytes(freed),
					"free_before": before, "free_after": after,
					"details": results,
				})
			}

			verb := "已清理"
			if dryRun {
				verb = "可清理（dry-run，未删除）"
			}
			outf("\n🧹 %s %d 个视频 / %d 个文件，释放 %s\n", verb, videos, files, diskspace.FormatBytes(freed))
			outf("   下载目录可用空间: %s → %s\n", diskspace.FormatBytes(before), diskspace.FormatBytes(after))
			if !includeSubs && files > 0 {
				outf("   （字幕/封面已保留；如需彻底删除用 --include-subtitles）\n")
			}
			return nil
		},
	}
	cmd.Flags().Bool("dry-run", false, "只统计可释放空间，不实际删除")
	cmd.Flags().Bool("include-subtitles", false, "连字幕/封面一起删除（默认保留，字幕审核通过后需上传）")
	cmd.Flags().Bool("json", false, "机器可读 JSON 输出（stdout 仅 JSON）")
	return cmd
}
