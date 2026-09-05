package cli

// // 转录: transcribe/bcut/whisper（语音转文字单步）

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/zolagz/ytb2bili-go/internal/pipeline"
	"github.com/zolagz/ytb2bili-go/internal/transcriber"
)

// ─── BCut ASR ──────────────────────────────────────────────────────────────

func newBcutCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "bcut <audio/video file>",
		Hidden: true, // 已由 transcribe --provider bcut 取代，保留直接调用
		Short:  "使用 Bcut ASR 听录音频",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("请输入音频或视频文件路径")
			}
			cfg := loadConfig()
			audioPath, err := pipeline.ResolveInput(cfg, args[0], "video")
			if err != nil {
				return err
			}

			outputDir := filepath.Dir(audioPath)
			videoID := strings.TrimSuffix(filepath.Base(audioPath), filepath.Ext(audioPath))

			fmt.Printf("🎤 Bcut ASR 听录: %s\n", audioPath)
			fmt.Printf("   输出目录: %s\n", outputDir)
			fmt.Println()

			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()

			start := time.Now()
			srtPath, err := transcriber.BcutASRContext(ctx, audioPath, outputDir, videoID)
			if err != nil {
				return fmt.Errorf("听录失败: %w", err)
			}

			fmt.Printf("✅ 听录完成! (耗时: %v)\n", time.Since(start).Round(time.Second))
			fmt.Printf("📄 %s\n", srtPath)
			return nil
		},
	}
	return cmd
}

// ─── Whisper.cpp 本地转录 ──────────────────────────────────────────────────

// ─── Whisper.cpp 本地转录 ──────────────────────────────────────────────────

func newWhisperCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "whisper <audio/video file>",
		Hidden: true, // 已由 transcribe --provider whisper 取代，保留直接调用
		Short:  "使用 whisper.cpp 本地听录音频",
		Long: `使用本地 whisper.cpp（whisper-cli）听录音频并生成 SRT 字幕。

默认模型/线程取自 config.yaml 的 transcriber.whisper，可用 --model/--threads 覆盖。
示例:
  ytb whisper video.mp4
  ytb whisper --model models/ggml-small.bin -l en video.mp4`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("请输入音频/视频文件路径或 videoId")
			}
			cfg := loadConfig()
			audioPath, err := pipeline.ResolveInput(cfg, args[0], "video")
			if err != nil {
				return err
			}

			wcfg := cfg.Transcriber.Whisper

			outputDir, _ := cmd.Flags().GetString("out")
			if outputDir == "" {
				outputDir = filepath.Dir(audioPath)
			}
			videoID := strings.TrimSuffix(filepath.Base(audioPath), filepath.Ext(audioPath))
			lang, _ := cmd.Flags().GetString("lang")
			if m, _ := cmd.Flags().GetString("model"); m != "" {
				wcfg.Model = m
			}
			if t, _ := cmd.Flags().GetInt("threads"); t > 0 {
				wcfg.Threads = t
			}

			fmt.Printf("🎤 whisper.cpp 听录: %s\n", audioPath)
			fmt.Printf("   模型: %s\n", wcfg.Model)
			fmt.Printf("   语言: %s\n", langOrAuto(lang))
			fmt.Printf("   输出目录: %s\n", outputDir)
			fmt.Println()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
			defer cancel()

			start := time.Now()
			srtPath, err := transcriber.WhisperContext(ctx, wcfg, audioPath, outputDir, videoID, lang)
			if err != nil {
				return fmt.Errorf("听录失败: %w", err)
			}

			fmt.Printf("✅ 听录完成! (耗时: %v)\n", time.Since(start).Round(time.Second))
			fmt.Printf("📄 %s\n", srtPath)
			return nil
		},
	}
	cmd.Flags().String("model", "", "GGML 模型路径（默认取 config 的 transcriber.whisper.model）")
	cmd.Flags().StringP("lang", "l", "", "语言代码 en/zh/auto（默认 auto）")
	cmd.Flags().Int("threads", 0, "推理线程数（默认取 config）")
	cmd.Flags().StringP("out", "o", "", "输出目录（默认与输入文件同目录）")
	return cmd
}

