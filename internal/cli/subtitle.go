package cli

// // 字幕管理：subtitle status/upload

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/zolagz/ytb2bili-go/internal/bili"
)

// ─── Subtitle ──────────────────────────────────────────────────────────────

func newSubtitleCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "subtitle", Short: "字幕管理"}

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "查看字幕上传状态",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			entries, _ := os.ReadDir(filepath.Join(cfg.DataDir, "subtitles"))
			if len(entries) == 0 {
				fmt.Println("\U0001f4ed 暂无字幕记录")
				return nil
			}
			fmt.Printf("\U0001f4cb 共 %d 个字幕记录:\n\n", len(entries))
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				fmt.Printf("  %s\n", e.Name())
			}
			return nil
		},
	}

	uploadCmd := &cobra.Command{
		Use:   "upload <bvid> <subtitle.srt>",
		Short: "上传字幕到已发布的视频",
		Long: `上传 SRT 字幕文件到已发布的 B站视频（获取 CID → 转换 → 保存草稿）。
语言可用 --lang 指定（默认 zh）。

示例:
  ytb subtitle upload BV1xx123 subtitle.zh-Hans.srt
  ytb subtitle upload BV1xx123 subtitle.srt --lang en`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 2 {
				return fmt.Errorf("用法: ytb subtitle upload <bvid> <subtitle.srt>")
			}
			cfg := loadConfig()
			cred, err := loadCredential(cfg)
			if err != nil {
				return err
			}
			lang, _ := cmd.Flags().GetString("lang")
			if lang == "" {
				lang = "zh"
			}
			fmt.Printf("📝 上传字幕 %s → %s (lang=%s)\n", args[1], args[0], lang)
			if err := bili.UploadSubtitle(cred, args[0], args[1], lang); err != nil {
				return err
			}
			fmt.Printf("✅ 字幕上传成功: %s\n", args[0])
			return nil
		},
	}
	uploadCmd.Flags().String("lang", "zh", "字幕语言（如 zh / zh-Hans / en）")

	cmd.AddCommand(statusCmd, uploadCmd)
	return cmd
}

// ─── Cookies ───────────────────────────────────────────────────────────────
