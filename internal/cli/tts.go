package cli

// // TTS: tts（腾讯云合成单步）

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/zolagz/ytb2bili-go/internal/pipeline"
	"github.com/zolagz/ytb2bili-go/internal/tts"
)

// ─── Tencent TTS ───────────────────────────────────────────────────────────

func newTencentTTSCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "tts <input.srt>",
		Aliases: []string{"tencent-tts"}, // 旧名保留可用
		Short:   "按字幕合成分段配音（默认腾讯云 TTS）",
		Long: `读取 SRT 字幕文件，逐条合成分段配音 MP3。
按字幕序号命名输出（1.mp3, 2.mp3, ...），供 audio-sync 步骤使用。

合成器由 config 的 tts.provider 控制（tencent / index）。
环境变量:
  TENCENTCLOUD_SECRET_ID    腾讯云 API 密钥 ID
  TENCENTCLOUD_SECRET_KEY   腾讯云 API 密钥 Key`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("请输入 SRT 字幕文件路径")
			}
			asJSON, _ := cmd.Flags().GetBool("json")
			jsonMode = asJSON
			defer func() { jsonMode = false }()

			cfg := loadConfig()
			srtPath, err := pipeline.ResolveInput(cfg, args[0], "zh-srt")
			if err != nil {
				return err
			}

			outputDir, _ := cmd.Flags().GetString("output")
			if outputDir == "" {
				outputDir = filepath.Join(filepath.Dir(srtPath), "voice")
			}
			concurrency, _ := cmd.Flags().GetInt("concurrency")

			// 合成器由 config 的 tts.provider 控制：index → 本地 IndexTTS，tencent → 腾讯云
			provider := pipeline.SelectTTSProvider(cfg)
			if provider == "index" {
				outf("🎙 本地 IndexTTS 合成: %s\n", srtPath)
				outf("   ├ 输出目录: %s\n", outputDir)
				outf("\n")
				if err := pipeline.RunIndexTTSSRT(context.Background(), srtPath, outputDir, cfg.TTS.Index, cfg.DataDir); err != nil {
					return err
				}
				if asJSON {
					return emitJSON(struct {
						OK       bool   `json:"ok"`
						Step     string `json:"step"`
						Provider string `json:"provider"`
						VoiceDir string `json:"voice_dir"`
					}{true, "tts", provider, outputDir})
				}
				return nil
			}

			ttsCfg := tts.FromAppConfig(cfg)
			if v, _ := cmd.Flags().GetInt64("voice"); cmd.Flags().Changed("voice") {
				ttsCfg.Voice = v
			}
			if v, _ := cmd.Flags().GetFloat64("volume"); cmd.Flags().Changed("volume") {
				ttsCfg.Volume = v
			}
			if v, _ := cmd.Flags().GetFloat64("speed"); cmd.Flags().Changed("speed") {
				ttsCfg.Speed = v
			}

			outf("🎤 腾讯云 TTS 合成: %s\n", srtPath)
			outf("   ├ 输出目录: %s\n", outputDir)
			outf("   └ 并发数: %d\n", concurrency)
			outf("\n")

			results, err := tts.SynthesizeSRT(context.Background(), srtPath, outputDir, ttsCfg, concurrency)
			if err != nil {
				return fmt.Errorf("合成失败: %w", err)
			}

			success, failed := 0, 0
			for _, r := range results {
				if r.Err != nil {
					failed++
				} else {
					success++
				}
			}
			outf("\n✅ 合成完成: %d 成功, %d 失败\n", success, failed)
			if asJSON {
				return emitJSON(struct {
					OK       bool   `json:"ok"`
					Step     string `json:"step"`
					Provider string `json:"provider"`
					VoiceDir string `json:"voice_dir"`
					Success  int    `json:"success"`
					Failed   int    `json:"failed"`
				}{true, "tts", provider, outputDir, success, failed})
			}
			return nil
		},
	}
	cmd.Flags().StringP("output", "o", "", "音频输出目录")
	cmd.Flags().Int("concurrency", 3, "并发合成数")
	cmd.Flags().Int64("voice", 0, "音色: 0=亲和女声, 1=成熟女声, 2=成熟男声, 3=亲和男声")
	cmd.Flags().Float64("volume", 2, "音量: 0-15")
	cmd.Flags().Float64("speed", 1, "语速: 0-2")
	cmd.Flags().Bool("json", false, "以 JSON 输出结果（stdout 仅含 JSON）")
	return cmd
}
