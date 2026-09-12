package transcriber

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveWhisperModelExistingPath(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "ggml-base.bin")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := resolveWhisperModel(p); got != p {
		t.Errorf("已存在路径应原样返回，得到 %q", got)
	}
}

func TestResolveWhisperModelFallsBackToHomeDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	nested := filepath.Join(home, ".biliup", "models")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(nested, "ggml-base.bin")
	if err := os.WriteFile(want, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 相对路径 & cwd 下不存在 → 应命中 ~/.biliup/models/ggml-base.bin
	if got := resolveWhisperModel("models/ggml-base.bin"); got != want {
		t.Errorf("应回退到 %q，实际 %q", want, got)
	}
}

func TestResolveWhisperModelAbsentReturnsInput(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	in := filepath.Join(home, "nope", "ggml-tiny.bin") // 绝对路径且不存在
	if got := resolveWhisperModel(in); got != in {
		t.Errorf("绝对路径找不到时应原样返回 %q，实际 %q", in, got)
	}
	if got := resolveWhisperModel(""); got != "" {
		t.Errorf("空路径应返回空，实际 %q", got)
	}
}

func TestResolveWhisperModelNotFoundKeepsRelative(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if got := resolveWhisperModel("models/ggml-large-v3.bin"); got != "models/ggml-large-v3.bin" {
		t.Errorf("找不到模型时应保持原值以便报错，实际 %q", got)
	}
}
