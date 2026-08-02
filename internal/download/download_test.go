package download

import (
	"os"
	"path/filepath"
	"reflect"
	"os/exec"
	"testing"
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

	baseArgs, _, err := prepareYTDLP(cookies)
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
