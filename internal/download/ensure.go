package download

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	ytDLPReleaseBase = "https://github.com/yt-dlp/yt-dlp/releases/latest/download/"

	// autoInstallCooldown 自动安装失败后的冷却时间。daemon 是长驻进程，队列里每个任务
	// 都会触发一次安装检查；网络/代理故障时用冷却期避免"每任务都打一遍 GitHub"的安装风暴。
	autoInstallCooldown = 10 * time.Minute
)

var (
	autoInstallMu   sync.Mutex
	lastInstallFail time.Time
)

// YTDLPPath 返回当前可用的 yt-dlp 可执行文件绝对路径（只查找，不安装）。
// 查找顺序：PATH → ~/.local/bin/yt-dlp → macOS pip --user 脚本目录。
func YTDLPPath() string {
	return findYTDLP()
}

// ytdlpBinary 返回 yt-dlp 路径；未安装时尝试自动安装（无需 sudo）。
// 安装策略按顺序尝试，任一成功并通过 --version 校验即返回：
//  1. curl 下载官方 standalone 二进制到 ~/.local/bin/yt-dlp（走 YOUTUBE_PROXY）
//  2. python3 -m pip install --user yt-dlp[default,curl-cffi]（含 curl-cffi 模拟；
//     PEP 668 受管环境自动追加 --break-system-packages）
//  3. macOS 且有 brew：brew install yt-dlp
//
// 设置 YTB2BILI_NO_AUTO_INSTALL=1 可关闭自动安装，退回直接报错。
func ytdlpBinary(ctx context.Context) (string, error) {
	if bin := findYTDLP(); bin != "" {
		return bin, nil
	}
	if noAutoInstall() {
		return "", fmt.Errorf("yt-dlp 未安装（已通过 YTB2BILI_NO_AUTO_INSTALL 禁用自动安装）。手动安装: %s", manualInstallHint())
	}
	return ensureYTDLP(ctx)
}

func ensureYTDLP(ctx context.Context) (string, error) {
	autoInstallMu.Lock()
	defer autoInstallMu.Unlock()

	if bin := findYTDLP(); bin != "" { // 双检：并发路径下可能已被装好
		return bin, nil
	}
	if !lastInstallFail.IsZero() && time.Since(lastInstallFail) < autoInstallCooldown {
		return "", fmt.Errorf("yt-dlp 未安装，自动安装约 %s 前已失败（冷却中，%s 后自动重试）。手动安装: %s",
			time.Since(lastInstallFail).Round(time.Minute),
			(autoInstallCooldown - time.Since(lastInstallFail)).Round(time.Minute),
			manualInstallHint())
	}

	log.Printf("🔧 yt-dlp 未安装，尝试自动安装到 ~/.local/bin（无需 sudo）...")
	type strategy struct {
		name string
		run  func(context.Context) error
	}
	strategies := []strategy{
		{"curl 下载官方二进制", installViaCurl},
		{"pip --user 安装", installViaPip},
	}
	if runtime.GOOS == "darwin" {
		strategies = append(strategies, strategy{"brew 安装", installViaBrew})
	}

	var errs []string
	for _, s := range strategies {
		if err := s.run(ctx); err != nil {
			errs = append(errs, s.name+": "+err.Error())
			log.Printf("  ⚠ %s 失败: %v", s.name, err)
			continue
		}
		bin := findYTDLP()
		if bin == "" {
			errs = append(errs, s.name+": 安装命令成功但未找到 yt-dlp 可执行文件")
			continue
		}
		if err := verifyYTDLP(ctx, bin); err != nil {
			// 只清理我们自己装到用户目录的产物，避免误删 brew/pip 管理的文件
			if dir, derr := userBinDir(); derr == nil && filepath.Dir(bin) == dir {
				os.Remove(bin)
			}
			errs = append(errs, fmt.Sprintf("%s: 安装后校验失败: %v", s.name, err))
			continue
		}
		lastInstallFail = time.Time{}
		log.Printf("  ✅ yt-dlp 已自动安装: %s", bin)
		return bin, nil
	}

	lastInstallFail = time.Now()
	return "", fmt.Errorf("yt-dlp 自动安装失败（%s）。手动安装: %s", strings.Join(errs, "; "), manualInstallHint())
}

// findYTDLP 按 PATH → ~/.local/bin → macOS pip --user 目录查找 yt-dlp。
func findYTDLP() string {
	if p, err := exec.LookPath("yt-dlp"); err == nil {
		if abs, aerr := filepath.Abs(p); aerr == nil {
			return abs
		}
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	userBin := filepath.Join(home, ".local", "bin", "yt-dlp")
	if isExecFile(userBin) {
		return userBin
	}
	if runtime.GOOS == "darwin" {
		if globs, _ := filepath.Glob(filepath.Join(home, "Library", "Python", "*", "bin", "yt-dlp")); len(globs) > 0 {
			return globs[0]
		}
	}
	return ""
}

func isExecFile(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0
}

func userBinDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "bin"), nil
}

