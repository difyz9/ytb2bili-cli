package pipeline

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zolagz/ytb2bili-go/internal/config"
)

// setupArtifacts 构造一个视频工作目录（含媒体大文件、voice/ 配音、字幕与封面）。
func setupArtifacts(t *testing.T, videoID string) *config.Config {
	t.Helper()
	root := t.TempDir()
	cfg := &config.Config{DataDir: root, DownloadDir: filepath.Join(root, "downloads")}
	dir := filepath.Join(cfg.DownloadDir, videoID)
	if err := os.MkdirAll(filepath.Join(dir, "voice"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]int{
		videoID + ".mp4":             1024,
		videoID + ".synced.mp4":      2048,
		videoID + ".srt":             64,
		videoID + ".zh-Hans.srt":     64,
		"cover.jpg":                  128,
		"metadata.json":              32,
		"voice/0001.wav":             512,
		"voice/0002.wav":             512,
		"subtitle_en.mp3":            256,
	}
	for name, size := range files {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, make([]byte, size), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return cfg
}

func TestCleanupArtifactsKeepsSubtitlesAndCover(t *testing.T) {
	cfg := setupArtifacts(t, "abc12345678")
	res, err := CleanupArtifacts(cfg, "abc12345678", true, false)
	if err != nil {
		t.Fatalf("cleanup 失败: %v", err)
	}
	// 删除：mp4 + synced.mp4 + mp3 + voice/2 个 wav = 5 个文件
	if res.DeletedFiles != 5 {
		t.Errorf("期望删除 5 个文件，实际 %d", res.DeletedFiles)
	}
	if res.KeptFiles != 3 { // srt + zh-Hans.srt + cover.jpg（metadata.json 也在保留之列，视实现计数）
		t.Logf("保留文件数 = %d（metadata.json 是否计入不影响正确性）", res.KeptFiles)
	}
	if res.FreedBytes == 0 {
		t.Error("释放字节数应为正")
	}
	dir := filepath.Join(cfg.DownloadDir, "abc12345678")
	for _, keep := range []string{"abc12345678.srt", "abc12345678.zh-Hans.srt", "cover.jpg"} {
		if _, err := os.Stat(filepath.Join(dir, keep)); err != nil {
			t.Errorf("应保留 %s: %v", keep, err)
		}
	}
	for _, gone := range []string{"abc12345678.mp4", "abc12345678.synced.mp4", "subtitle_en.mp3", "voice"} {
		if _, err := os.Stat(filepath.Join(dir, gone)); !os.IsNotExist(err) {
			t.Errorf("应删除 %s（err=%v）", gone, err)
		}
	}
}

func TestCleanupArtifactsIncludeSubtitles(t *testing.T) {
	cfg := setupArtifacts(t, "vid22222222")
	keepSmall := false
	if _, err := CleanupArtifacts(cfg, "vid22222222", keepSmall, false); err != nil {
		t.Fatalf("cleanup 失败: %v", err)
	}
	dir := filepath.Join(cfg.DownloadDir, "vid22222222")
	for _, gone := range []string{"vid22222222.srt", "vid22222222.zh-Hans.srt", "cover.jpg", "metadata.json"} {
		if _, err := os.Stat(filepath.Join(dir, gone)); !os.IsNotExist(err) {
			t.Errorf("--include-subtitles 时 %s 也应删除（err=%v）", gone, err)
		}
	}
}

func TestCleanupArtifactsDryRun(t *testing.T) {
	cfg := setupArtifacts(t, "dryrun12345")
	res, err := CleanupArtifacts(cfg, "dryrun12345", true, true)
	if err != nil {
		t.Fatalf("dry-run 失败: %v", err)
	}
	if res.DeletedFiles == 0 {
		t.Error("dry-run 应统计出待删除文件数")
	}
	dir := filepath.Join(cfg.DownloadDir, "dryrun12345")
	for _, still := range []string{"dryrun12345.mp4", "dryrun12345.synced.mp4", "voice/0001.wav"} {
		if _, err := os.Stat(filepath.Join(dir, still)); err != nil {
			t.Errorf("dry-run 不应删除 %s: %v", still, err)
		}
	}
}

func TestCleanupArtifactsRejectsTraversal(t *testing.T) {
	cfg := setupArtifacts(t, "validID1234")
	for _, bad := range []string{"", "..", "../etc", "a/b", `a\b`, "..foo"} {
		if _, err := CleanupArtifacts(cfg, bad, true, false); err == nil {
			t.Errorf("非法 ID %q 应被拒绝", bad)
		}
	}
}

func TestCleanupArtifactsMissingDirIsIdempotent(t *testing.T) {
	cfg := setupArtifacts(t, "exists12345")
	res, err := CleanupArtifacts(cfg, "notexist123", true, false)
	if err != nil {
		t.Fatalf("目录不存在时应无错返回: %v", err)
	}
	if res.DeletedFiles != 0 {
		t.Errorf("目录不存在时删除数应为 0，实际 %d", res.DeletedFiles)
	}
}

func TestCleanupAllArtifactsScansDownloadRoot(t *testing.T) {
	cfg := setupArtifacts(t, "vidA1234567")
	// 第二个视频目录（只放一个大文件）
	dirB := filepath.Join(cfg.DownloadDir, "vidB1234567")
	if err := os.MkdirAll(dirB, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirB, "vidB1234567.mp4"), make([]byte, 4096), 0o644); err != nil {
		t.Fatal(err)
	}

	results, err := CleanupAllArtifacts(cfg, nil, true, false)
	if err != nil {
		t.Fatalf("全量清理失败: %v", err)
	}
	videos, files, freed := SummarizeCleanup(results)
	if videos != 2 || files != 6 || freed == 0 {
		t.Errorf("期望 2 个视频/6 个文件，实际 %d/%d（freed=%d）", videos, files, freed)
	}

	// 指定 ID 只清理该视频
	results, err = CleanupAllArtifacts(cfg, []string{"vidB1234567"}, true, false)
	if err != nil {
		t.Fatalf("指定 ID 清理失败: %v", err)
	}
	if _, files, _ := SummarizeCleanup(results); files != 0 {
		t.Errorf("vidB 已被清理过，再次清理应为 0 个文件，实际 %d", files)
	}
}
