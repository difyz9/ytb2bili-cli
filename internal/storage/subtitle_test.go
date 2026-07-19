package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildSubtitleCandidatesPrefersZhHansName(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"video.en.zh.srt", "video.zh-Hans.srt", "video.en.srt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("subtitle"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	tracks := BuildSubtitleCandidates("video", dir)
	for _, track := range tracks {
		if track.Language == "zh" {
			if track.FileName != "video.zh-Hans.srt" {
				t.Fatalf("simplified Chinese candidate=%q", track.FileName)
			}
			return
		}
	}
	t.Fatal("missing simplified Chinese candidate")
}
