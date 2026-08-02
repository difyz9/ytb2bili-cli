package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/zolagz/ytb2bili-go/internal/channel"
	"github.com/zolagz/ytb2bili-go/internal/search"
	"github.com/zolagz/ytb2bili-go/internal/storage"
)

// statusIcon 返回任务/步骤状态的图标
func statusIcon(status string) string {
	switch status {
	case "", "pending":
		return "⏳"
	case "running":
		return "🔄"
	case "completed":
		return "✅"
	case "failed":
		return "❌"
	default:
		return "❓"
	}
}

// truncateTime 截断 RFC3339 时间戳到秒精度（前 19 位）
func truncateTime(ts string) string {
	if len(ts) > 19 {
		return ts[:19]
	}
	return ts
}

// taskProgress 返回已完成的步骤数和步骤总数
func taskProgress(t *storage.Task) (done, total int) {
	total = len(t.Steps)
	for _, s := range t.Steps {
		if s.Status == "completed" {
			done++
		}
	}
	return done, total
}

// formatTaskSummary 渲染任务列表中的一行
func formatTaskSummary(t *storage.Task) string {
	done, total := taskProgress(t)
	url := t.SourceURL
	if len(url) > 60 {
		url = url[:60] + "..."
	}
	return fmt.Sprintf("%s %s [%d/%d] %s %s",
		statusIcon(t.Status), t.ID, done, total, url, truncateTime(t.UpdatedAt))
}

// formatTaskDetail 渲染任务详情
func formatTaskDetail(t *storage.Task) string {
	var b strings.Builder
	fmt.Fprintln(&b, "📋 任务详情")
	fmt.Fprintf(&b, "  ID:     %s %s\n", statusIcon(t.Status), t.ID)
	if t.Title != "" {
		fmt.Fprintf(&b, "  标题:   %s\n", t.Title)
	}
	if t.BVID != "" {
		fmt.Fprintf(&b, "  BVID:   %s\n", t.BVID)
	}
	fmt.Fprintf(&b, "  来源:   %s\n", t.SourceURL)
	if t.CreatedAt != "" {
		fmt.Fprintf(&b, "  创建:   %s\n", truncateTime(t.CreatedAt))
	}
	if t.UpdatedAt != "" {
		fmt.Fprintf(&b, "  更新:   %s\n", truncateTime(t.UpdatedAt))
	}
	fmt.Fprintln(&b, "  步骤:")
	names := make([]string, 0, len(t.Steps))
	for name := range t.Steps {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		s := t.Steps[name]
		line := fmt.Sprintf("    %-12s %s", name, statusIcon(s.Status))
		if s.Error != "" {
			line += "  " + s.Error
		}
		fmt.Fprintln(&b, line)
	}
	return b.String()
}

// searchFilter 根据 CLI flag 构建搜索过滤器；全部为空时返回 nil
func searchFilter(sortBy, uploadDate, duration string) *search.SearchFilter {
	if sortBy == "" && uploadDate == "" && duration == "" {
		return nil
	}
	return &search.SearchFilter{
		SortBy:     sortBy,
		UploadDate: uploadDate,
		Duration:   duration,
	}
}

// formatSearchResults 渲染搜索结果列表，带已提交标记
func formatSearchResults(result *search.SearchResult, submitted map[string]bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "🔍 搜索: %s\n", result.Query)
	fmt.Fprintf(&b, "📺 找到 %d 个视频:\n\n", len(result.Videos))
	for i, v := range result.Videos {
		status := ""
		if submitted[v.ID] {
			status = " ✅已提交"
		}
		fmt.Fprintf(&b, "%d. %s%s\n", i+1, v.Title, status)
		fmt.Fprintf(&b, "   频道: %s\n", v.Channel)
		fmt.Fprintf(&b, "   时长: %s | 观看: %s | %s\n", v.Duration, v.Views, v.PublishTime)
		fmt.Fprintf(&b, "   链接: %s\n\n", v.URL)
	}
	return b.String()
}

// renderHistory 渲染提交历史列表
func renderHistory(videos []storage.SubmittedVideo) string {
	var b strings.Builder
	if len(videos) == 0 {
		fmt.Fprintln(&b, "📭 暂无提交历史")
		return b.String()
	}
	fmt.Fprintf(&b, "📜 共 %d 条提交记录:\n\n", len(videos))
	for _, v := range videos {
		fmt.Fprintf(&b, "  %s\n", v.Title)
		fmt.Fprintf(&b, "    BVID:     %s\n", v.BVID)
		fmt.Fprintf(&b, "    YouTube:  %s\n", v.YouTubeID)
		fmt.Fprintf(&b, "    提交时间: %s\n\n", truncateTime(v.SubmittedAt))
	}
	return b.String()
}

// renderDiscoveredVideos 渲染频道发现的视频列表
func renderDiscoveredVideos(videos []channel.DiscoveredVideo) string {
	var b strings.Builder
	if len(videos) == 0 {
		fmt.Fprintln(&b, "📭 暂无发现视频")
		return b.String()
	}
	fmt.Fprintf(&b, "📺 共 %d 个发现视频:\n\n", len(videos))
	for _, v := range videos {
		fmt.Fprintf(&b, "  [%s] %s\n", v.Status, v.Title)
		fmt.Fprintf(&b, "    ID:   %s\n", v.VideoID)
		fmt.Fprintf(&b, "    链接: %s\n", v.URL)
		fmt.Fprintf(&b, "    发布: %s\n\n", truncateTime(v.PublishedAt))
	}
	return b.String()
}
