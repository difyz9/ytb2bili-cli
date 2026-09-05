package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zolagz/ytb2bili-go/internal/config"
)

// existingVideo 返回目录中第一个视频文件路径（.mp4/.mkv/.webm，排除 *.synced.mp4），无则空。
func existingVideo(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".synced.mp4") {
			continue
		}
		if strings.HasSuffix(name, ".mp4") || strings.HasSuffix(name, ".mkv") || strings.HasSuffix(name, ".webm") {
			return filepath.Join(dir, name)
		}
	}
	return ""
}

// existingFile 返回目录中指定文件的路径，不存在则空。
func existingFile(dir, name string) string {
	path := filepath.Join(dir, name)
	if fi, err := os.Stat(path); err == nil && !fi.IsDir() {
		return path
	}
	return ""
}

// hasVoiceClips 报告 voice 目录是否存在至少一个非空音频文件。
func hasVoiceClips(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if fi, err := os.Stat(path); err == nil && fi.Size() > 0 {
			return true
		}
	}
	return false
}

// ResolveSyncArtifacts 从视频目录解析 audio-sync 所需的已有产物：
// 视频（排除 *.synced.mp4）、字幕（优先 <id>.zh-Hans.srt，回退 <id>.srt）、voice 配音目录。
func ResolveSyncArtifacts(videoDir, videoID string) (video, subtitle, voiceDir string, err error) {
	video = existingVideo(videoDir)
	if video == "" {
		return "", "", "", fmt.Errorf("未找到视频文件: %s", videoDir)
	}
	subtitle = existingFile(videoDir, videoID+".zh-Hans.srt")
	if subtitle == "" {
		subtitle = existingFile(videoDir, videoID+".srt")
	}
	if subtitle == "" {
		return "", "", "", fmt.Errorf("未找到字幕文件（需要 %s.zh-Hans.srt 或 %s.srt）", videoID, videoID)
	}
	voiceDir = filepath.Join(videoDir, "voice")
	if !hasVoiceClips(voiceDir) {
		return "", "", "", fmt.Errorf("未找到配音目录 voice/（需要先执行 tts 步骤）")
	}
	return video, subtitle, voiceDir, nil
}

// ResolveVideoDir 将命令参数解析为视频所在目录：
//   - 参数是已存在目录 → 直接用
//   - 参数是已存在文件 → 取其所在目录
//   - 否则视为 videoId → 返回 <download_dir>/<videoId>
func ResolveVideoDir(cfg *config.Config, arg string) string {
	if fi, err := os.Stat(arg); err == nil {
		if fi.IsDir() {
			return arg
		}
		return filepath.Dir(arg)
	}
	return filepath.Join(cfg.EffectiveDownloadDir(), arg)
}

// ResolveInput 将命令参数解析为具体资源文件：已存在路径直接用；
// 否则视为 videoId，在下载目录中按 kind 定位：
//   - "video"   视频文件（排除 *.synced.mp4）
//   - "srt"     <videoId>.srt 源字幕
//   - "zh-srt"  优先 <videoId>.zh-Hans.srt，回退 <videoId>.srt
func ResolveInput(cfg *config.Config, arg, kind string) (string, error) {
	if _, err := os.Stat(arg); err == nil {
		return arg, nil
	}
	dir := filepath.Join(cfg.EffectiveDownloadDir(), arg)
	switch kind {
	case "video":
		if v := existingVideo(dir); v != "" {
			return v, nil
		}
		return "", fmt.Errorf("未找到视频文件: %s（请先 ytb submit/download，或直接传入视频路径）", dir)
	case "zh-srt":
		if s := existingFile(dir, arg+".zh-Hans.srt"); s != "" {
			return s, nil
		}
		if s := existingFile(dir, arg+".srt"); s != "" {
			return s, nil
		}
		return "", fmt.Errorf("未找到译文字幕: %s（需要 %s.zh-Hans.srt 或 %s.srt）", dir, arg, arg)
	default: // "srt"
		if s := existingFile(dir, arg+".srt"); s != "" {
			return s, nil
		}
		return "", fmt.Errorf("未找到源字幕: %s（需要 %s.srt）", dir, arg)
	}
}
