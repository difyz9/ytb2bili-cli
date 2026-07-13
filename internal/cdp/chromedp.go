package cdp

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/chromedp/cdproto/network"
)

// ─── 类型定义 ──────────────────────────────────────────────────────────────

// BrowserInfo 浏览器信息
type BrowserInfo struct {
	Path    string
	Name    string
	Version string
}

// ChromeManager Chrome CDP 管理器 - 参考 MediaCrawler 的 CDPBrowserManager
type ChromeManager struct {
	System      string
	BrowserPath string
	DebugPort   int
	UserDataDir string
	Headless    bool
	cmd         *exec.Cmd
	allocCtx    context.Context
	cancel      context.CancelFunc
}

// Option 配置选项
type Option func(*ChromeManager)

// WithPort 设置 CDP 调试端口
func WithPort(port int) Option {
	return func(m *ChromeManager) { m.DebugPort = port }
}

// WithHeadless 设置无头模式
func WithHeadless(headless bool) Option {
	return func(m *ChromeManager) { m.Headless = headless }
}

// WithUserDataDir 设置用户数据目录
func WithUserDataDir(dir string) Option {
	return func(m *ChromeManager) { m.UserDataDir = dir }
}

// ─── 构造函数 ─────────────────────────────────────────────────────────────

// NewChromeManager 创建 Chrome 管理器
func NewChromeManager(opts ...Option) *ChromeManager {
	m := &ChromeManager{
		System:    runtime.GOOS,
		DebugPort: 9222,
		Headless:  false,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// ─── 浏览器检测 ───────────────────────────────────────────────────────────

// DetectBrowsers 检测系统中可用的浏览器 - 参考 MediaCrawler 的 detect_browser_paths()
func (m *ChromeManager) DetectBrowsers() []BrowserInfo {
	var browsers []BrowserInfo
	for _, c := range m.possiblePaths() {
		if _, err := os.Stat(c.Path); err == nil {
			browsers = append(browsers, BrowserInfo{
				Path:    c.Path,
				Name:    c.Name,
				Version: getVersion(c.Path),
			})
		}
	}
	return browsers
}

type candidate struct {
	Path string
	Name string
}

func (m *ChromeManager) possiblePaths() []candidate {
	switch m.System {
	case "windows":
		local := os.Getenv("LOCALAPPDATA")
		pf := os.Getenv("PROGRAMFILES")
		pfx86 := os.Getenv("PROGRAMFILES(X86)")
		return []candidate{
			{filepath.Join(pf, "Google/Chrome/Application/chrome.exe"), "Google Chrome"},
			{filepath.Join(pfx86, "Google/Chrome/Application/chrome.exe"), "Google Chrome"},
			{filepath.Join(local, "Google/Chrome/Application/chrome.exe"), "Google Chrome"},
			{filepath.Join(pf, "Microsoft/Edge/Application/msedge.exe"), "Microsoft Edge"},
			{filepath.Join(pfx86, "Microsoft/Edge/Application/msedge.exe"), "Microsoft Edge"},
		}
	case "darwin":
		return []candidate{
			{"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", "Google Chrome"},
			{"/Applications/Google Chrome Beta.app/Contents/MacOS/Google Chrome Beta", "Google Chrome Beta"},
			{"/Applications/Google Chrome Canary.app/Contents/MacOS/Google Chrome Canary", "Google Chrome Canary"},
			{"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge", "Microsoft Edge"},
		}
	default: // Linux
		return []candidate{
			{"/usr/bin/google-chrome", "Google Chrome"},
			{"/usr/bin/google-chrome-stable", "Google Chrome"},
			{"/usr/bin/google-chrome-beta", "Google Chrome Beta"},
			{"/usr/bin/google-chrome-unstable", "Google Chrome Dev"},
			{"/usr/bin/chromium-browser", "Chromium"},
			{"/usr/bin/chromium", "Chromium"},
			{"/snap/bin/chromium", "Chromium"},
			{"/usr/bin/microsoft-edge", "Microsoft Edge"},
			{"/usr/bin/microsoft-edge-stable", "Microsoft Edge"},
			{"/usr/bin/microsoft-edge-stable", "Microsoft Edge Stable"},
			{"/usr/bin/microsoft-edge-beta", "Microsoft Edge Beta"},
			{"/usr/bin/microsoft-edge-dev", "Microsoft Edge Dev"},
		}
	}
}

func getVersion(path string) string {
	out, err := exec.Command(path, "--version").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

// ─── 端口工具 ─────────────────────────────────────────────────────────────

// FindAvailablePort 查找可用端口 - 参考 MediaCrawler 的 find_available_port()
func FindAvailablePort(startPort int) int {
	port := startPort
	for port < startPort+100 {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			ln.Close()
			return port
		}
		port++
	}
	return startPort // 回退到默认端口
}

// TestPort 测试端口是否可连接 - 参考 MediaCrawler 的 _test_cdp_connection()
func TestPort(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// ─── Chrome 启动 ──────────────────────────────────────────────────────────

// Launch 启动 Chrome 并开启 CDP - 参考 MediaCrawler 的 launch_browser()
func (m *ChromeManager) Launch() error {
	// 1. 检测浏览器路径
	chromePath := m.findChrome()
	if chromePath == "" {
		return fmt.Errorf("未找到 Chrome/Chromium 浏览器")
	}
	m.BrowserPath = chromePath

	// 2. 查找可用端口
	m.DebugPort = FindAvailablePort(m.DebugPort)

	// 3. 准备用户数据目录
	if m.UserDataDir == "" {
		m.UserDataDir = filepath.Join(os.TempDir(), "ytb2bili-cdp")
	}
	os.MkdirAll(m.UserDataDir, 0755)

	// 4. 构建启动参数 - 参考 MediaCrawler 的反检测参数
	args := []string{
		chromePath,
		fmt.Sprintf("--remote-debugging-port=%d", m.DebugPort),
		"--remote-debugging-address=0.0.0.0",
		"--no-first-run",
		"--no-default-browser-check",
		"--no-sandbox",
		"--disable-blink-features=AutomationControlled",
		"--exclude-switches=enable-automation",
		"--disable-infobars",
		"--disable-dev-shm-usage",
		"--disable-background-timer-throttling",
		"--disable-backgrounding-occluded-windows",
		"--disable-renderer-backgrounding",
		"--disable-features=TranslateUI",
		"--disable-ipc-flooding-protection",
		"--disable-hang-monitor",
		"--disable-prompt-on-repost",
		"--disable-sync",
		fmt.Sprintf("--user-data-dir=%s", m.UserDataDir),
	}

	if m.Headless {
		args = append(args, "--headless=new", "--disable-gpu")
	}

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	m.cmd = cmd

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 Chrome 失败: %w", err)
	}

	// 5. 等待浏览器就绪 - 参考 MediaCrawler 的 wait_for_browser_ready()
	fmt.Fprintf(os.Stderr, "⏳ 等待 Chrome 启动 (端口 %d)...\n", m.DebugPort)
	start := time.Now()
	for time.Since(start) < 30*time.Second {
		if TestPort(m.DebugPort) {
			fmt.Fprintf(os.Stderr, "✅ Chrome 已就绪 (端口 %d)\n", m.DebugPort)
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("Chrome 启动超时（端口 %d 无响应）", m.DebugPort)
}

func (m *ChromeManager) findChrome() string {
	for _, br := range m.DetectBrowsers() {
		if strings.Contains(br.Name, "Chrome") || strings.Contains(br.Name, "Chromium") || strings.Contains(br.Name, "Edge") {
			return br.Path
		}
	}
	return ""
}

// ─── chromedp 上下文 ─────────────────────────────────────────────────────

// Connect 连接到 Chorme CDP - 类似 MediaCrawler 的 _connect_via_cdp()
func (m *ChromeManager) Connect() (context.Context, context.CancelFunc, error) {
	parentCtx := context.Background()

	// 配置 chromedp
	opts := []chromedp.ContextOption{
		chromedp.WithLogf(func(format string, args ...interface{}) {
			fmt.Fprintf(os.Stderr, format+"\n", args...)
		}),
	}

	if TestPort(m.DebugPort) {
		// 连接到已有 Chrome - 参考 MediaCrawler 的 _connect_existing_browser()
		fmt.Fprintf(os.Stderr, "🔗 连接到已有 Chrome (端口 %d)\n", m.DebugPort)
		allocCtx, cancel := chromedp.NewRemoteAllocator(parentCtx, fmt.Sprintf("http://127.0.0.1:%d", m.DebugPort))
		m.allocCtx = allocCtx
		m.cancel = cancel
		ctx, _ := chromedp.NewContext(allocCtx, opts...)
		return ctx, cancel, nil
	}

	// 启动新 Chrome - 参考 MediaCrawler 的启动流程
	fmt.Fprintf(os.Stderr, "🚀 启动 Chrome (端口 %d)\n", m.DebugPort)

	// 先启动 Chrome
	if err := m.Launch(); err != nil {
		return nil, nil, err
	}

	allocCtx, cancel := chromedp.NewRemoteAllocator(parentCtx, fmt.Sprintf("http://127.0.0.1:%d", m.DebugPort))
	m.allocCtx = allocCtx
	m.cancel = cancel
	ctx, _ := chromedp.NewContext(allocCtx, opts...)

	return ctx, cancel, nil
}

// ConnectExisting 连接到已有 Chrome（不启动新进程）
func (m *ChromeManager) ConnectExisting(port int) (context.Context, context.CancelFunc, error) {
	if !TestPort(port) {
		return nil, nil, fmt.Errorf("端口 %d 没有 Chrome 在监听", port)
	}

	m.DebugPort = port
	parentCtx := context.Background()
	allocCtx, cancel := chromedp.NewRemoteAllocator(parentCtx, fmt.Sprintf("http://127.0.0.1:%d", port))
	m.allocCtx = allocCtx
	m.cancel = cancel

	opts := []chromedp.ContextOption{}
	ctx, _ := chromedp.NewContext(allocCtx, opts...)

	// 测试连接
	ctx, cancelTimeout := context.WithTimeout(ctx, 10*time.Second)
	defer cancelTimeout()

	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return nil
	})); err != nil {
		cancel()
		return nil, nil, fmt.Errorf("连接 Chrome 失败: %w", err)
	}

	return ctx, cancel, nil
}

// ─── 浏览器操作 ───────────────────────────────────────────────────────────

// OpenURL 在浏览器中打开 URL
func OpenURL(ctx context.Context, url string) error {
	// 先创建新标签页
	ctx, cancel := chromedp.NewContext(ctx)
	defer cancel()

	return chromedp.Run(ctx,
		chromedp.Navigate(url),
	)
}

// OpenURLAndWait 打开 URL 并等待页面加载
func OpenURLAndWait(ctx context.Context, url string) error {
	ctx, cancel := chromedp.NewContext(ctx)
	defer cancel()

	return chromedp.Run(ctx,
		chromedp.Navigate(url),
		chromedp.WaitReady("body"),
	)
}

// Screenshot 截取页面截图 - 参考 MediaCrawler 的截图功能
func Screenshot(ctx context.Context, selector string) ([]byte, error) {
	var buf []byte
	ctx, cancel := chromedp.NewContext(ctx)
	defer cancel()

	if selector == "" {
		// 全页截图
		err := chromedp.Run(ctx,
			chromedp.FullScreenshot(&buf, 90),
		)
		return buf, err
	}

	// 元素截图
	err := chromedp.Run(ctx,
		chromedp.Screenshot(selector, &buf, chromedp.NodeVisible),
	)
	return buf, err
}

// EvaluateJS 执行 JavaScript - 类似 MediaCrawler 的 JS 表达式获取
func EvaluateJS(ctx context.Context, expression string) (string, error) {
	var result string
	ctx, cancel := chromedp.NewContext(ctx)
	defer cancel()

	err := chromedp.Run(ctx,
		chromedp.Evaluate(expression, &result),
	)
	return result, err
}

// ─── 清理 ──────────────────────────────────────────────────────────────────

// Cleanup 清理资源 - 参考 MediaCrawler 的 cleanup()
func (m *ChromeManager) Cleanup() {
	if m.cancel != nil {
		m.cancel()
	}
	if m.cmd != nil && m.cmd.Process != nil {
		fmt.Fprintf(os.Stderr, "🧹 关闭 Chrome 进程...\n")
		pgid, err := syscall.Getpgid(m.cmd.Process.Pid)
		if err == nil {
			syscall.Kill(-pgid, syscall.SIGTERM)
			go func() {
				time.Sleep(3 * time.Second)
				syscall.Kill(-pgid, syscall.SIGKILL)
			}()
		} else {
			m.cmd.Process.Kill()
		}
	}
}

// RegisterCleanup 注册信号清理处理器 - 参考 MediaCrawler 的 _register_cleanup_handlers()
func RegisterCleanup(m *ChromeManager) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-c
		m.Cleanup()
		os.Exit(0)
	}()
}

