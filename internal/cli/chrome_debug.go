package cli

// // Chrome 远程调试辅助（cookies refresh / server 共用的调试浏览器生命周期）

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/zolagz/ytb2bili-go/internal/cdp"
	"github.com/zolagz/ytb2bili-go/internal/config"
)

// ─── Server ────────────────────────────────────────────────────────────────

// serverDaemonCommand 构建以后台方式启动 HTTP 服务的 exec.Cmd（前台进程跑在 server run）。
func serverDaemonCommand(addr string) *exec.Cmd {
	selfPath, _ := os.Executable()
	args := []string{"server", "run"}
	if addr != "" {
		args = append(args, "--addr", addr)
	}
	cmd := exec.Command(selfPath, args...)
	cmd.Env = os.Environ()
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd
}

// ─── Chrome 调试进程管理 ──────────────────────────────────────────────────
// 供 ytb cookies refresh 连接 localhost:9222 提取 YouTube cookies。

// ─── Chrome 调试进程管理 ──────────────────────────────────────────────────
// 供 ytb cookies refresh 连接 localhost:9222 提取 YouTube cookies。

func chromePidFile(dataDir string) string {
	return filepath.Join(dataDir, "chrome.pid")
}

func chromePortFile(dataDir string) string {
	return filepath.Join(dataDir, "chrome.port")
}

// refreshYouTubeCookiesFromDebugChrome 刷新 YouTube cookies 到当前生效路径：
// 连接调试 Chrome（不在则自动拉起）→ CDP 提取 cookies → 写文件并验证。
// daemon 定期刷新与 ytb cookies refresh 共用此入口。
func refreshYouTubeCookiesFromDebugChrome(cfg *config.Config) error {
	// 确保调试 Chrome 在跑（daemon 无 server 时自拉起；已运行则复用）
	if _, err := startChromeDebug(cfg); err != nil {
		return fmt.Errorf("启动调试 Chrome 失败: %w", err)
	}

	cookiesFile := cfg.EffectiveCookiesPath()
	if err := os.MkdirAll(filepath.Dir(cookiesFile), 0o755); err != nil {
		return err
	}

	port := readChromePort(cfg)
	cm := cdp.NewChromeManager(cdp.WithPort(port))
	ctx, cancel, err := cm.ConnectExisting(port)
	if err != nil {
		return fmt.Errorf("连接 Chrome 失败: %w", err)
	}
	defer cancel()

	if _, err := cdp.RefreshYouTubeCookies(ctx, cookiesFile); err != nil {
		return fmt.Errorf("刷新 cookies 失败: %w", err)
	}
	return nil
}

// readChromePort 读取上次启动记录的 Chrome 调试端口，无记录则用配置起始端口。
func readChromePort(cfg *config.Config) int {
	dataDir := cfg.DataDir
	if data, err := os.ReadFile(chromePortFile(dataDir)); err == nil {
		var port int
		if n, _ := fmt.Sscanf(string(data), "%d", &port); n == 1 && port > 0 {
			return port
		}
	}
	return cfg.EffectiveChromeDebugPort()
}

// startChromeDebug 以远程调试模式启动 Chrome 并记录其 PID 与端口。
// 已在运行则跳过；返回是否本次新启动。
// 说明：直接执行 Chrome 二进制，并用独立 --user-data-dir 启动一个单独的调试实例，
// 与用户日常的 Chrome 互不干扰，且能独立启停。端口用 FindAvailablePort 自动避开占用。
// 支持平台: macOS（/Applications/...）、Linux（google-chrome / chromium 等）。
func startChromeDebug(cfg *config.Config) (started bool, err error) {
	dataDir := cfg.DataDir
	// 已在运行（pid 文件 + 进程存活）则跳过
	if pidData, rerr := os.ReadFile(chromePidFile(dataDir)); rerr == nil {
		var pid int
		fmt.Sscanf(string(pidData), "%d", &pid)
		if proc, perr := os.FindProcess(pid); perr == nil && proc.Signal(syscall.Signal(0)) == nil {
			return false, nil
		}
	}

	chromeBin, err := findChromeBinary()
	if err != nil {
		return false, err
	}
	port := cdp.FindAvailablePort(cfg.EffectiveChromeDebugPort())
	profileDir, _ := filepath.Abs(filepath.Join(dataDir, "chrome-profile"))
	cmd := exec.Command(chromeBin,
		"--remote-debugging-port="+fmt.Sprint(port),
		"--remote-allow-origins=*", // Chrome 111+ 默认拒绖非 localhost Origin 的 CDP WS 连接
		"--user-data-dir="+profileDir,
		"--no-first-run", "--no-default-browser-check")
	if err := cmd.Start(); err != nil {
		return false, fmt.Errorf("启动 Chrome 失败: %w", err)
	}
	go cmd.Wait() // 回收进程，让 Chrome 独立存活
	os.WriteFile(chromePidFile(dataDir), []byte(fmt.Sprintf("%d", cmd.Process.Pid)), 0644)
	os.WriteFile(chromePortFile(dataDir), []byte(fmt.Sprintf("%d", port)), 0644)
	return true, nil
}

// findChromeBinary 查找本机 Chrome/Chromium 可执行文件路径。
// 按平台顺序探测：macOS 固定路径 → Linux 常见命令/路径 → Windows 常见路径。
func findChromeBinary() (string, error) {
	candidates := []string{}
	switch runtime.GOOS {
	case "darwin":
		candidates = []string{"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"}
	case "windows":
		candidates = []string{
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
		}
	default: // linux 及类 unix
		candidates = []string{
			"google-chrome", "google-chrome-stable", "chromium", "chromium-browser",
			"/usr/bin/google-chrome",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
			"/snap/bin/chromium",
			"/opt/google/chrome/chrome",
		}
	}
	for _, c := range candidates {
		if strings.Contains(c, "/") {
			if _, err := os.Stat(c); err == nil {
				return c, nil
			}
		} else if p, err := exec.LookPath(c); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("未找到 Chrome/Chromium，请安装 Chrome 或 Chromium 浏览器")
}

// stopChromeDebug 停止 Chrome 调试进程并清理 pid/port 文件。
func stopChromeDebug(cfg *config.Config) (stopped bool, err error) {
	dataDir := cfg.DataDir
	pidFile := chromePidFile(dataDir)
	pidData, rerr := os.ReadFile(pidFile)
	os.Remove(pidFile)
	os.Remove(chromePortFile(dataDir))
	if rerr != nil {
		return false, nil // 无记录
	}
	var pid int
	fmt.Sscanf(string(pidData), "%d", &pid)
	if pid <= 0 {
		return false, nil
	}
	proc, perr := os.FindProcess(pid)
	if perr != nil {
		return false, nil
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return false, nil // 已退出
	}
	time.Sleep(300 * time.Millisecond)
	return true, nil
}
