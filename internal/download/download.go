package download

import (
	"encoding/json"
	"fmt"
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
	os.MkdirAll(outputDir, 0755)

	// Check yt-dlp
	if _, err := exec.LookPath("yt-dlp"); err != nil {
		return nil, fmt.Errorf("yt-dlp 未安装，请先安装: sudo curl -L https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp -o /usr/local/bin/yt-dlp && sudo chmod a+rx /usr/local/bin/yt-dlp")
	}

	// Ensure deno is in PATH for yt-dlp YouTube JS challenges
	env := os.Environ()
	denoPath := "/home/ubuntu/.deno/bin"
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

	// Resolve cookies: prefer task cookies, fallback to global cookies
	cookiesFile := ""
	if len(cookiesPath) > 0 && cookiesPath[0] != "" {
		cookiesFile = cookiesPath[0]
	}
	// Fallback to global YouTube cookies if task cookies are empty or have expire=0
	if cookiesFile == "" || !hasValidCookies(cookiesFile) {
		globalCookies := os.Getenv("YOUTUBE_COOKIES")
		if globalCookies == "" {
			globalCookies = "/home/ubuntu/ytb2bili-cli/cookies/youtube_cookies.txt"
		}
		if _, err := os.Stat(globalCookies); err == nil {
			cookiesFile = globalCookies
		}
	}

	// Build base args
	baseArgs := []string{}
	if cookiesFile != "" {
		baseArgs = append(baseArgs, "--cookies", cookiesFile)
	}

	// Get video info first
	infoArgs := append(baseArgs, "--dump-json", "--no-download", url)
	infoCmd := exec.Command("yt-dlp", infoArgs...)
	infoCmd.Env = env
	infoOut, err := infoCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("获取视频信息失败: %w", err)
	}

	var info VideoInfo
	if err := json.Unmarshal(infoOut, &info); err != nil {
		// Just use first line
		lines := strings.SplitN(string(infoOut), "\n", 2)
		json.Unmarshal([]byte(lines[0]), &info)
	}

	// Check subtitle languages
	subLangs := fmt.Sprintf("%s,zh-Hans,zh-Hant,ja", lang)

	// Download video
	template := filepath.Join(outputDir, "%(id)s.%(ext)s")

	args := append(baseArgs,
		"-f", "bestvideo[ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]/best",
		"--write-auto-sub", "--sub-langs", subLangs,
		"--convert-subs", "srt",
		"--embed-metadata",
		"-o", template,
		"--print", "after_move:video_path:{filepath}",
		url,
	)

	cmd := exec.Command("yt-dlp", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
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

	// Find subtitle
	var srtPath string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".srt") {
			srtPath = filepath.Join(outputDir, e.Name())
			break
		}
	}

	// Download cover
	var coverPath string
	if info.Thumbnail != "" {
		coverPath = filepath.Join(outputDir, "cover.jpg")
		coverCmd := exec.Command("curl", "-sL", "-o", coverPath, info.Thumbnail)
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
