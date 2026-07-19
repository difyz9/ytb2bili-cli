package download

import (
	"reflect"
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
