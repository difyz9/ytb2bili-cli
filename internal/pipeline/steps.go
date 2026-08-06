package pipeline

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/zolagz/ytb2bili-go/internal/audiosync"
	"github.com/zolagz/ytb2bili-go/internal/auth"
	"github.com/zolagz/ytb2bili-go/internal/bili"
	"github.com/zolagz/ytb2bili-go/internal/config"
	"github.com/zolagz/ytb2bili-go/internal/download"
	"github.com/zolagz/ytb2bili-go/internal/metadata"
	"github.com/zolagz/ytb2bili-go/internal/storage"
	"github.com/zolagz/ytb2bili-go/internal/transcriber"
	"github.com/zolagz/ytb2bili-go/internal/translator"
	"github.com/zolagz/ytb2bili-go/internal/tts"
	"github.com/zolagz/ytb2bili-go/internal/workflow"
)

type pipelineStep interface {
	Definition() workflow.Step
	Run(context.Context, *PipelineState) error
}

type stepDeps struct {
	config  *config.Config
	tasks   *storage.TaskStore
	history *storage.HistoryStore
}

func buildRegistry(state *PipelineState, deps stepDeps) (*workflow.Registry, error) {
	steps := []pipelineStep{
		&downloadStep{config: deps.config},
		&transcribeStep{config: deps.config},
		&translateStep{config: deps.config},
		&ttsStep{config: deps.config},
		&audioSyncStep{},
		&metadataStep{config: deps.config},
		&uploadStep{config: deps.config, tasks: deps.tasks, history: deps.history},
	}
	definitions := make([]workflow.Step, 0, len(steps))
	for _, implementation := range steps {
		definition := implementation.Definition()
		step := implementation
		definition.Run = func(ctx context.Context, _ *workflow.State) error { return step.Run(ctx, state) }
		definitions = append(definitions, definition)
	}
	return workflow.NewRegistry(definitions...)
}

type downloadStep struct{ config *config.Config }

func (*downloadStep) Definition() workflow.Step {
	return workflow.Step{Name: "download", Description: "下载视频、字幕和封面"}
}
func (s *downloadStep) Run(ctx context.Context, state *PipelineState) error {
	req, result := state.Request, state.Result
	result.DownloadDir = filepath.Join(s.config.EffectiveDownloadDir(), result.ArtifactID())
	cookies := req.CookiesPath
	if cookies == "" {
		cookies = s.config.EffectiveCookiesPath()
	}

	// 幂等：已存在视频文件则跳过下载，只拉取元数据（标题供 metadata 步骤使用）
	if video := existingVideo(result.DownloadDir); video != "" {
		fmt.Printf("  ⏭ 视频已存在，跳过下载: %s\n", filepath.Base(video))
		subtitle := existingFile(result.DownloadDir, result.ArtifactID()+".srt")
		cover := existingFile(result.DownloadDir, "cover.jpg")
		info, err := download.InfoContext(ctx, req.URL, cookies)
		if err != nil {
			return fmt.Errorf("获取视频信息失败: %w", err)
		}
		state.Download = &download.Result{VideoPath: video, SubtitlePath: subtitle, CoverPath: cover, Info: *info}
		result.VideoPath, result.SubtitlePath = video, subtitle
		return nil
	}

	var err error
	state.Download, err = download.VideoContext(ctx, req.URL, result.DownloadDir, req.SourceLang, cookies)
	if err != nil {
		return fmt.Errorf("下载失败: %w", err)
	}
	result.VideoPath, result.SubtitlePath = state.Download.VideoPath, state.Download.SubtitlePath
	return nil
}

type transcribeStep struct{ config *config.Config }

