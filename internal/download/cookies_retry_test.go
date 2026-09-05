package download

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleCookies = `# Netscape HTTP Cookie File
.youtube.com	TRUE	/	TRUE	1728000000	__Secure-3PSID	value3psid
.youtube.com	TRUE	/	TRUE	1728000000	__Secure-3PSIDTS	stale-rotating
.youtube.com	TRUE	/	TRUE	1728000000	__Secure-1PSIDTS	stale-rotating
.youtube.com	TRUE	/	TRUE	1728000000	SAPISID	value-sapisid
.youtube.com	TRUE	/	TRUE	1728000000	SSID	value-ssid
`

func TestSanitizeCookieFileDropsRotatingTokens(t *testing.T) {
	dir := t.TempDir()
	orig := filepath.Join(dir, "cookies.txt")
	if err := os.WriteFile(orig, []byte(sampleCookies), 0o600); err != nil {
		t.Fatal(err)
	}

	got := SanitizeCookieFile(orig)
	if got == orig {
		t.Fatal("expected sanitized copy path, got original")
	}
	data, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if strings.Contains(content, "PSIDTS") {
		t.Errorf("sanitized file still contains PSIDTS:\n%s", content)
	}
	for _, keep := range []string{"__Secure-3PSID", "SAPISID", "SSID"} {
		if !strings.Contains(content, keep) {
			t.Errorf("sanitized file lost %s", keep)
		}
	}
	// 原文件保持不变
	origData, _ := os.ReadFile(orig)
	if !strings.Contains(string(origData), "__Secure-3PSIDTS") {
		t.Error("original file was modified")
	}
}

func TestSanitizeCookieFileNoTokensReturnsOriginal(t *testing.T) {
	dir := t.TempDir()
	orig := filepath.Join(dir, "cookies.txt")
	clean := strings.ReplaceAll(sampleCookies, "__Secure-3PSIDTS\tstale-rotating\n", "")
	clean = strings.ReplaceAll(clean, "__Secure-1PSIDTS\tstale-rotating\n", "")
	os.WriteFile(orig, []byte(clean), 0o600)

	if got := SanitizeCookieFile(orig); got != orig {
		t.Fatalf("expected original path, got %q", got)
	}
}

func TestSanitizeCookieFileMissingReturnsOriginal(t *testing.T) {
	if got := SanitizeCookieFile("/nonexistent/cookies.txt"); got != "/nonexistent/cookies.txt" {
		t.Fatalf("expected original path, got %q", got)
	}
}

func TestIsStaleCookieErrorPatterns(t *testing.T) {
	cases := []struct {
		msg  string
		want bool
	}{
		{"ERROR: [youtube] xx: The page needs to be reloaded.", true},
		{"ERROR: [youtube] xx: Sign in to confirm you’re not a bot. Use --cookies...", true},
		{"WARNING: The provided YouTube account cookies are no longer valid. They have likely been rotated", true},
		{"ERROR: [youtube] xx: Requested format is not available", false},
		{"", false},
	}
	for _, c := range cases {
		if got := isStaleCookieError(c.msg); got != c.want {
			t.Errorf("isStaleCookieError(%q) = %v, want %v", c.msg, got, c.want)
		}
	}
}

func TestSwapCookiesToBrowser(t *testing.T) {
	t.Setenv("YOUTUBE_COOKIES_FROM_BROWSER", "")
	base := []string{"--cookies", "/tmp/c.txt", "--dump-json"}

	// darwin（测试运行平台为 macOS）：默认 chrome
	got := swapCookiesToBrowser(base)
	want := []string{"--dump-json", "--cookies-from-browser", "chrome"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("swapCookiesToBrowser = %v, want %v", got, want)
	}

	// 显式禁用 → nil
	t.Setenv("YOUTUBE_COOKIES_FROM_BROWSER", "off")
	if got := swapCookiesToBrowser(base); got != nil {
		t.Fatalf("disabled browser cookies: want nil, got %v", got)
	}

	// 无 --cookies 可换 → nil
	t.Setenv("YOUTUBE_COOKIES_FROM_BROWSER", "")
	if got := swapCookiesToBrowser([]string{"--dump-json"}); got != nil {
		t.Fatalf("no --cookies: want nil, got %v", got)
	}

	// 显式指定浏览器
	t.Setenv("YOUTUBE_COOKIES_FROM_BROWSER", "firefox:profile1")
	got = swapCookiesToBrowser(base)
	want = []string{"--dump-json", "--cookies-from-browser", "firefox:profile1"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("explicit browser: swapCookiesToBrowser = %v, want %v", got, want)
	}
}

func TestExtractYTDLPErrorPrefersERRORLine(t *testing.T) {
	stderr := "WARNING: some warning\nERROR: [youtube] xx: The real error\nsome trailing"
	got := extractYTDLPError(nil, stderr)
	if got != "ERROR: [youtube] xx: The real error" {
		t.Fatalf("got %q", got)
	}
	if got := extractYTDLPError(nil, ""); got != "yt-dlp failed" {
		t.Fatalf("empty stderr: got %q", got)
	}
	// 仅有 warning 时取末行
	if got := extractYTDLPError(nil, "WARNING: a\nWARNING: b"); got != "WARNING: b" {
		t.Fatalf("warning only: got %q", got)
	}
}
