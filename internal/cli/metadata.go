package cli

// // 元数据: metadata（AI 生成标题/简介/标签单步）

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/zolagz/ytb2bili-go/internal/metadata"
	"github.com/zolagz/ytb2bili-go/internal/pipeline"
)

// ─── Metadata 生成 ─────────────────────────────────────────────────────────

func newMetadataCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "metadata <videoId or path-to-srt>",
		Aliases: []string{"meta"},
		Short:   "根据字幕生成B站标题、描述和标签（保存 JSON）",
		Long: `读取字幕文件内容，调用 LLM 生成投稿 B站用的中文标题、描述和标签，
并保存为 JSON（默认 <字幕同目录>/<名字>.meta.json）。

参数支持 videoId 或完整字幕路径。
示例:
  ytb metadata lVIvZM8zay4
  ytb metadata data/downloads/lVIvZM8zay4/lVIvZM8zay4.zh-Hans.srt
  ytb metadata --output meta.json video.srt`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("请输入 videoId 或字幕文件路径")
			}
			asJSON, _ := cmd.Flags().GetBool("json")
			jsonMode = asJSON
			defer func() { jsonMode = false }()

			cfg := loadConfig()
			srtPath, err := pipeline.ResolveInput(cfg, args[0], "zh-srt")
			if err != nil {
				return err
			}

			output, _ := cmd.Flags().GetString("output")
			if output == "" {
				output = strings.TrimSuffix(srtPath, filepath.Ext(srtPath)) + ".meta.json"
			}

			outf("🤖 生成元数据: %s\n", srtPath)
			outf("\n")

			start := time.Now()
			meta, err := metadata.GenerateFromSRT(context.Background(), srtPath, cfg)
			if err != nil {
				return fmt.Errorf("生成元数据失败: %w", err)
			}

			data, err := json.MarshalIndent(meta, "", "  ")
			if err != nil {
				return err
			}
			if err := os.WriteFile(output, data, 0644); err != nil {
				return fmt.Errorf("写入 JSON 失败: %w", err)
			}

			outf("✅ 生成完成 (耗时: %v)\n", time.Since(start).Round(time.Second))
			outf("  标题: %s\n", meta.Title)
			outf("  标签: %s\n", strings.Join(meta.Tags, ", "))
			outf("📄 %s\n", output)
			if asJSON {
				return emitJSON(struct {
					OK          bool     `json:"ok"`
					Step        string   `json:"step"`
					Title       string   `json:"title"`
					Description string   `json:"description"`
					Tags        []string `json:"tags"`
					Output      string   `json:"output"`
				}{true, "metadata", meta.Title, meta.Description, meta.Tags, output})
			}
			return nil
		},
	}
	cmd.Flags().StringP("output", "o", "", "JSON 输出路径（默认 <字幕>.meta.json）")
	cmd.Flags().Bool("json", false, "以 JSON 输出结果（stdout 仅含 JSON）")
	return cmd
}

// ─── Translate ─────────────────────────────────────────────────────────────