func (*transcribeStep) Definition() workflow.Step {
	return workflow.Step{Name: "transcribe", Description: "获取或生成源字幕", Requires: []string{"download"}}
}
func (s *transcribeStep) Run(ctx context.Context, state *PipelineState) error {
	// 幂等：下载目录已存在 <id>.srt 则跳过转录
	if state.Result.SubtitlePath == "" {
		if existing := existingFile(state.Result.DownloadDir, state.Result.ArtifactID()+".srt"); existing != "" {
			state.Result.SubtitlePath = existing
		}
	}
	if state.Result.SubtitlePath != "" {
		log.Printf("  📝 已有字幕: %s\n", filepath.Base(state.Result.SubtitlePath))
		return nil
	}
	videoSize := "?"
	if fi, err := os.Stat(state.Result.VideoPath); err == nil {
		videoSize = fmt.Sprintf("%.0f MB", float64(fi.Size())/(1024*1024))
	}
	fmt.Printf("  \U0001f399 音频来源: %s (%s)\n", filepath.Base(state.Result.VideoPath), videoSize)
	var err error
	switch selectTranscriberProvider(s.config) {
	case "bcut":
		state.Result.SubtitlePath, err = transcriber.BcutASRContext(ctx, state.Result.VideoPath, state.Result.DownloadDir, state.Result.ArtifactID())
	default: // whisper
		var wcfg *config.WhisperConfig
		if s.config != nil && s.config.Transcriber != nil {
			wcfg = s.config.Transcriber.Whisper
		}
		state.Result.SubtitlePath, err = transcriber.WhisperContext(ctx, wcfg,
			state.Result.VideoPath, state.Result.DownloadDir, state.Result.ArtifactID(), state.Request.SourceLang)
	}
	if err != nil {
		return fmt.Errorf("转写失败: %w", err)
	}
	fmt.Printf("  \u2705 字幕: %s\n", filepath.Base(state.Result.SubtitlePath))
	return nil
}

// selectTranscriberProvider 决定 transcribe 步骤使用的转录器。
// 显式配置 transcriber.provider (bcut/whisper) 时按配置；
// 否则默认本地 whisper.cpp。
func selectTranscriberProvider(cfg *config.Config) string {
	if cfg != nil {
		switch strings.ToLower(strings.TrimSpace(cfg.EffectiveTranscriberProvider())) {
		case "bcut":
			return "bcut"
		}
	}
	return "whisper"
}

type translateStep struct{ config *config.Config }

func (*translateStep) Definition() workflow.Step {
	return workflow.Step{Name: "translate", Description: "翻译字幕", Requires: []string{"transcribe"}}
}
func (s *translateStep) Run(ctx context.Context, state *PipelineState) error {
	log.Printf("  \U0001f310 翻译: %s \u2192 %s", state.Request.SourceLang, state.Request.TargetLang)
	log.Printf("  \U0001f4c4 来源: %s", filepath.Base(state.Result.SubtitlePath))
	// 幂等：<id>.<targetLang>.srt 已存在则跳过翻译
	// 防御：跳过前校验产物内容确实是目标语言——历史 bug 中"假翻译"（英文原文）
	// 被误写入 .zh-Hans.srt 后幂等跳过，导致英文配音/英文投稿（BV1GKMr6fEZy）。
	if existing := existingFile(state.Result.DownloadDir, state.Result.ArtifactID()+"."+state.Request.TargetLang+".srt"); existing != "" {
		if translator.ValidateSRTFile(existing, state.Request.TargetLang) {
			state.Result.SubtitlePath = existing
			log.Printf("  ⏭ 译文已存在: %s", filepath.Base(existing))
			return nil
		}
		log.Printf("  ⚠ 译文文件存在但内容不符合目标语言(%s)，重新翻译: %s", state.Request.TargetLang, filepath.Base(existing))
	}
	translated, err := translator.SRTContext(ctx, state.Result.SubtitlePath, state.Request.SourceLang, state.Request.TargetLang, s.config)
	if err != nil {
		return fmt.Errorf("翻译失败: %w", err)
	}
	state.Result.SubtitlePath = translated
	log.Printf("  \u2705 译文: %s", filepath.Base(state.Result.SubtitlePath))
	return nil
}

type metadataStep struct{ config *config.Config }

// ttsStep 将翻译后的字幕合成为分段配音。
// 优先使用腾讯云 TTS（配置了 TENCENTCLOUD_SECRET_ID/KEY 时），
// 否则回退到本地 IndexTTS 服务（需要 .venv/bin/python3 + IndexTTS2 HTTP API）。
type ttsStep struct{ config *config.Config }

