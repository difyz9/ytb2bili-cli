package audiosync

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/zolagz/ytb2bili-go/internal/resource"
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
	python := pythonPath()
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

// pythonPath 返回运行音画同步脚本的 Python 解释器路径。
// 优先级：$YTB2BILI_PYTHON > 项目根 .venv/bin/python3（存在时）> python3。
// 与 scriptPath 的候选搜索方式一致，支持从任意 cwd 运行。
func pythonPath() string {
	if configured := os.Getenv("YTB2BILI_PYTHON"); configured != "" {
		return configured
	}
	var candidates []string
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, ".venv", "bin", "python3"))
	}
	if executable, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(executable), ".venv", "bin", "python3"))
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return "python3"
}

func scriptPath() (string, error) {
	if configured := os.Getenv("YTB2BILI_AUDIO_SYNC_SCRIPT"); configured != "" {
		if info, err := os.Stat(configured); err == nil && !info.IsDir() {
			return configured, nil
		}
		return "", fmt.Errorf("YTB2BILI_AUDIO_SYNC_SCRIPT 不存在: %s", configured)
	}
	// 统一走 resource 包定位（config skills_dir > $YTB2BILI_PROJECT_DIR > exe/cwd 自动探测）
	script := resource.SkillScript(filepath.Join("audio-video-sync", "scripts", "audio_processor_v2.py"))
	if info, err := os.Stat(script); err == nil && !info.IsDir() {
		return script, nil
	}
	return "", fmt.Errorf(
		"找不到 audio_processor_v2.py（解析到 %s）；可用 config.yaml 的 skills_dir 或环境变量 YTB2BILI_PROJECT_DIR/YTB2BILI_AUDIO_SYNC_SCRIPT 指定", script)
}

func tail(value string, max int) string {
	if len(value) > max {
		return value[len(value)-max:]
	}
	return value
}
