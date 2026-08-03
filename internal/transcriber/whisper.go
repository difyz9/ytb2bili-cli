package transcriber

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/zolagz/ytb2bili-go/internal/config"
)

// WhisperContext 使用本地 whisper.cpp（whisper-cli 子进程）听录音频并生成 SRT 字幕。
//
// videoPath 可以是任意 ffmpeg 可读的音频/视频文件；生成的 SRT 位于
// <outputDir>/<videoID>.srt（与 BcutASR 的命名约定一致，供下游 translate/tts/audio-sync 直接使用）。
//
// language: 语言代码（en/zh/...）；空或 "auto" 时交给 whisper 自动检测。
func WhisperContext(ctx context.Context, wcfg *config.WhisperConfig, videoPath, outputDir, videoID, language string) (string, error) {
	if wcfg == nil {
		wcfg = &config.WhisperConfig{}
	}
	binary := strings.TrimSpace(wcfg.Binary)
	model := strings.TrimSpace(wcfg.Model)
	threads := wcfg.Threads
	if binary == "" {
		binary = "whisper-cli"
	}
	if model == "" {
		model = "models/ggml-base.bin"
	}
	if threads <= 0 {
		threads = 4
	}
	// 支持 ~/ 开头的主目录路径（config 或 --model flag 均可写 ~/...）
	model = config.ExpandHome(model)
	if videoID == "" {
		videoID = "subtitle"
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("创建输出目录失败: %w", err)
	}

	// 前置检查：二进制与模型必须可用（给出安装/下载提示，而不是 whisper-cli 晦涩的报错）
	if _, err := exec.LookPath(binary); err != nil {
		return "", fmt.Errorf("未找到 %s，请先安装 whisper.cpp（macOS: brew install whisper-cpp；Debian/Ubuntu: sudo apt install whisper-cpp）", binary)
	}
	if _, err := os.Stat(model); err != nil {
		return "", fmt.Errorf("whisper 模型不存在: %s\n  请下载: mkdir -p %s && curl -L -o %s https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-base.bin", model, filepath.Dir(model), model)
	}

	// Step 1: 提取 16kHz 单声道 PCM WAV（whisper 最佳输入，参考 whisper_with_go ConvertToWAV）
	// 用唯一临时文件名，避免与输入文件同名（如输入本身是 <videoID>.wav）导致 ffmpeg 拒绝原地覆盖。
	tmpWav, err := os.CreateTemp(outputDir, videoID+".*.wav")
	if err != nil {
		return "", fmt.Errorf("创建临时音频失败: %w", err)
	}
	wavPath := tmpWav.Name()
	tmpWav.Close()
	defer os.Remove(wavPath)
	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", videoPath,
		"-vn", "-ar", "16000", "-ac", "1", "-c:a", "pcm_s16le", wavPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("提取音频失败: %s", string(out))
	}

	// Step 2: 调用 whisper-cli 输出 SRT（-of 不带扩展名，whisper-cli 会写成 <of>.srt）
	lang := strings.TrimSpace(language)
	if lang == "" {
		lang = "auto"
	}
	args := []string{
		"-m", model,
		"-t", strconv.Itoa(threads),
		"-l", lang,
		"-osrt",
		"-of", filepath.Join(outputDir, videoID),
		wavPath,
	}
	log.Printf("  运行 %s (model=%s, lang=%s)...", binary, filepath.Base(model), lang)
	cmd = exec.CommandContext(ctx, binary, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("whisper-cli 转写失败: %s\n%s", err, strings.TrimSpace(string(out)))
	}

	srtPath := filepath.Join(outputDir, videoID+".srt")
	if _, statErr := os.Stat(srtPath); statErr != nil {
		// 回退：某些 whisper-cli 版本未生成文件时，从控制台时间戳行解析
		segments := parseWhisperConsoleOutput(string(out))
		if len(segments) == 0 {
			return "", fmt.Errorf("whisper-cli 未生成字幕文件且无法从输出解析: %s", strings.TrimSpace(string(out)))
		}
		if err := generateSRT(&bcutResult{Utterances: segments}, srtPath); err != nil {
			return "", err
		}
	}
	return srtPath, nil
}

// whisperSegmentRe 匹配 whisper-cli 控制台输出行，例如：
//
//	[00:00:00.000 --> 00:00:02.500]   Hello world
var whisperSegmentRe = regexp.MustCompile(`\[\s*(\d{2}):(\d{2}):(\d{2})\.(\d{3})\s*-->\s*(\d{2}):(\d{2}):(\d{2})\.(\d{3})\s*\]\s*(.*)`)

// parseWhisperConsoleOutput 从 whisper-cli 的 stdout 中提取带时间戳的转写片段。
func parseWhisperConsoleOutput(output string) []Segment {
	var segments []Segment
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		m := whisperSegmentRe.FindStringSubmatch(scanner.Text())
		if m == nil {
			continue
		}
		text := strings.TrimSpace(m[9])
		if text == "" {
			continue
		}
		segments = append(segments, Segment{
			Transcript: text,
			StartTime:  timestampToSeconds(m[1], m[2], m[3], m[4]),
			EndTime:    timestampToSeconds(m[5], m[6], m[7], m[8]),
		})
	}
	return segments
}

// timestampToSeconds 将时分秒毫秒转成秒（float64），供 generateSRT 使用。
func timestampToSeconds(h, m, s, ms string) float64 {
	hh, _ := strconv.Atoi(h)
	mm, _ := strconv.Atoi(m)
	ss, _ := strconv.Atoi(s)
	mmm, _ := strconv.Atoi(ms)
	return float64(hh*3600+mm*60+ss) + float64(mmm)/1000.0
}