// ─── 系统命令辅助 ─────────────────────────────────────────────────────────

// OpenURLSystem 使用系统命令打开 URL（不依赖 CDP）
func OpenURLSystem(url string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("cmd", "/c", "start", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	default:
		if _, err := exec.LookPath("xdg-open"); err == nil {
			return exec.Command("xdg-open", url).Start()
		}
		if _, err := exec.LookPath("gio"); err == nil {
			return exec.Command("gio", "open", url).Start()
		}
		return fmt.Errorf("未找到 xdg-open 或 gio")
	}
}

// ─── Cookie 刷新 ───────────────────────────────────────────────────────────

// RefreshYouTubeCookies 从 Chrome 获取最新的 YouTube cookies 并保存到文件
func RefreshYouTubeCookies(ctx context.Context, outputPath string) (int, error) {
	ctx, cancel := chromedp.NewContext(ctx)
	defer cancel()

	// 打开 YouTube 首页以触发 cookie 同步
	if err := chromedp.Run(ctx,
		chromedp.Navigate("https://www.youtube.com"),
		chromedp.WaitReady("body"),
	); err != nil {
		return 0, fmt.Errorf("打开 YouTube 失败（可能需要登录）: %w", err)
	}

	// 检查是否已登录（看是否有头像按钮）
	var loggedIn bool
	if err := chromedp.Run(ctx,
		chromedp.Evaluate(`document.querySelector('#avatar-btn') !== null`, &loggedIn),
	); err != nil {
		loggedIn = false
	}
	if !loggedIn {
		fmt.Fprintf(os.Stderr, "⚠️  未检测到 YouTube 登录状态，将使用访客 cookies\n")
	}

	// 获取 cookies
	var cookies []map[string]interface{}
	if err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			cookies, err = networkGetAllCookies(ctx)
			return err
		}),
	); err != nil {
		return 0, fmt.Errorf("获取 cookies 失败: %w", err)
	}

	// 只保留 YouTube 相关 cookies
	var youtubeCookies []map[string]interface{}
	for _, c := range cookies {
		domain, _ := c["domain"].(string)
		if strings.Contains(domain, "youtube.com") || strings.Contains(domain, "ytimg.com") || domain == ".google.com" {
			youtubeCookies = append(youtubeCookies, c)
		}
	}

	if len(youtubeCookies) == 0 {
		return 0, fmt.Errorf("未找到任何 YouTube cookies，请确认已在 Chrome 中登录 YouTube")
	}

	// 写入 Netscape 格式文件
	f, err := os.Create(outputPath)
	if err != nil {
		return 0, fmt.Errorf("创建 cookies 文件失败: %w", err)
	}
	defer f.Close()

	fmt.Fprintf(f, "# Netscape HTTP Cookie File\n")
	fmt.Fprintf(f, "# Generated by ytb2bili-go on %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(f, "# This file is auto-generated. Do not edit.\n\n")

	written := 0
	for _, c := range youtubeCookies {
		domain, _ := c["domain"].(string)
		path, _ := c["path"].(string)
		name, _ := c["name"].(string)
		value, _ := c["value"].(string)
		secure, _ := c["secure"].(bool)
		httpOnly, _ := c["httpOnly"].(bool)
		expires, _ := c["expires"].(float64)

		if domain == "" || name == "" {
			continue
		}

		// 处理过期时间
		expiry := int64(expires)
		if expiry == 0 {
			expiry = 2147483647 // 永不过期
		}

		// 域名标志：以 . 开头的用 TRUE
		domainFlag := "FALSE"
		if strings.HasPrefix(domain, ".") {
			domainFlag = "TRUE"
		}

		// secure 标志
		secureFlag := "FALSE"
		if secure || httpOnly {
			secureFlag = "TRUE"
		}

		// 清理 value 中的特殊字符
		value = strings.ReplaceAll(value, "\n", "")
		value = strings.ReplaceAll(value, "\r", "")

		fmt.Fprintf(f, "%s\t%s\t%s\t%s\t%d\t%s\t%s\n",
			domain, domainFlag, path, secureFlag, expiry, name, value)
		written++
	}

	return written, nil
}

