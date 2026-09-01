package resource

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProjectRootLocatesSkillsDir(t *testing.T) {
	root := ProjectRoot()
	if !hasSkillsDir(root) {
		t.Fatalf("ProjectRoot()=%s 下不存在 skills/ 目录", root)
	}
}

func TestSkillScriptJoinsSkillsDir(t *testing.T) {
	got := SkillScript(filepath.Join("audio-video-sync", "requirements.txt"))
	want := filepath.Join(ProjectRoot(), "skills", "audio-video-sync", "requirements.txt")
	if got != want {
		t.Fatalf("SkillScript()=%s, want %s", got, want)
	}
}

func TestSetSkillsDirOverride(t *testing.T) {
	t.Setenv("YTB2BILI_PROJECT_DIR", "") // 清除环境干扰

	// 绝对路径直接生效
	abs := t.TempDir()
	SetSkillsDir(abs)
	t.Cleanup(func() { SetSkillsDir("") })
	if got := SkillsDir(); got != abs {
		t.Fatalf("SkillsDir()=%s, want %s", got, abs)
	}

	// 相对路径相对项目根解析
	SetSkillsDir(filepath.Join("assets", "skills"))
	if got, want := SkillsDir(), filepath.Join(ProjectRoot(), "assets", "skills"); got != want {
		t.Fatalf("SkillsDir()=%s, want %s", got, want)
	}

	// 空值恢复自动探测
	SetSkillsDir("")
	if got, want := SkillsDir(), filepath.Join(ProjectRoot(), "skills"); got != want {
		t.Fatalf("SkillsDir()=%s, want %s", got, want)
	}
}

func TestProjectRootEnvOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("YTB2BILI_PROJECT_DIR", dir)
	if got := ProjectRoot(); got != dir {
		t.Fatalf("ProjectRoot()=%s, want %s", got, dir)
	}
}

func TestVenvPythonFallback(t *testing.T) {
	// CI 等无 .venv 环境下应回退 PATH 中的 python3
	root := ProjectRoot()
	if _, err := os.Stat(filepath.Join(root, ".venv", "bin", "python3")); err == nil {
		t.Skip("本机存在 .venv，跳过回退路径测试")
	}
	if got := VenvPython(); got != "python3" {
		t.Fatalf("VenvPython()=%s, want python3", got)
	}
}
