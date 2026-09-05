package download

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

// staleRotatingCookies 是 YouTube 的短周期轮换令牌：浏览器活跃时会不断轮换，
// 从文件副本（扩展导出/复制）带到 yt-dlp 时几乎必然已过期，
// 过期后 YouTube 直接拒绝整个会话（报 "The page needs to be reloaded"）。
// 丢弃它们让 yt-dlp 退回长效会话 cookie（__Secure-3PSID/SAPISID 等）或匿名访问。
var staleRotatingCookies = []string{"__Secure-1PSIDTS", "__Secure-3PSIDTS"}

// staleCookieErrorPatterns 命中任一即认为 cookies 会话失效，值得换浏览器 cookies 重试。
var staleCookieErrorPatterns = []string{
	"The page needs to be reloaded", // PSIDTS 轮换令牌过期
	"Sign in to confirm",            // 需要登录验证（bot check）
	"not a bot",                     // 同上（完整文案 "Sign in to confirm you're not a bot"）
	"cookies are no longer valid",   // yt-dlp 明确提示 cookie 已被浏览器轮换
	"Please sign in",                // 会话失效要求登录
}

// sanitizeCookieFile 将 cookies 文件中的轮换令牌（__Secure-*PSIDTS）剔除，
// 写入同目录的 "<name>.sanitized"（非 .txt 后缀，不会被最新文件选择逻辑当作候选），
// 返回净化文件路径。原文件保持不变（扩展/刷新流程继续原地更新它）。
// 剔除失败时返回原路径（尽力而为，不阻塞下载）。
func SanitizeCookieFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return path
	}
	var kept []string
	dropped := 0
	for _, line := range strings.Split(string(data), "\n") {
		if isStaleRotatingCookieLine(line) {
			dropped++
			continue
		}
		kept = append(kept, line)
	}
	if dropped == 0 {
		return path
	}
	out := path + ".sanitized"
	if err := os.WriteFile(out, []byte(strings.Join(kept, "\n")), 0o600); err != nil {
		return path
	}
	return out
}

// isStaleRotatingCookieLine 判断一行 Netscape cookie 是否为轮换令牌。
// 字段序：domain flag path secure expiry name value（name 为第 6 列，下标 5）。
func isStaleRotatingCookieLine(line string) bool {
	fields := strings.Split(line, "\t")
	if len(fields) < 7 {
		return false
	}
	for _, name := range staleRotatingCookies {
		if fields[5] == name {
			return true
		}
	}
	return false
}

// isStaleCookieError 判断 yt-dlp 错误信息是否源于 cookies 会话失效。
func isStaleCookieError(msg string) bool {
	for _, p := range staleCookieErrorPatterns {
		if strings.Contains(msg, p) {
			return true
		}
	}
	return false
}

// swapCookiesToBrowser 把 args 里的 `--cookies <file>` 替换为 `--cookies-from-browser <browser>`，
// 用于 cookie 失效后的重试。无 --cookies 或浏览器 cookies 被禁用/无默认时返回 nil（不可重试）。
// 浏览器来源：YOUTUBE_COOKIES_FROM_BROWSER（off/none 禁用）；未设置时仅 macOS 默认 chrome
// （本机默认 Chrome 有活跃登录态；Linux daemon 无头环境默认关闭，显式设置才启用）。
func swapCookiesToBrowser(args []string) []string {
	hasCookies := false
	for _, a := range args {
		if a == "--cookies" {
			hasCookies = true
			break
		}
	}
	if !hasCookies {
		return nil
	}

	browser := strings.TrimSpace(os.Getenv("YOUTUBE_COOKIES_FROM_BROWSER"))
	if strings.EqualFold(browser, "off") || strings.EqualFold(browser, "none") {
		return nil
	}
	if browser == "" {
		if runtime.GOOS != "darwin" {
			return nil // 无头服务器默认无浏览器登录态，避免无意义重试
		}
		browser = "chrome"
	}

	var out []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--cookies" && i+1 < len(args) {
			i++ // 跳过 --cookies 的值
			continue
		}
		out = append(out, args[i])
	}
	out = append(out, "--cookies-from-browser", browser)
	return out
}

// cookieFallbackReason 生成重试日志前缀。
func cookieFallbackReason(err error) string {
	return fmt.Sprintf("cookies 会话失效（%s）", strings.TrimSpace(err.Error()))
}