// releaseAssetFor 返回当前平台对应的 yt-dlp 官方 standalone 发布产物名，
// 无预编译产物（如 windows / linux armv7）返回空字符串。
func releaseAssetFor(goos, goarch string) string {
	switch goos {
	case "darwin":
		return "yt-dlp_macos" // universal2
	case "linux":
		switch goarch {
		case "amd64":
			return "yt-dlp_linux_x86_64"
		case "arm64":
			return "yt-dlp_linux_aarch64"
		}
	}
	return ""
}

func installViaCurl(ctx context.Context) error {
	asset := releaseAssetFor(runtime.GOOS, runtime.GOARCH)
	if asset == "" {
		return fmt.Errorf("无 %s/%s 预编译二进制，跳过", runtime.GOOS, runtime.GOARCH)
	}
	curlPath, err := exec.LookPath("curl")
	if err != nil {
		return fmt.Errorf("curl 不可用")
	}
	binDir, err := userBinDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return fmt.Errorf("创建 %s 失败: %w", binDir, err)
	}
	dest := filepath.Join(binDir, "yt-dlp")

	cctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	args := []string{"-fsSL"}
	if proxy := strings.TrimSpace(os.Getenv("YOUTUBE_PROXY")); proxy != "" {
		args = append(args, "--proxy", proxy)
	}
	args = append(args, "-o", dest, ytDLPReleaseBase+asset)
	if out, err := exec.CommandContext(cctx, curlPath, args...).CombinedOutput(); err != nil {
		os.Remove(dest)
		return fmt.Errorf("%v: %s", err, truncateOut(out))
	}
	if err := os.Chmod(dest, 0o755); err != nil {
		return fmt.Errorf("chmod 失败: %w", err)
	}
	return nil
}

func installViaPip(ctx context.Context) error {
	python, err := exec.LookPath("python3")
	if err != nil {
		if python, err = exec.LookPath("python"); err != nil {
			return fmt.Errorf("python3 不可用")
		}
	}
	cctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	run := func(extra ...string) error {
		args := append([]string{"-m", "pip", "install", "--user", "--upgrade"}, extra...)
		args = append(args, "yt-dlp[default,curl-cffi]")
		cmd := exec.CommandContext(cctx, python, args...)
		cmd.Env = proxyEnv()
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("%v: %s", err, truncateOut(out))
		}
		return nil
	}
	if err := run(); err != nil {
		// PEP 668（Debian 12+/Ubuntu 23+ 受管环境）需要显式 --break-system-packages
		if strings.Contains(err.Error(), "externally-managed-environment") {
			return run("--break-system-packages")
		}
		return err
	}
	return nil
}

func installViaBrew(ctx context.Context) error {
	brew, err := exec.LookPath("brew")
	if err != nil {
		return fmt.Errorf("brew 不可用")
	}
	cctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	if out, err := exec.CommandContext(cctx, brew, "install", "yt-dlp").CombinedOutput(); err != nil {
		return fmt.Errorf("%v: %s", err, truncateOut(out))
	}
	return nil
}

func verifyYTDLP(ctx context.Context, bin string) error {
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cctx, bin, "--version").Output()
	if err != nil {
		return fmt.Errorf("%v: %s", err, truncateOut(out))
	}
	if strings.TrimSpace(string(out)) == "" {
		return fmt.Errorf("版本输出为空")
	}
	return nil
}

// proxyEnv 在配置了 YOUTUBE_PROXY 时把代理传给安装命令（GitHub/PyPI 可能需要走代理）。
func proxyEnv() []string {
	env := os.Environ()
	if proxy := strings.TrimSpace(os.Getenv("YOUTUBE_PROXY")); proxy != "" {
		env = append(env, "HTTPS_PROXY="+proxy, "HTTP_PROXY="+proxy)
	}
	return env
}

func noAutoInstall() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("YTB2BILI_NO_AUTO_INSTALL"))) {
	case "", "0", "false", "no", "off":
		return false
	}
	return true
}

func manualInstallHint() string {
	if runtime.GOOS == "darwin" {
		return "brew install yt-dlp"
	}
	asset := releaseAssetFor(runtime.GOOS, runtime.GOARCH)
	if asset == "" {
		asset = "yt-dlp"
	}
	return fmt.Sprintf("curl -L %s%s -o ~/.local/bin/yt-dlp && chmod a+rx ~/.local/bin/yt-dlp（或 python3 -m pip install --user 'yt-dlp[default,curl-cffi]'）", ytDLPReleaseBase, asset)
}

func truncateOut(out []byte) string {
	s := strings.TrimSpace(string(out))
	if len(s) > 300 {
		s = "..." + s[len(s)-300:] // 保留尾部（错误信息通常在最后）
	}
	return s
}