func (*ttsStep) Definition() workflow.Step {
	return workflow.Step{Name: "tts", Description: "合成分段中文配音（腾讯云 TTS / IndexTTS）", Requires: []string{"translate"}}
}

func (s *ttsStep) Run(ctx context.Context, state *PipelineState) error {
	audioDir := filepath.Join(state.Result.DownloadDir, "voice")
	// 防御：合成前校验字幕内容是目标语言。
	// 历史 bug：翻译短路导致 .zh-Hans.srt 内容为英文原文，TTS 据此合成了英文配音（BV1GKMr6fEZy）。
	if state.Result.SubtitlePath != "" && strings.HasSuffix(state.Result.SubtitlePath, "."+state.Request.TargetLang+".srt") {
		if !translator.ValidateSRTFile(state.Result.SubtitlePath, state.Request.TargetLang) {
			return fmt.Errorf("TTS 中止: 字幕文件内容不是目标语言(%s): %s（疑似翻译失败/假翻译，请先重新翻译）", state.Request.TargetLang, filepath.Base(state.Result.SubtitlePath))
		}
	}
	// 幂等：配音片段已存在则跳过合成（避免重复计费）
	if hasVoiceClips(audioDir) {
		fmt.Printf("  ⏭ 配音已存在，跳过合成: %s\n", filepath.Base(audioDir))
		state.AudioDir = audioDir
		return nil
	}
	// 确保目录存在
	if err := os.MkdirAll(audioDir, 0755); err != nil {
		return fmt.Errorf("创建配音目录失败: %w", err)
	}

	switch SelectTTSProvider(s.config) {
	case "tencent":
		return s.runTencent(ctx, state, audioDir)
	default:
		return s.runIndexTTS(ctx, state, audioDir)
	}
}

// SelectTTSProvider 决定 tts 步骤使用的合成器。
// 显式配置 tts.provider (tencent/index) 时强制使用；
// 否则自动检测：有腾讯云凭据 → tencent，无 → index。
func SelectTTSProvider(cfg *config.Config) string {
	if cfg != nil && cfg.TTS != nil {
		switch strings.ToLower(strings.TrimSpace(cfg.TTS.Provider)) {
		case "tencent":
			return "tencent"
		case "index", "index-tts":
			return "index"
		}
	}
	if tencentConfigured(cfg) {
		return "tencent"
	}
	return "index"
}

// tencentConfigured 报告腾讯云 TTS 凭据是否可用（配置或环境变量）。
func tencentConfigured(cfg *config.Config) bool {
	if cfg != nil && cfg.TencentCloud != nil && cfg.TencentCloud.SecretID != "" && cfg.TencentCloud.SecretKey != "" {
		return true
	}
	return os.Getenv("TENCENTCLOUD_SECRET_ID") != "" && os.Getenv("TENCENTCLOUD_SECRET_KEY") != ""
}

// runTencent 使用腾讯云 TTS 逐条合成字幕（输出 <n>.mp3，供 audio-sync 使用）。
func (s *ttsStep) runTencent(ctx context.Context, state *PipelineState, audioDir string) error {
	ttsCfg := tts.FromAppConfig(s.config)
	fmt.Printf("  🎙 使用腾讯云 TTS 合成分段配音\n")
	results, err := tts.SynthesizeSRT(ctx, state.Result.SubtitlePath, audioDir, ttsCfg, 3)
	if err != nil {
		return fmt.Errorf("TTS 合成失败: %w", err)
	}
	ok, skipped := 0, 0
	for _, r := range results {
		switch {
		case r.Err == nil:
			ok++
		case strings.Contains(r.Err.Error(), "空文本"):
			skipped++
		default:
			return fmt.Errorf("TTS 合成失败: 第 %d 条出错: %w", r.Index, r.Err)
		}
	}
	fmt.Printf("  ✅ 腾讯云 TTS 合成完成: %d 成功, %d 跳过空文本\n", ok, skipped)
	state.AudioDir = audioDir
	return nil
}

