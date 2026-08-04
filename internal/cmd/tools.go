package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/zolagz/ytb2bili-go/internal/auth"
	"github.com/zolagz/ytb2bili-go/internal/cdp"
	"github.com/zolagz/ytb2bili-go/internal/download"
	"github.com/zolagz/ytb2bili-go/internal/metadata"
	"github.com/zolagz/ytb2bili-go/internal/pipeline"
	"github.com/zolagz/ytb2bili-go/internal/search"
	"github.com/zolagz/ytb2bili-go/internal/storage"
	"github.com/zolagz/ytb2bili-go/internal/transcriber"
	"github.com/zolagz/ytb2bili-go/internal/translator"
	"github.com/zolagz/ytb2bili-go/internal/tts"
)

// ─── Login ─────────────────────────────────────────────────────────────────

func newLoginCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "B站扫码登录",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			credDir := filepath.Join(cfg.DataDir, "cookies")
			store := storage.NewCredentialStore(credDir)

			fmt.Println("📱 获取二维码...")
			qr, err := auth.GetQRCode()
			if err != nil {
				return err
			}

			qrPath := filepath.Join(credDir, "bilibili_qrcode.png")
			SaveQRCode(qr.URL, qrPath)
			PrintQRCodeTerminal(qr.URL)

			// 后台打开浏览器
			go func() {
				cm := cdp.NewChromeManager(
					cdp.WithPort(9222),
					cdp.WithUserDataDir(filepath.Join(cfg.DataDir, "browser_data")),
				)
				cdp.RegisterCleanup(cm)
				if ctx, cancel, err := cm.Connect(); err == nil {
					defer cancel()
					if err := cdp.OpenURL(ctx, qr.URL); err == nil {
						fmt.Fprintf(os.Stderr, "🌐 已在 Chrome 中打开扫码页面\n")
						return
					}
				}
				if err := cdp.OpenURLSystem(qr.URL); err == nil {
					fmt.Fprintf(os.Stderr, "🌐 已在浏览器中打开二维码页面\n")
					return
				}
				fmt.Fprintf(os.Stderr, "💡 请手动打开二维码图片: %s\n", qrPath)
			}()

			fmt.Fprintf(os.Stderr, "⏳ 等待扫码...（最长120秒）\n")

			pollCtx, pollCancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer pollCancel()

			cred, err := auth.PollQRCodeContext(pollCtx, qr.AuthCode, 120*time.Second)
			if err != nil {
				return fmt.Errorf("登录失败: %w", err)
			}

			store.Save(cred)
			fmt.Println("\n✅ ===== 扫码成功! =====")
			fmt.Println("   登录凭据已保存")

			info, _ := auth.GetUserInfo(cred)
			if name, ok := info["name"].(string); ok {
				mid, _ := info["mid"].(float64)
				fmt.Printf("   用户: %s (UID: %.0f)\n", name, mid)
				if cred.TokenInfo.Uname == "" || cred.TokenInfo.Uname != name {
					cred.TokenInfo.Uname = name
				}
				if cred.TokenInfo.Mid == 0 && mid > 0 {
					cred.TokenInfo.Mid = int64(mid)
				}
				store.Save(cred)
			}
			fmt.Println("✅ =====================")
			return nil
		},
	}
}

// ─── WhoAmI ────────────────────────────────────────────────────────────────

func newWhoamiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "查看当前登录的B站账号信息",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			credDir := filepath.Join(cfg.DataDir, "cookies")
			cs := storage.NewCredentialStore(credDir)

			if !cs.Exists() {
				fmt.Println("❌ 未登录")
				fmt.Println("💡 请先执行: ytb login")
				return nil
			}

			var cred auth.LoginInfo
			if err := cs.Load(&cred); err != nil {
				return fmt.Errorf("读取登录凭据失败: %w", err)
			}

			valid, err := auth.ValidateLogin(&cred)
			if err != nil || !valid {
				fmt.Println("❌ 登录已过期，请重新登录")
				fmt.Println("💡 执行: ytb login")
				return nil
			}

			fmt.Println("✅ 登录状态有效")
			if cred.TokenInfo.Uname != "" {
				fmt.Printf("   用户名: %s\n", cred.TokenInfo.Uname)
			}
			if cred.TokenInfo.Mid > 0 {
				fmt.Printf("   UID: %d\n", cred.TokenInfo.Mid)
			}
			return nil
		},
	}
}

// ─── Download ──────────────────────────────────────────────────────────────

func newDownloadCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "download <YouTube URL or video ID>",
		Short: "下载 YouTube 视频",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("请输入 YouTube URL 或视频 ID")
			}
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

			fmt.Printf("⬇️  下载视频: %s\n", cleanURL)
			fmt.Printf("📁 输出目录: %s\n", outputDir)
			fmt.Println()

			result, err := download.Video(cleanURL, outputDir, "en", cookiesPath)
			if err != nil {
				return fmt.Errorf("下载失败: %w", err)
			}

			fmt.Printf("✅ 下载完成\n")
			fmt.Printf("  视频: %s\n", result.VideoPath)
			fmt.Printf("  封面: %s\n", result.CoverPath)
			if result.Info.Title != "" {
				fmt.Printf("  标题: %s\n", result.Info.Title)
			}
			return nil
		},
	}
	cmd.Flags().StringP("output", "o", "", "输出目录")
	return cmd
}

// ─── BCut ASR ──────────────────────────────────────────────────────────────

func newBcutCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "bcut <audio/video file>",
		Aliases: []string{"transcribe"},
		Short:   "使用 Bcut ASR 听录音频",
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

func newWhisperCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "whisper <audio/video file>",
		Aliases: []string{"transcribe-local"},
		Short:   "使用 whisper.cpp 本地听录音频",
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
			cfg := loadConfig()
			srtPath, err := pipeline.ResolveInput(cfg, args[0], "zh-srt")
			if err != nil {
				return err
			}

			output, _ := cmd.Flags().GetString("output")
			if output == "" {
				output = strings.TrimSuffix(srtPath, filepath.Ext(srtPath)) + ".meta.json"
			}

			fmt.Printf("🤖 生成元数据: %s\n", srtPath)
			fmt.Println()

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

			fmt.Printf("✅ 生成完成 (耗时: %v)\n", time.Since(start).Round(time.Second))
			fmt.Printf("  标题: %s\n", meta.Title)
			fmt.Printf("  标签: %s\n", strings.Join(meta.Tags, ", "))
			fmt.Printf("📄 %s\n", output)
			return nil
		},
	}
	cmd.Flags().StringP("output", "o", "", "JSON 输出路径（默认 <字幕>.meta.json）")
	return cmd
}

// ─── Translate ─────────────────────────────────────────────────────────────

func newTranslateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "translate <input.srt>",
		Short: "翻译 SRT 字幕文件",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("请输入 SRT 字幕文件路径")
			}
			cfg := loadConfig()
			inputPath, err := pipeline.ResolveInput(cfg, args[0], "srt")
			if err != nil {
				return err
			}

			sourceLang, _ := cmd.Flags().GetString("source-lang")
			targetLang, _ := cmd.Flags().GetString("target-lang")
			if targetLang == "" {
				targetLang = cfg.EffectiveTranslationTargetLang()
			}

			fmt.Printf("🌐 翻译字幕: %s\n", inputPath)
			fmt.Printf("   %s → %s\n", sourceLang, targetLang)
			fmt.Println()

			start := time.Now()
			result, err := translator.SRTContext(context.Background(), inputPath, sourceLang, targetLang, cfg)
			if err != nil {
				return fmt.Errorf("翻译失败: %w", err)
			}

			fmt.Printf("✅ 翻译完成 (耗时: %v)\n", time.Since(start).Round(time.Second))
			fmt.Printf("📄 %s\n", result)
			return nil
		},
	}
	cmd.Flags().String("source-lang", "en", "源语言")
	cmd.Flags().String("target-lang", "", "目标语言（默认读取配置）")
	return cmd
}

// ─── Tencent TTS ───────────────────────────────────────────────────────────

func newTencentTTSCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "tencent-tts <input.srt>",
		Aliases: []string{"tts"},
		Short:   "腾讯云 TTS 语音合成",
		Long: `读取 SRT 字幕文件，逐条调用腾讯云 TTS 生成 MP3 音频。
按字幕序号命名输出（1.mp3, 2.mp3, ...），供 audio-sync 步骤使用。

环境变量:
  TENCENTCLOUD_SECRET_ID    腾讯云 API 密钥 ID
  TENCENTCLOUD_SECRET_KEY   腾讯云 API 密钥 Key`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("请输入 SRT 字幕文件路径")
			}
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

			fmt.Printf("🎤 腾讯云 TTS 合成: %s\n", srtPath)
			fmt.Printf("   ├ 输出目录: %s\n", outputDir)
			fmt.Printf("   └ 并发数: %d\n", concurrency)
			fmt.Println()

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
			fmt.Printf("\n✅ 合成完成: %d 成功, %d 失败\n", success, failed)
			return nil
		},
	}
	cmd.Flags().StringP("output", "o", "", "音频输出目录")
	cmd.Flags().Int("concurrency", 3, "并发合成数")
	cmd.Flags().Int64("voice", 0, "音色: 0=亲和女声, 1=成熟女声, 2=成熟男声, 3=亲和男声")
	cmd.Flags().Float64("volume", 2, "音量: 0-15")
	cmd.Flags().Float64("speed", 1, "语速: 0-2")
	return cmd
}
