package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// ResolveSyncArtifacts 解析 audio-sync 所需的已有产物：
// 视频（排除 *.synced.mp4）、字幕（优先 <id>.zh-Hans.srt，回退 <id>.srt）、voice 配音目录。
func ResolveSyncArtifacts(dataDir, videoID string) (video, subtitle, voiceDir string, err error) {
	dlDir := filepath.Join(dataDir, "downloads", videoID)
	video = existingVideo(dlDir)
	if video == "" {
		return "", "", "", fmt.Errorf("未找到视频文件: %s", dlDir)
	}
	subtitle = existingFile(dlDir, videoID+".zh-Hans.srt")
	if subtitle == "" {
		subtitle = existingFile(dlDir, videoID+".srt")
	}
	if subtitle == "" {
		return "", "", "", fmt.Errorf("未找到字幕文件（需要 %s.zh-Hans.srt 或 %s.srt）", videoID, videoID)
	}
	voiceDir = filepath.Join(dlDir, "voice")
	if !hasVoiceClips(voiceDir) {
		return "", "", "", fmt.Errorf("未找到配音目录 voice/（需要先执行 tts 步骤）")
	}
	return video, subtitle, voiceDir, nil
}
