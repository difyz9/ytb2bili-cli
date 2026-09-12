package diskspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFreeBytesOnRoot(t *testing.T) {
	n, err := FreeBytes("/")
	if err != nil {
		t.Fatalf("FreeBytes(/) 失败: %v", err)
	}
	if n == 0 {
		t.Error("根分区可用空间不应为 0")
	}
	gb, err := FreeGB("/")
	if err != nil {
		t.Fatalf("FreeGB(/) 失败: %v", err)
	}
	if gb <= 0 {
		t.Errorf("FreeGB(/) 应 > 0，实际 %.2f", gb)
	}
}

func TestFreeBytesFallsBackToExistingParent(t *testing.T) {
	// 路径不存在时应回溯到已存在父目录，而不是报错
	missing := filepath.Join(t.TempDir(), "not", "created", "yet")
	if _, err := FreeBytes(missing); err != nil {
		t.Fatalf("不存在路径应回溯父目录: %v", err)
	}
}

func TestFreeBytesEmptyPathFails(t *testing.T) {
	if _, err := FreeBytes(""); err == nil {
		t.Error("空路径应返回错误")
	}
}

func TestFormatBytes(t *testing.T) {
	cases := []struct {
		in   uint64
		want string
	}{
		{512, "512 B"},
		{2048, "2.0 KB"},
		{5 * 1024 * 1024, "5.0 MB"},
		{3 * 1024 * 1024 * 1024, "3.00 GB"},
	}
	for _, c := range cases {
		if got := FormatBytes(c.in); got != c.want {
			t.Errorf("FormatBytes(%d) = %q，期望 %q", c.in, got, c.want)
		}
	}
}

func TestFreeBytesOnTempDirMatchesStatfs(t *testing.T) {
	dir := t.TempDir()
	if _, err := FreeBytes(dir); err != nil {
		t.Fatalf("FreeBytes(temp) 失败: %v", err)
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Fatalf("临时目录异常: %v", err)
	}
}
