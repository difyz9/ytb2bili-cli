package cli

// // 音画同步：audio-sync（对已有产物做配音合成）

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/zolagz/ytb2bili-go/internal/audiosync"
	"github.com/zolagz/ytb2bili-go/internal/pipeline"
)

// ─── Audio-Sync ─────────────────────────────────────────────────────────────

func newAudioSyncCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audio-sync <videoId>",
		Short: "对已有下载产物执行音画同步",
		Long: `直接使用 data/downloads/<videoId>/ 下的已有产物做音画同步：
视频 + 译文字幕（优先 zh-Hans，回退源字幕）+ voice/ 配音目录，输出 <videoId>.synced.mp4。

示例:
  ytb audio-sync yn4MSHbKgmo

配合幂等续跑：audio-sync 生成 synced.mp4 后，再执行 submit <videoId> 会跳过
已完成的 download/transcribe/translate/tts/audio-sync，直接做元数据生成和投稿。`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("请输入 videoId 或视频路径")
			}
			asJSON, _ := cmd.Flags().GetBool("json")
			jsonMode = asJSON
			defer func() { jsonMode = false }()

			cfg := loadConfig()
			videoDir := pipeline.ResolveVideoDir(cfg, args[0])
			videoID := filepath.Base(videoDir)

			video, subtitle, voiceDir, err := pipeline.ResolveSyncArtifacts(videoDir, videoID)
			if err != nil {
				return err
			}
			output := filepath.Join(filepath.Dir(video), videoID+".synced.mp4")

			missing, _ := cmd.Flags().GetString("missing")
			noSpeed, _ := cmd.Flags().GetBool("no-speed-adjust")

			outf("🎬 音画同步: %s\n", filepath.Base(video))
			outf("   📄 字幕: %s\n", filepath.Base(subtitle))
			outf("   🎤 配音: %s\n", filepath.Base(voiceDir))

			start := time.Now()
			result, err := audiosync.Sync(context.Background(), audiosync.Options{
				VideoPath: video, SubtitlePath: subtitle, AudioDir: voiceDir, OutputPath: output,
				DisableSpeedAdjust: noSpeed, MissingMode: missing,
			})
			if err != nil {
				return err
			}
			outf("✅ 音画同步完成 (耗时 %v): %s\n", time.Since(start).Round(time.Second), result.Output)
			outf("   📦 时长 %.0fs | 片段 %d | 调整 %d | 缺失 %d\n",
				result.Duration, result.Clips, result.Adjusted, result.Missing)
			if asJSON {
				return emitJSON(struct {
					OK       bool    `json:"ok"`
					Step     string  `json:"step"`
					VideoID  string  `json:"video_id"`
					Output   string  `json:"output"`
					Duration float64 `json:"duration"`
					Clips    int     `json:"clips"`
					Adjusted int     `json:"adjusted"`
					Missing  int     `json:"missing"`
				}{true, "audio-sync", videoID, result.Output, result.Duration, result.Clips, result.Adjusted, result.Missing})
			}
			return nil
		},
	}
	cmd.Flags().String("missing", "", "缺失配音处理: error(默认) / silence")
	cmd.Flags().Bool("no-speed-adjust", false, "不调整配音语速")
	cmd.Flags().Bool("json", false, "以 JSON 输出结果（stdout 仅含 JSON）")
	return cmd
}

// ─── Submit ────────────────────────────────────────────────────────────────
