package pipeline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExistingVideo(t *testing.T) {
	t.Run("finds mp4 and excludes synced", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "abc123.mp4"), []byte("x"), 0644)
		os.WriteFile(filepath.Join(dir, "abc123.synced.mp4"), []byte("x"), 0644)
		got := existingVideo(dir)
		if got != filepath.Join(dir, "abc123.mp4") {
			t.Fatalf("got %q, want %q", got, filepath.Join(dir, "abc123.mp4"))
		}
	})

	t.Run("returns empty when no video", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "abc123.srt"), []byte("x"), 0644)
		if got := existingVideo(dir); got != "" {
			t.Fatalf("got %q, want empty", got)
		}
	})

	t.Run("finds webm too", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "abc123.webm"), []byte("x"), 0644)
		if got := existingVideo(dir); got == "" {
			t.Fatal("expected webm to be found")
		}
	})

	t.Run("empty dir", func(t *testing.T) {
		if got := existingVideo(t.TempDir()); got != "" {
			t.Fatalf("got %q, want empty", got)
		}
	})
}

func TestExistingFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "abc123.srt"), []byte("x"), 0644)
	if got := existingFile(dir, "abc123.srt"); got == "" {
		t.Fatal("expected existing file to be found")
	}
	if got := existingFile(dir, "missing.srt"); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestHasVoiceClips(t *testing.T) {
	t.Run("false when dir missing", func(t *testing.T) {
		if hasVoiceClips(filepath.Join(t.TempDir(), "voice")) {
			t.Fatal("expected false for missing dir")
		}
	})

	t.Run("true with non-empty clip", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "voice")
		os.MkdirAll(dir, 0755)
		os.WriteFile(filepath.Join(dir, "1.mp3"), []byte("audio"), 0644)
		if !hasVoiceClips(dir) {
			t.Fatal("expected true with clips")
		}
	})

	t.Run("false with empty file only", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "voice")
		os.MkdirAll(dir, 0755)
		os.WriteFile(filepath.Join(dir, "1.mp3"), []byte(""), 0644)
		if hasVoiceClips(dir) {
			t.Fatal("expected false with empty file")
		}
	})
}

func TestResolveSyncArtifacts(t *testing.T) {
	t.Run("all artifacts present, prefers zh-Hans", func(t *testing.T) {
		dataDir := t.TempDir()
		dlDir := filepath.Join(dataDir, "downloads", "abc123")
		os.MkdirAll(filepath.Join(dlDir, "voice"), 0755)
		os.WriteFile(filepath.Join(dlDir, "abc123.mp4"), []byte("v"), 0644)
		os.WriteFile(filepath.Join(dlDir, "abc123.srt"), []byte("s"), 0644)
		os.WriteFile(filepath.Join(dlDir, "abc123.zh-Hans.srt"), []byte("t"), 0644)
		os.WriteFile(filepath.Join(dlDir, "voice", "1.mp3"), []byte("a"), 0644)

		video, subtitle, voiceDir, err := ResolveSyncArtifacts(dataDir, "abc123")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(video, "abc123.mp4") {
			t.Fatalf("video=%q", video)
		}
		if !strings.HasSuffix(subtitle, "abc123.zh-Hans.srt") {
			t.Fatalf("subtitle=%q, want zh-Hans preferred", subtitle)
		}
		if !strings.HasSuffix(voiceDir, "voice") {
			t.Fatalf("voiceDir=%q", voiceDir)
		}
	})

	t.Run("falls back to source srt when no translation", func(t *testing.T) {
		dataDir := t.TempDir()
		dlDir := filepath.Join(dataDir, "downloads", "abc123")
		os.MkdirAll(filepath.Join(dlDir, "voice"), 0755)
		os.WriteFile(filepath.Join(dlDir, "abc123.mp4"), []byte("v"), 0644)
		os.WriteFile(filepath.Join(dlDir, "abc123.srt"), []byte("s"), 0644)
		os.WriteFile(filepath.Join(dlDir, "voice", "1.mp3"), []byte("a"), 0644)

		_, subtitle, _, err := ResolveSyncArtifacts(dataDir, "abc123")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(subtitle, "abc123.srt") {
			t.Fatalf("subtitle=%q", subtitle)
		}
	})

	t.Run("missing video errors", func(t *testing.T) {
		if _, _, _, err := ResolveSyncArtifacts(t.TempDir(), "nope123"); err == nil {
			t.Fatal("expected error for missing video")
		}
	})

	t.Run("missing voice errors", func(t *testing.T) {
		dataDir := t.TempDir()
		dlDir := filepath.Join(dataDir, "downloads", "abc123")
		os.MkdirAll(dlDir, 0755)
		os.WriteFile(filepath.Join(dlDir, "abc123.mp4"), []byte("v"), 0644)
		os.WriteFile(filepath.Join(dlDir, "abc123.srt"), []byte("s"), 0644)
		if _, _, _, err := ResolveSyncArtifacts(dataDir, "abc123"); err == nil {
			t.Fatal("expected error for missing voice")
		}
	})
}
