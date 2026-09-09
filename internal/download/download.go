package download

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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

	ytdlpBin, baseArgs, env, err := prepareYTDLP(ctx, cookiesPath...)
	if err != nil {
		return nil, err
	}

	// Get video info first (轻量元数据，不含视频下载)
	info, err := InfoContext(ctx, url, cookiesPath...)
	if err != nil {
		return nil, err
	}

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

	cmd := exec.CommandContext(ctx, ytdlpBin, args...)
	cmd.Stdout = log.Writer()
	var downloadStderr bytes.Buffer
	cmd.Stderr = io.MultiWriter(log.Writer(), &downloadStderr)
	cmd.Env = env
	if err := cmd.Run(); err != nil {
		errMsg := extractYTDLPError(err, strings.TrimSpace(downloadStderr.String()))
		// cookies 会话失效（PSIDTS 轮换等）：换浏览器 cookies 重试一次
		retried := false
		if isStaleCookieError(errMsg) {
			if retryArgs := swapCookiesToBrowser(args); retryArgs != nil {
				log.Printf("  ⚠ cookies 会话失效（%s），改用浏览器 cookies 重试...\n", errMsg)
				retry := exec.CommandContext(ctx, ytdlpBin, retryArgs...)
				retry.Stdout = log.Writer()
				var retryStderr bytes.Buffer
				retry.Stderr = io.MultiWriter(log.Writer(), &retryStderr)
				retry.Env = env
				if rerr := retry.Run(); rerr != nil {
					retryMsg := extractYTDLPError(rerr, strings.TrimSpace(retryStderr.String()))
					return nil, fmt.Errorf("下载失败: %s（已尝试浏览器 cookies 重试: %s）", errMsg, retryMsg)
				}
				retried = true
			} else {
				log.Printf("  ⚠ cookies 会话失效（%s），且无法回退浏览器 cookies（非 macOS 或已禁用）\n", errMsg)
			}
		}
		if !retried {
			return nil, fmt.Errorf("下载失败: %s", errMsg)
		}
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
		curlArgs := []string{"-sL"}
		if proxy := strings.TrimSpace(os.Getenv("YOUTUBE_PROXY")); proxy != "" {
			curlArgs = append(curlArgs, "--proxy", proxy)
		}
		curlArgs = append(curlArgs, "-o", coverPath, info.Thumbnail)
		coverCmd := exec.CommandContext(ctx, "curl", curlArgs...)
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
		Info:         *info,
	}, nil
}

// prepareYTDLP 解析 yt-dlp 可执行文件（缺失时自动安装）并构建基础参数（deno PATH + cookies 解析）。
// 返回 bin（yt-dlp 绝对路径，后续 exec 直接用它，避免依赖进程 PATH）、
// baseArgs（给 yt-dlp 的通用参数）和 env（含 deno PATH）。
func prepareYTDLP(ctx context.Context, cookiesPath ...string) (bin string, baseArgs, env []string, err error) {
	bin, err = ytdlpBinary(ctx)
	if err != nil {
		return "", nil, nil, err
	}

	// Ensure deno is in PATH for yt-dlp YouTube JS challenges
	env = os.Environ()
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
		// ~/.local/bin 也加入 PATH（自动安装的 yt-dlp/ffmpeg 等放在这里）
		if udir, uerr := userBinDir(); uerr == nil {
			inPath := false
			for _, p := range strings.Split(os.Getenv("PATH"), ":") {
				if p == udir {
					inPath = true
					break
				}
			}
			if !inPath {
				env = append(env, "PATH="+udir+":"+os.Getenv("PATH"))
			}
		}
	}

	// Resolve cookies: prefer explicit cookie file, fallback to browser session
	cookiesFile := ""
	if len(cookiesPath) > 0 && cookiesPath[0] != "" {
		cookiesFile = cookiesPath[0]
	}
	// If the provided cookie file is empty or has no valid entries, try global YOUTUBE_COOKIES env var.
	if cookiesFile == "" || !HasValidCookies(cookiesFile) {
		if global := os.Getenv("YOUTUBE_COOKIES"); global != "" {
			if _, err := os.Stat(global); err == nil {
				cookiesFile = global
			}
		}
	}
	// 剔除短周期轮换令牌（__Secure-*PSIDTS）：文件副本几乎必然已过期，
	// 带着 stale 令牌会被 YouTube 直接拒绝（"The page needs to be reloaded"）。
	// 原文件不动（扩展/刷新流程继续原地更新），净化副本用 .sanitized 后缀。
	if cookiesFile != "" {
		if s := SanitizeCookieFile(cookiesFile); s != cookiesFile {
			log.Printf("  🍪 剔除已轮换的 PSIDTS 令牌: %s → %s\n", filepath.Base(cookiesFile), filepath.Base(s))
			cookiesFile = s
		}
	}

	// On macOS, prefer --cookies-from-browser chrome over a stale cookies file,
	// since Chrome keeps an active YouTube login session.  Set
	// YOUTUBE_COOKIES_FROM_BROWSER to override the browser/profile syntax accepted
	// by yt-dlp, or to "off" to disable browser-cookie discovery entirely.
	baseArgs = cookieArgs(cookiesFile, os.Getenv("YOUTUBE_COOKIES_FROM_BROWSER"), runtime.GOOS)

	// YouTube 下载专用代理（可选）：config.yaml `youtube_proxy` 或环境变量 YOUTUBE_PROXY，
	// 格式如 socks5://user:pass@host:port / http://user:pass@host:port。
	// 仅传给 yt-dlp（下载 + 取元数据），B站/翻译等国内流量不受影响。
	if proxy := strings.TrimSpace(os.Getenv("YOUTUBE_PROXY")); proxy != "" {
		baseArgs = append(baseArgs, "--proxy", proxy)
	}
	return bin, baseArgs, env, nil
}

