package bili

import (
	"fmt"
	"path/filepath"

	"github.com/zolagz/ytb2bili-go/internal/auth"
	"github.com/zolagz/ytb2bili-go/internal/storage"
)

// SubtitleWatchLogger 字幕监听日志接口，由调用方提供
// command.go 使用 fmt.Printf 输出到终端
// server.go 使用 log.Printf 输出到日志
type SubtitleWatchLogger interface {
	Printf(format string, args ...interface{})
}

// watchPrintfLogger fmt.Printf 适配
type watchPrintfLogger struct{}

func (watchPrintfLogger) Printf(format string, args ...interface{}) {
	fmt.Printf(format, args...)
}

// FmtLogger 返回使用 fmt.Printf 的日志记录器
func FmtLogger() SubtitleWatchLogger {
	return watchPrintfLogger{}
}

// WatchAndUploadSubtitle 异步监听B站视频审核状态，审核通过后上传字幕
// 被 command.go 和 server.go 共享使用，消除代码重复
// 参数:
//
//	bvid: B站视频ID
//	videoID: YouTube 视频ID 或 task ID
//	dlDir: 字幕文件所在目录
//	cred: B站登录凭证
//	dataDir: 应用数据根目录
//	logger: 日志输出接口
func WatchAndUploadSubtitle(bvid, videoID, dlDir string, cred *auth.LoginInfo, dataDir string, logger SubtitleWatchLogger) {
	subStore := storage.NewSubtitleStore(filepath.Join(dataDir, "subtitles"))

	// 同步最新状态
	subStore.SyncFromDownload(videoID, bvid, dlDir)
	pending := subStore.GetPending(videoID)
	if len(pending) == 0 {
		return
	}

	logger.Printf("\n⏳ [字幕] 监听视频 %s 审核状态 (共 %d 个字幕待上传)...\n", bvid, len(pending))

	// 等待审核通过
	status, err := WaitForReviewPassed(cred, bvid)
	if err != nil {
		logger.Printf("❌ [字幕] 等待审核失败: %v\n", err)
		return
	}
	if status == nil {
		logger.Printf("❌ [字幕] 获取审核状态失败\n")
		return
	}
	logger.Printf("✅ [字幕] 视频审核通过 (state=%d)\n", status.State)

	// 重新同步（字幕文件可能已更新）
	subStore.SyncFromDownload(videoID, bvid, dlDir)
	pending = subStore.GetPending(videoID)
	if len(pending) == 0 {
		logger.Printf("ℹ️ [字幕] 没有待上传的字幕文件\n")
		return
	}

	successCount := 0
	for _, track := range pending {
		logger.Printf("  📤 上传字幕: %s (%s)... ", track.FileName, track.Language)

		err := UploadSubtitle(cred, bvid, track.FilePath, track.Language)
		if err != nil {
			logger.Printf("❌ %v\n", err)
			subStore.MarkFailed(videoID, track.Language, err.Error())
			continue
		}

		subStore.MarkUploaded(videoID, track.Language)
		logger.Printf("✅\n")
		successCount++
	}

	if successCount > 0 {
		allDone := subStore.AllUploaded(videoID)
		if allDone {
			logger.Printf("✅ [字幕] 全部字幕上传完成! https://www.bilibili.com/video/%s\n", bvid)
		} else {
			logger.Printf("✅ [字幕] 已上传 %d 个字幕文件，部分仍待处理\n", successCount)
		}
	} else {
		logger.Printf("❌ [字幕] 所有字幕上传均失败，请稍后重试: ytb2bili subtitle retry %s\n", bvid)
	}
}
