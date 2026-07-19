package audiosync

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type Options struct {
	VideoPath, SubtitlePath, AudioDir, OutputPath string
	DisableSpeedAdjust                            bool
	MissingMode                                   string
}

type Result struct {
	Output                   string  `json:"output"`
	Duration                 float64 `json:"duration"`
	Clips, Adjusted, Missing int
}

func Sync(ctx context.Context, options Options) (*Result, error) {
	if options.VideoPath == "" || options.SubtitlePath == "" || options.AudioDir == "" || options.OutputPath == "" {
		return nil, fmt.Errorf("audio sync requires video, subtitle, audio directory, and output")
	}
	if options.MissingMode != "" && options.MissingMode != "error" && options.MissingMode != "silence" {
		return nil, fmt.Errorf("invalid missing audio mode %q", options.MissingMode)
	}
	script, err := scriptPath()
	if err != nil {
		return nil, err
	}
	python := os.Getenv("YTB2BILI_PYTHON")
	if python == "" {
		python = "python3"
	}
	args := []string{script, "--video", options.VideoPath, "--srt", options.SubtitlePath, "--audio-dir", options.AudioDir, "--output", options.OutputPath}
	if options.DisableSpeedAdjust {
		args = append(args, "--no-speed-adjust")
	}
	if options.MissingMode != "" {
		args = append(args, "--missing", options.MissingMode)
	}
	cmd := exec.CommandContext(ctx, python, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("音画同步脚本失败: %w: %s", err, tail(string(output), 1200))
	}
	var last string
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) != "" {
			last = scanner.Text()
		}
	}
	var result Result
	if err := json.Unmarshal([]byte(last), &result); err != nil {
		return nil, fmt.Errorf("解析音画同步结果失败: %w", err)
	}
	if result.Output == "" {
		return nil, fmt.Errorf("音画同步脚本未返回输出文件")
	}
	if info, err := os.Stat(result.Output); err != nil || info.IsDir() {
		return nil, fmt.Errorf("音画同步输出文件无效: %s", result.Output)
	}
	return &result, nil
}

func scriptPath() (string, error) {
	if configured := os.Getenv("YTB2BILI_AUDIO_SYNC_SCRIPT"); configured != "" {
		if info, err := os.Stat(configured); err == nil && !info.IsDir() {
			return configured, nil
		}
		return "", fmt.Errorf("YTB2BILI_AUDIO_SYNC_SCRIPT 不存在: %s", configured)
	}
	var candidates []string
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, "skills", "audio-video-sync", "scripts", "audio_processor_v2.py"))
	}
	if executable, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(executable), "skills", "audio-video-sync", "scripts", "audio_processor_v2.py"))
	}
	if _, file, _, ok := runtime.Caller(0); ok {
		candidates = append(candidates, filepath.Join(filepath.Dir(file), "..", "..", "skills", "audio-video-sync", "scripts", "audio_processor_v2.py"))
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			absolute, _ := filepath.Abs(candidate)
			return absolute, nil
		}
	}
	return "", fmt.Errorf("未找到 audio-video-sync Skill 脚本；可设置 YTB2BILI_AUDIO_SYNC_SCRIPT")
}

func tail(value string, max int) string {
	if len(value) > max {
		return value[len(value)-max:]
	}
	return value
}
