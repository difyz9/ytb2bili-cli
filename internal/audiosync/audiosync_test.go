package audiosync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScriptPathFindsBundledSkill(t *testing.T) {
	path, err := scriptPath()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(path, "skills/audio-video-sync/scripts/audio_processor_v2.py") {
		t.Fatalf("path=%q", path)
	}
}

func TestConfiguredMissingScriptFails(t *testing.T) {
	t.Setenv("YTB2BILI_AUDIO_SYNC_SCRIPT", t.TempDir()+"/missing.py")
	_, err := scriptPath()
	if err == nil {
		t.Fatal("expected error")
	}
	_ = os.Unsetenv("YTB2BILI_AUDIO_SYNC_SCRIPT")
}

func TestPythonPath(t *testing.T) {
	t.Run("respects YTB2BILI_PYTHON env", func(t *testing.T) {
		t.Setenv("YTB2BILI_PYTHON", "/custom/python3")
		if got := pythonPath(); got != "/custom/python3" {
			t.Fatalf("got %q, want /custom/python3", got)
		}
	})

	t.Run("prefers .venv/bin/python3 when present", func(t *testing.T) {
		t.Setenv("YTB2BILI_PYTHON", "")
		dir := t.TempDir()
		venvBin := filepath.Join(dir, ".venv", "bin")
		os.MkdirAll(venvBin, 0755)
		os.WriteFile(filepath.Join(venvBin, "python3"), []byte("#!/bin/sh"), 0755)
		t.Chdir(dir)
		got := pythonPath()
		want := filepath.Join(dir, ".venv", "bin", "python3")
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("falls back to python3", func(t *testing.T) {
		t.Setenv("YTB2BILI_PYTHON", "")
		t.Chdir(t.TempDir())
		if got := pythonPath(); got != "python3" {
			t.Fatalf("got %q, want python3", got)
		}
	})
}
