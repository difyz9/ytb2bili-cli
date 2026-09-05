package download

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCookieArgsMacDefaultsToChrome(t *testing.T) {
	got := cookieArgs("", "", "darwin")
	want := []string{"--cookies-from-browser", "chrome"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("cookieArgs() = %v, want %v", got, want)
	}
}

func TestCookieArgsCanDisableBrowserCookies(t *testing.T) {
	if got := cookieArgs("", "off", "darwin"); got != nil {
		t.Fatalf("cookieArgs() = %v, want nil", got)
	}
}

func TestCookieArgsSupportsConfiguredBrowserProfile(t *testing.T) {
	got := cookieArgs("", "chrome:Default", "linux")
	want := []string{"--cookies-from-browser", "chrome:Default"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("cookieArgs() = %v, want %v", got, want)
	}
}

func TestPrepareYTDLPAddsValidCookiesFile(t *testing.T) {
	if _, err := exec.LookPath("yt-dlp"); err != nil {
		t.Skip("yt-dlp not installed")
	}
	dir := t.TempDir()
	cookies := filepath.Join(dir, "cookies.txt")
	// Netscape cookie line with non-zero expiry (field[4] != "0")
	os.WriteFile(cookies, []byte("www.youtube.com\tTRUE\t/\tTRUE\t1728000000\tSID\tvalue\n"), 0644)

	_, baseArgs, _, err := prepareYTDLP(context.Background(), cookies)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for i, a := range baseArgs {
		if a == "--cookies" && i+1 < len(baseArgs) && baseArgs[i+1] == cookies {
			found = true
		}
	}
	if !found {
		t.Fatalf("prepareYTDLP missing --cookies %s: %v", cookies, baseArgs)
	}
}

func TestReleaseAssetFor(t *testing.T) {
	cases := []struct {
		goos, goarch, want string
	}{
		{"darwin", "arm64", "yt-dlp_macos"},
		{"darwin", "amd64", "yt-dlp_macos"},
		{"linux", "amd64", "yt-dlp_linux_x86_64"},
		{"linux", "arm64", "yt-dlp_linux_aarch64"},
		{"linux", "arm", ""},
		{"windows", "amd64", ""},
	}
	for _, c := range cases {
		if got := releaseAssetFor(c.goos, c.goarch); got != c.want {
			t.Errorf("releaseAssetFor(%s, %s) = %q, want %q", c.goos, c.goarch, got, c.want)
		}
	}
}

func TestFindYTDLPFindsUserLocalBin(t *testing.T) {
	home := t.TempDir()
	binDir := filepath.Join(home, ".local", "bin")
	os.MkdirAll(binDir, 0o755)
	fake := filepath.Join(binDir, "yt-dlp")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	emptyPath := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", emptyPath) // 模拟 systemd 服务 PATH 里没有 yt-dlp

	if got := findYTDLP(); got != fake {
		t.Fatalf("findYTDLP() = %q, want %q", got, fake)
	}
}

func TestYTDLPBinaryCooldownAfterFailedInstall(t *testing.T) {
	emptyPath := t.TempDir()
	t.Setenv("PATH", emptyPath)
	t.Setenv("HOME", t.TempDir()) // 无 yt-dlp

	lastInstallFail = time.Now().Add(-time.Minute) // 1 分钟前失败 → 仍在冷却期
	t.Cleanup(func() { lastInstallFail = time.Time{} })

	_, err := ytdlpBinary(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "冷却") {
		t.Fatalf("expected cooldown error, got: %v", err)
	}
}

func TestYTDLPBinaryRespectsNoAutoInstall(t *testing.T) {
	emptyPath := t.TempDir()
	t.Setenv("PATH", emptyPath)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("YTB2BILI_NO_AUTO_INSTALL", "1")

	_, err := ytdlpBinary(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "手动安装") {
		t.Fatalf("expected manual install hint, got: %v", err)
	}
}

func TestTruncateOut(t *testing.T) {
	if got := truncateOut([]byte("  short  ")); got != "short" {
		t.Fatalf("truncateOut = %q, want %q", got, "short")
	}
	long := strings.Repeat("a", 500)
	got := truncateOut([]byte(long))
	if len(got) != 303 || !strings.HasPrefix(got, "...") {
		t.Fatalf("truncateOut len=%d, want 303 with ... prefix", len(got))
	}
}

// TestManualAutoInstallE2E 真实网络端到端验证：模拟 PATH 无 yt-dlp 的环境（如 systemd 服务），
// 触发完整自动安装流程。默认跳过，需 YTB_TEST_AUTO_INSTALL=1 显式开启。
func TestManualAutoInstallE2E(t *testing.T) {
	if os.Getenv("YTB_TEST_AUTO_INSTALL") != "1" {
		t.Skip("set YTB_TEST_AUTO_INSTALL=1 to run real network install")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	// PATH 仅含系统目录（curl 可用、无 yt-dlp），模拟服务环境
	t.Setenv("PATH", "/usr/bin:/bin")
	lastInstallFail = time.Time{}
	t.Cleanup(func() { lastInstallFail = time.Time{} })

	bin, err := ytdlpBinary(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".local", "bin", "yt-dlp")
	if bin != want {
		t.Fatalf("installed bin = %q, want %q", bin, want)
	}
	out, err := exec.Command(bin, "--version").Output()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("auto-installed yt-dlp %s at %s", strings.TrimSpace(string(out)), bin)
}