// runIndexTTS 使用本地 IndexTTS 服务合成（需要 .venv/bin/python3 + IndexTTS2 HTTP API）。
// 服务地址与合成参数从配置 tts.index 读取，未配置时使用默认值 http://localhost:18765。
func (s *ttsStep) runIndexTTS(ctx context.Context, state *PipelineState, audioDir string) error {
	var idxCfg *config.IndexTTSConfig
	if s.config != nil && s.config.TTS != nil && s.config.TTS.Index != nil {
		idxCfg = s.config.TTS.Index
	}
	if err := RunIndexTTSSRT(ctx, state.Result.SubtitlePath, audioDir, idxCfg, s.config.DataDir); err != nil {
		return err
	}
	state.AudioDir = audioDir
	return nil
}

// RunIndexTTSSRT 使用本地 IndexTTS2 HTTP 服务将 SRT 字幕合成为分段配音。
// 独立于 PipelineState，供 pipeline tts 步骤与 CLI `ytb tts` 命令复用。
// idxCfg 为 nil 时使用默认配置；dataDir 用于定位项目 .venv 的 python3。
func RunIndexTTSSRT(ctx context.Context, srtPath, outputDir string, idxCfg *config.IndexTTSConfig, dataDir string) error {
	script := filepath.Join("skills", "audio-video-sync", "scripts", "synthesize_srt.py")

	// 从配置读取 IndexTTS 服务参数（缺失时用默认值兜底）
	if idxCfg == nil {
		idxCfg = config.DefaultIndexTTSConfig()
	}
	apiURL := strings.TrimSpace(idxCfg.APIURL)
	if apiURL == "" {
		apiURL = "http://localhost:18765"
	}
	emotion := strings.TrimSpace(idxCfg.Emotion)
	if emotion == "" {
		emotion = "default"
	}
	concurrency := idxCfg.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}
	retries := idxCfg.Retries
	if retries < 0 {
		retries = 3
	}
	timeout := idxCfg.Timeout
	if timeout <= 0 {
		timeout = 180
	}

	args := []string{
		script,
		"--srt", srtPath,
		"--output-dir", outputDir,
		"--api-url", apiURL,
		"--concurrency", fmt.Sprintf("%d", concurrency),
		"--retries", fmt.Sprintf("%d", retries),
		"--timeout", fmt.Sprintf("%.0f", timeout),
		"--emotion", emotion,
		"--emo-alpha", fmt.Sprintf("%.2f", idxCfg.EmotionAlpha),
	}
	if idxCfg.RefAudio != "" {
		args = append(args, "--ref-audio", idxCfg.RefAudio)
	}
	if idxCfg.UseEmoText {
		args = append(args, "--use-emo-text")
	}
	if idxCfg.EmoText != "" {
		args = append(args, "--emo-text", idxCfg.EmoText)
	}
	if idxCfg.ServerOutputDir != "" {
		args = append(args, "--server-output-dir", idxCfg.ServerOutputDir)
	}

	fmt.Printf("  🎙 使用本地 IndexTTS 合成分段配音 (%s)\n", apiURL)
	cmd := exec.CommandContext(ctx, filepath.Join(dataDir, "..", ".venv", "bin", "python3"), args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("TTS 合成失败: %w\n输出: %s", err, string(output))
	}
	fmt.Printf("   %s", string(output))
	return nil
}

type audioSyncStep struct{}

