package download

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type Result struct {
	VideoPath    string
	SubtitlePath string
	CoverPath    string
	Info         VideoInfo
}

type VideoInfo struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Duration    int    `json:"duration"`
	Thumbnail   string `json:"thumbnail"`
	ID          string `json:"id"`
}

func Video(url, outputDir, lang string, cookiesPath ...string) (*Result, error) {
	return VideoContext(context.Background(), url, outputDir, lang, cookiesPath...)
}

func VideoContext(ctx context.Context, url, outputDir, lang string, cookiesPath ...string) (*Result, error) {
	os.MkdirAll(outputDir, 0755)

	// Check yt-dlp
	if _, err := exec.LookPath("yt-dlp"); err != nil {
		return nil, fmt.Errorf("yt-dlp 未安装，请先安装: sudo curl -L https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp -o /usr/local/bin/yt-dlp && sudo chmod a+rx /usr/local/bin/yt-dlp")
	}

	// Ensure deno is in PATH for yt-dlp YouTube JS challenges
	env := os.Environ()
	denoPath := os.Getenv("DENO_PATH")
	if denoPath == "" {
		denoPath = filepath.Join(os.Getenv("HOME"), ".deno", "bin")
	}
	if runtime.GOOS != "windows" {
		pathExists := false
		for _, p := range strings.Split(os.Getenv("PATH"), ":") {
			if p == denoPath {
				pathExists = true
				break
			}
		}
		if !pathExists {
			env = append(env, "PATH="+denoPath+":"+os.Getenv("PATH"))
		}
	}

	// Resolve cookies: prefer explicit cookie file, fallback to browser session
	cookiesFile := ""
	if len(cookiesPath) > 0 && cookiesPath[0] != "" {
		cookiesFile = cookiesPath[0]
	}
	// If the provided cookie file is empty or has no valid entries, try global YOUTUBE_COOKIES env var.
	if cookiesFile == "" || !hasValidCookies(cookiesFile) {
		if global := os.Getenv("YOUTUBE_COOKIES"); global != "" {
			if _, err := os.Stat(global); err == nil {
				cookiesFile = global
			}
		}
	}

	// On macOS, prefer --cookies-from-browser chrome over a stale cookies file,
	// since Chrome keeps an active YouTube login session.  Set
	// YOUTUBE_COOKIES_FROM_BROWSER to override the browser/profile syntax accepted
	// by yt-dlp, or to "off" to disable browser-cookie discovery entirely.
	baseArgs := cookieArgs(cookiesFile, os.Getenv("YOUTUBE_COOKIES_FROM_BROWSER"), runtime.GOOS)

	// Get video info first
	infoArgs := append(baseArgs, "--dump-json", "--no-download", "--remote-components", "ejs:github", url)
	infoCmd := exec.CommandContext(ctx, "yt-dlp", infoArgs...)
	infoCmd.Env = env
	var infoStderr bytes.Buffer
	infoCmd.Stderr = &infoStderr
	infoOut, err := infoCmd.Output()
	if err != nil {
		stderrStr := strings.TrimSpace(infoStderr.String())
		// Extract the most relevant error line (first ERROR: line)
		errMsg := err.Error()
		for _, line := range strings.Split(stderrStr, "\n") {
			if strings.HasPrefix(line, "ERROR:") {
				errMsg = strings.TrimSpace(line)
				break
			}
		}
		if errMsg == err.Error() && stderrStr != "" {
			lines := strings.Split(stderrStr, "\n")
			errMsg = strings.TrimSpace(lines[len(lines)-1])
		}
		return nil, fmt.Errorf("获取视频信息失败: %s", errMsg)
	}

	var info VideoInfo
	if err := json.Unmarshal(infoOut, &info); err != nil {
		// Try just the first line in case yt-dlp prefix metadata on the first line
		lines := strings.SplitN(string(infoOut), "\n", 2)
		if len(lines) > 0 {
			if err2 := json.Unmarshal([]byte(lines[0]), &info); err2 != nil {
				return nil, fmt.Errorf("解析视频信息 JSON 失败: %w (second attempt: %v)", err, err2)
			}
		} else {
			return nil, fmt.Errorf("解析视频信息 JSON 失败: %w", err)
		}
	}

	// Check subtitle languages
	// Download video (字幕通过 BCut ASR 单独听录，不使用 yt-dlp 下载的字幕)
	template := filepath.Join(outputDir, "%(id)s.%(ext)s")

	args := append(baseArgs,
		"-f", "bestvideo[ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]/best",
		"--merge-output-format", "mp4",
		"--embed-metadata",
		"--ignore-errors",
		"--remote-components", "ejs:github",
		"--retries", "5",
		"--extractor-retries", "5",
		"--retry-sleep", "exp=2:5",
		"-o", template,
		"--print", "after_move:video_path:{filepath}",
		url,
	)

	cmd := exec.CommandContext(ctx, "yt-dlp", args...)
	cmd.Stdout = log.Writer()
	cmd.Stderr = log.Writer()
	cmd.Env = env
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("下载失败: %w", err)
	}

	// Find video file
	var videoPath string
	entries, _ := os.ReadDir(outputDir)
	for _, e := range entries {
		if !e.IsDir() && (strings.HasSuffix(e.Name(), ".mp4") || strings.HasSuffix(e.Name(), ".mkv") || strings.HasSuffix(e.Name(), ".webm")) {
			videoPath = filepath.Join(outputDir, e.Name())
			break
		}
	}
	if videoPath == "" {
		return nil, fmt.Errorf("未找到下载的视频文件")
	}

	// Find subtitle：已不再使用 yt-dlp 下载的字幕，统一由 BCut ASR 听录
	var srtPath string

	// Download cover
	var coverPath string
	if info.Thumbnail != "" {
		coverPath = filepath.Join(outputDir, "cover.jpg")
		coverCmd := exec.CommandContext(ctx, "curl", "-sL", "-o", coverPath, info.Thumbnail)
		if coverCmd.Run() == nil {
			if fi, err := os.Stat(coverPath); err != nil || fi.Size() == 0 {
				coverPath = ""
			}
		} else {
			coverPath = ""
		}
	}

	return &Result{
		VideoPath:    videoPath,
		SubtitlePath: srtPath,
		CoverPath:    coverPath,
		Info:         info,
	}, nil
}

func cookieArgs(cookiesFile, browser, goos string) []string {
	if cookiesFile != "" && hasValidCookies(cookiesFile) {
		return []string{"--cookies", cookiesFile}
	}

	browser = strings.TrimSpace(browser)
	if strings.EqualFold(browser, "off") || strings.EqualFold(browser, "none") {
		return nil
	}
	if browser == "" && goos == "darwin" {
		browser = "chrome"
	}
	if browser != "" {
		return []string{"--cookies-from-browser", browser}
	}
	return nil
}

// hasValidCookies checks if a Netscape cookies file has non-zero expiry timestamps
func hasValidCookies(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	lines := strings.Split(string(data), "\n")
	validCount := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) >= 5 {
			if fields[4] != "0" {
				validCount++
			}
		}
	}
	return validCount > 0
}
