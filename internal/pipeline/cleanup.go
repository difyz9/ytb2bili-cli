package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zolagz/ytb2bili-go/internal/config"
	"github.com/zolagz/ytb2bili-go/internal/diskspace"
)

// mediaExtensions 需要清理的重型媒体产物扩展名（下载视频、音画同步产物、配音中间音频等）。
// 白名单思路：只删这里的媒体类型 + voice/ 目录，其它文件（字幕、封面、元数据）一律保留。
var mediaExtensions = map[string]bool{
	".mp4": true, ".mkv": true, ".webm": true, ".flv": true, ".avi": true,
	".mov": true, ".m4v": true, ".ts": true, ".part": true, ".ytdl": true,
	".m4a": true, ".mp3": true, ".wav": true, ".aac": true, ".flac": true,
	".ogg": true, ".opus": true, ".wma": true,
}

// voiceDirName 配音中间产物目录名（IndexTTS 逐段 wav）。
const voiceDirName = "voice"

// CleanupResult 单个视频工作目录的清理结果。
type CleanupResult struct {
	VideoID      string
	DeletedFiles int
	FreedBytes   uint64
	KeptFiles    int
	DryRun       bool
}

// ErrInvalidVideoID 视频 ID 非法（含路径分隔符/上跳），拒绝清理以防误删。
var ErrInvalidVideoID = fmt.Errorf("非法的视频 ID")

// validArtifactID 校验视频 ID 可安全用作目录名（禁止路径穿越）。
func validArtifactID(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" || id == "." || id == ".." {
		return false
	}
	if strings.ContainsAny(id, `/\`) || strings.Contains(id, "..") {
		return false
	}
	return true
}

// CleanupArtifacts 清理某个视频工作目录 <download_dir>/<videoID>/ 下的重型产物：
//   - 删除媒体文件（视频/音画同步产物/音频）与 voice/ 配音目录
//   - keepSmall=true 时保留字幕(.srt)、封面(.jpg/.png/.webp) 等小文件（默认行为）
//
// dryRun=true 时只统计不删除。字幕必须在投稿后保留：审核通过后仍需异步上传字幕。
func CleanupArtifacts(cfg *config.Config, videoID string, keepSmall, dryRun bool) (CleanupResult, error) {
	res := CleanupResult{VideoID: videoID, DryRun: dryRun}
	if !validArtifactID(videoID) {
		return res, fmt.Errorf("%w: %q", ErrInvalidVideoID, videoID)
	}
	dir := filepath.Join(cfg.EffectiveDownloadDir(), strings.TrimSpace(videoID))
	fi, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return res, nil // 目录不存在 = 无需清理（幂等）
		}
		return res, fmt.Errorf("读取目录失败 %s: %w", dir, err)
	}
	if !fi.IsDir() {
		return res, fmt.Errorf("不是目录: %s", dir)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return res, fmt.Errorf("遍历目录失败 %s: %w", dir, err)
	}

	for _, e := range entries {
		p := filepath.Join(dir, e.Name())
		if e.IsDir() {
			if e.Name() == voiceDirName {
				n, b, herr := dirStats(p)
				if herr != nil {
					return res, herr
				}
				if !dryRun {
					if rerr := os.RemoveAll(p); rerr != nil {
						return res, fmt.Errorf("删除配音目录失败 %s: %w", p, rerr)
					}
				}
				res.DeletedFiles += n
				res.FreedBytes += b
				continue
			}
			// 其它子目录（如历史遗留的 audio/）按内容递归判断
			n, b, herr := cleanupMediaInDir(p, dryRun)
			if herr != nil {
				return res, herr
			}
			res.DeletedFiles += n
			res.FreedBytes += b
			continue
		}

		ext := strings.ToLower(filepath.Ext(e.Name()))
		// keepSmall=true：只删媒体文件，字幕/封面/元数据保留；
		// keepSmall=false：连字幕封面一起删（ytb clean --include-subtitles）。
		if keepSmall && !mediaExtensions[ext] {
			res.KeptFiles++
			continue
		}
		info, ierr := e.Info()
		if ierr != nil {
			// 文件可能刚好被删除，跳过
			continue
		}
		size := uint64(info.Size())
		if !dryRun {
			if rerr := os.Remove(p); rerr != nil {
				return res, fmt.Errorf("删除文件失败 %s: %w", p, rerr)
			}
		}
		res.DeletedFiles++
		res.FreedBytes += size
	}
	return res, nil
}

// cleanupMediaInDir 递归清理目录内的媒体文件（不含 voice 特判），返回删除数与释放字节。
func cleanupMediaInDir(dir string, dryRun bool) (int, uint64, error) {
	var files int
	var freed uint64
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, werr error) error {
		if werr != nil {
			return nil // 单个文件不可读不影响整体清理
		}
		if d.IsDir() {
			return nil
		}
		if !mediaExtensions[strings.ToLower(filepath.Ext(p))] {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			return nil
		}
		if !dryRun {
			if rerr := os.Remove(p); rerr != nil {
				return rerr
			}
		}
		files++
		freed += uint64(info.Size())
		return nil
	})
	if err != nil {
		return files, freed, err
	}
	return files, freed, nil
}

// dirStats 统计目录内文件数/总字节（用于 dry-run 与删除前估算）。
func dirStats(dir string) (int, uint64, error) {
	var files int
	var size uint64
	err := filepath.WalkDir(dir, func(_ string, d os.DirEntry, werr error) error {
		if werr != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if info, ierr := d.Info(); ierr == nil {
			files++
			size += uint64(info.Size())
		}
		return nil
	})
	return files, size, err
}

// CleanupAllArtifacts 清理下载目录下所有视频工作目录（daemon 启动清理 / ytb clean 用）。
// videoIDs 为空时扫描整个下载目录；否则只清理指定 ID。
func CleanupAllArtifacts(cfg *config.Config, videoIDs []string, keepSmall, dryRun bool) ([]CleanupResult, error) {
	ids := videoIDs
	if len(ids) == 0 {
		root := cfg.EffectiveDownloadDir()
		entries, err := os.ReadDir(root)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, nil
			}
			return nil, fmt.Errorf("读取下载目录失败 %s: %w", root, err)
		}
		for _, e := range entries {
			if e.IsDir() {
				ids = append(ids, e.Name())
			}
		}
	}
	results := make([]CleanupResult, 0, len(ids))
	for _, id := range ids {
		r, err := CleanupArtifacts(cfg, id, keepSmall, dryRun)
		if err != nil {
			return results, err
		}
		results = append(results, r)
	}
	return results, nil
}

// SummarizeCleanup 汇总清理结果（视频数/文件数/释放空间）。
func SummarizeCleanup(results []CleanupResult) (videos, files int, freed uint64) {
	for _, r := range results {
		if r.DeletedFiles > 0 {
			videos++
		}
		files += r.DeletedFiles
		freed += r.FreedBytes
	}
	return
}

// CleanupLogLine 生成清理日志（daemon/CLI 共用）。
func CleanupLogLine(r CleanupResult) string {
	if r.DryRun {
		return fmt.Sprintf("🧹 [dry-run] %s: 将删除 %d 个媒体文件，可释放 %s", r.VideoID, r.DeletedFiles, diskspace.FormatBytes(r.FreedBytes))
	}
	return fmt.Sprintf("🧹 已清理 %s: 删除 %d 个媒体文件，释放 %s（保留 %d 个小文件）", r.VideoID, r.DeletedFiles, diskspace.FormatBytes(r.FreedBytes), r.KeptFiles)
}