// InfoContext 仅获取视频元数据（不下载视频），返回 VideoInfo。
func InfoContext(ctx context.Context, url string, cookiesPath ...string) (*VideoInfo, error) {
	ytdlpBin, baseArgs, env, err := prepareYTDLP(ctx, cookiesPath...)
	if err != nil {
		return nil, err
	}
	infoArgs := append(baseArgs, "--dump-json", "--no-download", "--remote-components", "ejs:github", url)
	infoCmd := exec.CommandContext(ctx, ytdlpBin, infoArgs...)
	infoCmd.Env = env
	var infoStderr bytes.Buffer
	infoCmd.Stderr = &infoStderr
	infoOut, err := infoCmd.Output()
	if err != nil {
		stderrStr := strings.TrimSpace(infoStderr.String())
		errMsg := extractYTDLPError(err, stderrStr)

		// cookies 会话失效（bot check/轮换）时换浏览器 cookies 重试一次
		if isStaleCookieError(errMsg) {
			if retryArgs := swapCookiesToBrowser(infoArgs); retryArgs != nil {
				log.Printf("  ⚠ cookies 会话失效（%s），改用浏览器 cookies 重试...\n", errMsg)
				retryCmd := exec.CommandContext(ctx, ytdlpBin, retryArgs...)
				retryCmd.Env = env
				var retryStderr bytes.Buffer
				retryCmd.Stderr = &retryStderr
				infoOut, err = retryCmd.Output()
				if err != nil {
					errMsg = extractYTDLPError(err, strings.TrimSpace(retryStderr.String()))
				}
			} else {
				log.Printf("  ⚠ cookies 会话失效（%s），且无法回退浏览器 cookies（非 macOS 或已禁用）\n", errMsg)
			}
		}
		if err != nil {
			return nil, fmt.Errorf("获取视频信息失败: %s", errMsg)
		}
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
	return &info, nil
}

// extractYTDLPError 从 yt-dlp 失败输出中提取最相关的错误信息（首个 ERROR: 行，
// 无则取 stderr 末行，再无则用退出错误本身）。
func extractYTDLPError(err error, stderrStr string) string {
	for _, line := range strings.Split(stderrStr, "\n") {
		if strings.HasPrefix(line, "ERROR:") {
			return strings.TrimSpace(line)
		}
	}
	if stderrStr != "" {
		lines := strings.Split(stderrStr, "\n")
		return strings.TrimSpace(lines[len(lines)-1])
	}
	if err != nil {
		return err.Error()
	}
	return "yt-dlp failed"
}

func cookieArgs(cookiesFile, browser, goos string) []string {
	if cookiesFile != "" && HasValidCookies(cookiesFile) {
		// 防御：YouTube 需要完整登录态（含 SID/SSID）。缺 SID 的半登录 cookie（meta 扩展导出常见）
		// 会导致 yt-dlp 在媒体阶段被 403 / bot 验证拦截，且日志里看不出是 cookie 问题。
		// 显式配置的文件缺 SID 时大声告警，避免静默失败。
		if !hasAuthSession(cookiesFile) {
			log.Printf("⚠ WARNING: cookies %s 缺少 SID/SSID（非完整登录态），YouTube 下载大概率 403/bot 拦截。\n   请从已登录 YouTube 的浏览器重新导出完整 cookie（应包含 SID、__Secure-3PSID、SSID、APISID 等 ≥20 行）", cookiesFile)
			if args := browserCookieArgs(browser, goos); args != nil {
				log.Printf("  🍪 改用浏览器 cookies: %s", args[1])
				return args
			}
		}
		return []string{"--cookies", cookiesFile}
	}
	return browserCookieArgs(browser, goos)
}

func browserCookieArgs(browser, goos string) []string {
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

// hasAuthSession 检查 cookie 文件是否含 YouTube 登录态核心字段（SID 或 __Secure-3PSID）。
// 仅凭 3PSID 仍可能被限（实测 meta 导出有 3PSID 无 SID → 媒体 403），完整会话应含 SID。
func hasAuthSession(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) >= 7 && (fields[5] == "SID" || fields[5] == "SSID") {
			return true
		}
	}
	return false
}

// HasValidCookies 检查 Netscape cookies 文件是否含非零过期时间戳的有效条目。
// （导出供 config 层选择最新有效 cookies 文件使用。）
func HasValidCookies(path string) bool {
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