func (*audioSyncStep) Definition() workflow.Step {
	return workflow.Step{Name: "audio-sync", Description: "按字幕时间轴对齐分段配音并替换视频音轨；需要请求提供 audio-dir", Requires: []string{"translate"}}
}
func (*audioSyncStep) Run(ctx context.Context, state *PipelineState) error {
	baseName := state.Result.VideoID
	if baseName == "" {
		baseName = state.Result.TaskID
	}
	output := filepath.Join(state.Result.DownloadDir, baseName+".synced.mp4")
	// 幂等：音画同步结果已存在则跳过
	if fi, err := os.Stat(output); err == nil && !fi.IsDir() && fi.Size() > 0 {
		fmt.Printf("  ⏭ 音画同步结果已存在: %s\n", filepath.Base(output))
		state.Result.SyncedVideoPath = output
		state.Result.VideoPath = output
		return nil
	}
	audioDir := state.Request.AudioDir
	if audioDir == "" && state.AudioDir != "" {
		audioDir = state.AudioDir
	}
	if audioDir == "" {
		return fmt.Errorf("audio-sync 需要通过 --audio-dir 提供按字幕编号命名的配音目录，或先执行 tts 步骤")
	}
	result, err := audiosync.Sync(ctx, audiosync.Options{VideoPath: state.Result.VideoPath, SubtitlePath: state.Result.SubtitlePath, AudioDir: audioDir, OutputPath: output, DisableSpeedAdjust: state.Request.DisableAudioSpeedAdjust, MissingMode: state.Request.AudioMissingMode})
	if err != nil {
		return err
	}
	state.Result.SyncedVideoPath = result.Output
	state.Result.VideoPath = result.Output
	return nil
}

func (*metadataStep) Definition() workflow.Step {
	return workflow.Step{Name: "metadata", Description: "生成标题、简介和标签", Requires: []string{"download"}}
}
func (s *metadataStep) Run(ctx context.Context, state *PipelineState) error {
	fmt.Printf("  \U0001f916 原始标题: %s\n", state.Download.Info.Title)
	meta, err := metadata.GenerateContext(ctx, state.Download.Info, s.config)
	if err != nil {
		meta = &metadata.VideoMeta{Title: metadata.ClampTitle(state.Download.Info.Title), Description: state.Download.Info.Description}
		fmt.Printf("  \u26a0 AI 生成失败，使用原标题\n")
	}
	fmt.Printf("  \u2705 中文标题: %s\n", meta.Title)
	state.Metadata, state.Result.Metadata = meta, meta
	return nil
}

type uploadStep struct {
	config  *config.Config
	tasks   *storage.TaskStore
	history *storage.HistoryStore
}

func (*uploadStep) Definition() workflow.Step {
	return workflow.Step{Name: "upload", Description: "投稿到 B站并记录历史", Requires: []string{"download", "metadata"}}
}
func (s *uploadStep) Run(ctx context.Context, state *PipelineState) error {
	var cred auth.LoginInfo
	if err := storage.NewCredentialStore(filepath.Join(s.config.DataDir, "cookies")).Load(&cred); err != nil {
		return fmt.Errorf("请先登录: ytb2bili login")
	}
	r, req := state.Result, state.Request
	videoSize := "?"
	if fi, err := os.Stat(r.VideoPath); err == nil {
		videoSize = fmt.Sprintf("%.0f MB", float64(fi.Size())/(1024*1024))
	}
	fmt.Printf("  \U0001f4e4 上传视频: %s (%s)\n", filepath.Base(r.VideoPath), videoSize)
	fmt.Printf("  \U0001f3a8 标题: %s\n", state.Metadata.Title)
	bvid, err := bili.UploadContext(ctx, &cred, &bili.UploadParams{VideoPath: r.VideoPath, Title: state.Metadata.Title, Desc: state.Metadata.Description, Tags: state.Metadata.Tags, Source: req.URL, Tid: req.Tid, CoverPath: state.Download.CoverPath})
	if err != nil {
		return fmt.Errorf("上传失败: %w", err)
	}
	r.BVID = bvid
	s.tasks.SetBVID(r.TaskID, bvid)
	if err = s.history.Add(&storage.SubmittedVideo{YouTubeID: r.VideoID, BVID: bvid, Title: state.Metadata.Title, Channel: req.Source}); err != nil {
		return fmt.Errorf("保存投稿历史失败: %w", err)
	}
	_, _ = storage.NewSubtitleStore(filepath.Join(s.config.DataDir, "subtitles")).SyncFromDownload(r.ArtifactID(), bvid, r.DownloadDir)
	fmt.Printf("  \u2705 B站: https://www.bilibili.com/video/%s\n", bvid)
	return nil
}
