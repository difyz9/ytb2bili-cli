package cli

// // 下载: download（yt-dlp 封装单步）

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/zolagz/ytb2bili-go/internal/download"
	"github.com/zolagz/ytb2bili-go/internal/search"
)

// ─── Download ──────────────────────────────────────────────────────────────

func newDownloadCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "download <YouTube URL or video ID>",
		Short: "下载 YouTube 视频",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("请输入 YouTube URL 或视频 ID")
			}
			asJSON, _ := cmd.Flags().GetBool("json")
			jsonMode = asJSON
			defer func() { jsonMode = false }()

			cfg := loadConfig()
			raw := args[0]

			videoID := search.ExtractVideoID(raw)
			if videoID == "" {
				return fmt.Errorf("无法提取视频 ID: %s", raw)
			}
			cleanURL := "https://www.youtube.com/watch?v=" + videoID

			outputDir, _ := cmd.Flags().GetString("output")
			if outputDir == "" {
				outputDir = filepath.Join(cfg.EffectiveDownloadDir(), videoID)
			}

			cookiesPath := cfg.EffectiveCookiesPath()

			outf("⬇️  下载视频: %s\n", cleanURL)
			outf("📁 输出目录: %s\n", outputDir)
			outf("\n")

			result, err := download.Video(cleanURL, outputDir, "en", cookiesPath)
			if err != nil {
				return fmt.Errorf("下载失败: %w", err)
			}

			outf("✅ 下载完成\n")
			outf("  视频: %s\n", result.VideoPath)
			outf("  封面: %s\n", result.CoverPath)
			if result.Info.Title != "" {
				outf("  标题: %s\n", result.Info.Title)
			}
			if asJSON {
				return emitJSON(struct {
					OK       bool   `json:"ok"`
					Step     string `json:"step"`
					VideoID  string `json:"video_id"`
					Dir      string `json:"dir"`
					Video    string `json:"video"`
					Cover    string `json:"cover"`
					Subtitle string `json:"subtitle"`
					Title    string `json:"title"`
				}{true, "download", videoID, outputDir, result.VideoPath, result.CoverPath, result.SubtitlePath, result.Info.Title})
			}
			return nil
		},
	}
	cmd.Flags().StringP("output", "o", "", "输出目录")
	cmd.Flags().Bool("json", false, "以 JSON 输出结果（stdout 仅含 JSON）")
	return cmd
}

// ─── BCut ASR ──────────────────────────────────────────────────────────────