// networkGetAllCookies 通过 Chrome DevTools Protocol 获取所有 cookies
func networkGetAllCookies(ctx context.Context) ([]map[string]interface{}, error) {
	var result []map[string]interface{}
	err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			cookies, err := network.GetCookies().Do(ctx)
			if err != nil {
				return fmt.Errorf("network.GetCookies failed: %w", err)
			}
			for _, c := range cookies {
				result = append(result, map[string]interface{}{
					"domain":   c.Domain,
					"path":     c.Path,
					"name":     c.Name,
					"value":    c.Value,
					"secure":   c.Secure,
					"httpOnly": c.HTTPOnly,
					"expires":  c.Expires,
				})
			}
			return nil
		}),
	)
	return result, err
}

// TestYouTubeCookies 测试 cookies 是否有效（尝试获取一个视频的信息）
func TestYouTubeCookies(cookiesPath string) error {
	cmd := exec.Command("yt-dlp",
		"--impersonate", "chrome",
		"--cookies", cookiesPath,
		"--dump-json", "--no-download",
		"--remote-components", "ejs:github",
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ",
	)
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("cookies 验证失败（可能需要重新登录 YouTube）: %w", err)
	}
	// 检查是否包含 title
	if !strings.Contains(string(out), `"title"`) {
		return fmt.Errorf("cookies 验证失败：返回数据异常")
	}
	return nil
}

// ShowImageSystem 使用系统图片查看器显示图片
func ShowImageSystem(path string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("cmd", "/c", "start", path).Start()
	case "darwin":
		return exec.Command("open", path).Start()
	default:
		viewers := []string{"xdg-open", "eog", "gwenview", "display", "feh"}
		for _, v := range viewers {
			if p, err := exec.LookPath(v); err == nil {
				return exec.Command(p, path).Start()
			}
		}
		return fmt.Errorf("未找到图片查看器")
	}
}