func langOrAuto(lang string) string {
	if strings.TrimSpace(lang) == "" {
		return "auto"
	}
	return lang
}

// ─── Transcribe（统一转录） ───────────────────────────────────────────────

// ─── Transcribe（统一转录） ───────────────────────────────────────────────

func newTranscribeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "transcribe <audio/video file>",
		Short: "听录音频生成字幕（本地 whisper / 云 Bcut）",
		Long: `听录音频/视频生成 SRT 字幕，后端由 --provider 或 config 的 transcriber.provider 决定。

默认本地 whisper.cpp；--provider bcut 用云 ASR。参数支持 videoId 或完整路径。
示例:
  ytb transcribe video.mp4
  ytb transcribe --provider bcut video.mp4
  ytb transcribe --model models/ggml-base.bin -l en video.mp4`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("请输入音频/视频文件路径或 videoId")
			}
			asJSON, _ := cmd.Flags().GetBool("json")
			jsonMode = asJSON
			defer func() { jsonMode = false }()

			cfg := loadConfig()
			audioPath, err := pipeline.ResolveInput(cfg, args[0], "video")
			if err != nil {
				return err
			}
			provider, _ := cmd.Flags().GetString("provider")
			if provider == "" {
				provider = cfg.EffectiveTranscriberProvider()
			}

			outputDir, _ := cmd.Flags().GetString("out")
			if outputDir == "" {
				outputDir = filepath.Dir(audioPath)
			}
			videoID := strings.TrimSuffix(filepath.Base(audioPath), filepath.Ext(audioPath))

			outf("🎤 听录 (%s): %s\n", provider, audioPath)
			outf("   输出目录: %s\n", outputDir)
			outf("\n")

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
			defer cancel()

			start := time.Now()
			var srtPath string
			switch provider {
			case "bcut":
				srtPath, err = transcriber.BcutASRContext(ctx, audioPath, outputDir, videoID)
			default: // whisper
				wcfg := cfg.Transcriber.Whisper
				if m, _ := cmd.Flags().GetString("model"); m != "" {
					wcfg.Model = m
				}
				if t, _ := cmd.Flags().GetInt("threads"); t > 0 {
					wcfg.Threads = t
				}
				lang, _ := cmd.Flags().GetString("lang")
				srtPath, err = transcriber.WhisperContext(ctx, wcfg, audioPath, outputDir, videoID, lang)
			}
			if err != nil {
				return fmt.Errorf("听录失败: %w", err)
			}

			outf("✅ 听录完成! (耗时: %v)\n", time.Since(start).Round(time.Second))
			outf("📄 %s\n", srtPath)
			if asJSON {
				return emitJSON(struct {
					OK       bool   `json:"ok"`
					Step     string `json:"step"`
					Provider string `json:"provider"`
					VideoID  string `json:"video_id"`
					Srt      string `json:"srt"`
				}{true, "transcribe", provider, videoID, srtPath})
			}
			return nil
		},
	}
	cmd.Flags().String("provider", "", "转录后端: whisper(默认)/bcut")
	cmd.Flags().String("model", "", "whisper 模型路径（覆盖 config）")
	cmd.Flags().StringP("lang", "l", "", "whisper 语言 en/zh/auto（默认 auto）")
	cmd.Flags().Int("threads", 0, "whisper 推理线程数（覆盖 config）")
	cmd.Flags().StringP("out", "o", "", "输出目录（默认与输入同目录）")
	cmd.Flags().Bool("json", false, "以 JSON 输出结果（stdout 仅含 JSON）")
	return cmd
}

// ─── Metadata 生成 ─────────────────────────────────────────────────────────
