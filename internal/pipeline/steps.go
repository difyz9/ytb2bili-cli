package pipeline

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/zolagz/ytb2bili-go/internal/audiosync"
	"github.com/zolagz/ytb2bili-go/internal/auth"
	"github.com/zolagz/ytb2bili-go/internal/bili"
	"github.com/zolagz/ytb2bili-go/internal/config"
	"github.com/zolagz/ytb2bili-go/internal/download"
	"github.com/zolagz/ytb2bili-go/internal/metadata"
	"github.com/zolagz/ytb2bili-go/internal/storage"
	"github.com/zolagz/ytb2bili-go/internal/transcriber"
	"github.com/zolagz/ytb2bili-go/internal/translator"
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
		&transcribeStep{},
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
	result.DownloadDir = filepath.Join(s.config.DataDir, "downloads", result.ArtifactID())
	cookies := req.CookiesPath
	if cookies == "" {
		cookies = s.config.YouTubeCookies
	}
	if cookies == "" {
		cookies = filepath.Join(s.config.DataDir, "youtube_cookies.txt")
	}
	var err error
	state.Download, err = download.VideoContext(ctx, req.URL, result.DownloadDir, req.SourceLang, cookies)
	if err != nil {
		return fmt.Errorf("下载失败: %w", err)
	}
	result.VideoPath, result.SubtitlePath = state.Download.VideoPath, state.Download.SubtitlePath
	return nil
}

type transcribeStep struct{}

func (*transcribeStep) Definition() workflow.Step {
	return workflow.Step{Name: "transcribe", Description: "获取或生成源字幕", Requires: []string{"download"}}
}
func (*transcribeStep) Run(ctx context.Context, state *PipelineState) error {
	if state.Result.SubtitlePath != "" {
		return nil
	}
	var err error
	state.Result.SubtitlePath, err = transcriber.BcutASRContext(ctx, state.Result.VideoPath, state.Result.DownloadDir, state.Result.ArtifactID())
	if err != nil {
		return fmt.Errorf("转写失败: %w", err)
	}
	return nil
}

type translateStep struct{ config *config.Config }

func (*translateStep) Definition() workflow.Step {
	return workflow.Step{Name: "translate", Description: "翻译字幕", Requires: []string{"transcribe"}}
}
func (s *translateStep) Run(ctx context.Context, state *PipelineState) error {
	translated, err := translator.SRTContext(ctx, state.Result.SubtitlePath, state.Request.SourceLang, state.Request.TargetLang, s.config)
	if err != nil {
		return fmt.Errorf("翻译失败: %w", err)
	}
	state.Result.SubtitlePath = translated
	return nil
}

type metadataStep struct{ config *config.Config }

// ttsStep 使用 IndexTTS 将翻译后的字幕合成为中文配音片段
type ttsStep struct{ config *config.Config }

func (*ttsStep) Definition() workflow.Step {
	return workflow.Step{Name: "tts", Description: "使用 IndexTTS 合成分段中文配音", Requires: []string{"translate"}}
}

func (s *ttsStep) Run(ctx context.Context, state *PipelineState) error {
	script := filepath.Join("skills", "audio-video-sync", "scripts", "synthesize_srt.py")
	audioDir := filepath.Join(state.Result.DownloadDir, "voice")
	// 确保目录存在
	if err := os.MkdirAll(audioDir, 0755); err != nil {
		return fmt.Errorf("创建配音目录失败: %w", err)
	}

	cmd := exec.CommandContext(ctx, filepath.Join(s.config.DataDir, "..", ".venv", "bin", "python3"), script,
		"--srt", state.Result.SubtitlePath,
		"--output-dir", audioDir,
		"--api-url", "http://localhost:18765",
		"--concurrency", "1",
		"--retries", "3",
		"--timeout", "180",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("TTS 合成失败: %w\n输出: %s", err, string(output))
	}
	fmt.Printf("   %s", string(output))
	state.AudioDir = audioDir
	return nil
}

type audioSyncStep struct{}

func (*audioSyncStep) Definition() workflow.Step {
	return workflow.Step{Name: "audio-sync", Description: "按字幕时间轴对齐分段配音并替换视频音轨；需要请求提供 audio-dir", Requires: []string{"translate"}}
}
func (*audioSyncStep) Run(ctx context.Context, state *PipelineState) error {
	audioDir := state.Request.AudioDir
	if audioDir == "" && state.AudioDir != "" {
		audioDir = state.AudioDir
	}
	if audioDir == "" {
		return fmt.Errorf("audio-sync 需要通过 --audio-dir 提供按字幕编号命名的配音目录，或先执行 tts 步骤")
	}
	baseName := state.Result.VideoID
	if baseName == "" {
		baseName = state.Result.TaskID
	}
	output := filepath.Join(state.Result.DownloadDir, baseName+".synced.mp4")
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
	meta, err := metadata.GenerateContext(ctx, state.Download.Info, s.config)
	if err != nil {
		meta = &metadata.VideoMeta{Title: state.Download.Info.Title, Description: state.Download.Info.Description}
	}
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
	return nil
}
