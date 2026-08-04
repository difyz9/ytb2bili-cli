package channel

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/zolagz/ytb2bili-go/internal/storage"
)

// ─── Data Structures ──────────────────────────────────────────────────────────

// Subscription 表示一个被监控的 YouTube 频道
type Subscription struct {
	ChannelID    string `json:"channel_id"`
	ChannelTitle string `json:"channel_title"`
	Type         string `json:"type,omitempty"` // channel / playlist
	AddedAt      string `json:"added_at"`
	LastSyncAt   string `json:"last_sync_at,omitempty"`
	Status       string `json:"status"` // active / inactive
}

// DetectType 根据 ID 前缀判断订阅类型。
// 频道 ID 固定以 UC 开头；其余（PL/UU/FL 等）视为播放列表。
func DetectType(id string) string {
	if strings.HasPrefix(id, "UC") {
		return "channel"
	}
	return "playlist"
}

// feedURL 返回该订阅对应的 YouTube RSS feed 地址
func (s Subscription) feedURL() string {
	if s.Type == "playlist" {
		return fmt.Sprintf("https://www.youtube.com/feeds/videos.xml?playlist_id=%s", s.ChannelID)
	}
	return fmt.Sprintf("https://www.youtube.com/feeds/videos.xml?channel_id=%s", s.ChannelID)
}

// ChannelURL 返回该订阅的频道主页 URL（供 yt-dlp 抓取）。
func (s Subscription) ChannelURL() string {
	if s.Type == "playlist" {
		return fmt.Sprintf("https://www.youtube.com/playlist?list=%s", s.ChannelID)
	}
	return fmt.Sprintf("https://www.youtube.com/channel/%s", s.ChannelID)
}

// ResolveHandle 将 @handle 解析为 channel_id（用 yt-dlp 查询）。
func ResolveHandle(ctx context.Context, handle string) (string, error) {
	handle = strings.TrimPrefix(handle, "@")
	url := "https://www.youtube.com/@" + handle
	cmd := exec.CommandContext(ctx, "yt-dlp",
		"--skip-download", "--no-warnings",
		"--print", "%(channel_id)s",
		url)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &bytes.Buffer{}
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("yt-dlp 解析 @%s 失败: %w", handle, err)
	}
	id := strings.TrimSpace(out.String())
	if id == "" || !strings.HasPrefix(id, "UC") {
		return "", fmt.Errorf("无法从 @%s 解析出有效频道 ID", handle)
	}
	return id, nil
}

// DiscoveredVideo 表示通过 RSS 发现的视频
type DiscoveredVideo struct {
	VideoID      string `json:"video_id"`
	ChannelID    string `json:"channel_id"`
	Title        string `json:"title"`
	URL          string `json:"url"`
	PublishedAt  string `json:"published_at"`
	DiscoveredAt string `json:"discovered_at"`
	Views        int    `json:"views,omitempty"` // 发现时播放量（来自 RSS media:statistics）
	Status       string `json:"status"`          // new / queued / submitted / skipped
}

// YouTubeFeed RSS feed 结构
type YouTubeFeed struct {
	XMLName xml.Name       `xml:"feed"`
	Title   string         `xml:"title"`
	Entries []YouTubeEntry `xml:"entry"`
}

// YouTubeEntry RSS feed 中的单个视频条目
type YouTubeEntry struct {
	ID        string  `xml:"id"`
	Title     string  `xml:"title"`
	Link      YTLink  `xml:"link"`
	Published string  `xml:"published"`
	Updated   string  `xml:"updated"`
	VideoID   YTID    `xml:"videoId"`
	ChannelID YTID    `xml:"channelId"`
	// Views 来自 <media:group><media:community><media:statistics views="N"/>（RSS 自带播放量）
	MediaGroup struct {
		Community struct {
			Statistics struct {
				Views int `xml:"views,attr"`
			} `xml:"statistics"`
		} `xml:"community"`
	} `xml:"group"`
}

type YTLink struct {
	Href string `xml:"href,attr"`
}

type YTID struct {
	Value string `xml:",chardata"`
}

// ─── Monitor ──────────────────────────────────────────────────────────────────

// Monitor 管理频道订阅监控
type Monitor struct {
	subDir    string
	videoDir  string
	obsDir    string
	subPath   string
	videoPath string
	mu        sync.Mutex
}

func NewMonitor(dataDir string) *Monitor {
	subDir := filepath.Join(dataDir, "subscriptions")
	videoDir := filepath.Join(dataDir, "monitored_videos")
	obsDir := filepath.Join(dataDir, "observations")
	os.MkdirAll(subDir, 0755)
	os.MkdirAll(videoDir, 0755)
	os.MkdirAll(obsDir, 0755)

	return &Monitor{
		subDir:    subDir,
		videoDir:  videoDir,
		obsDir:    obsDir,
		subPath:   filepath.Join(subDir, "subscriptions.json"),
		videoPath: filepath.Join(videoDir, "videos.json"),
	}
}

// ─── Subscriptions ────────────────────────────────────────────────────────────

func (m *Monitor) loadSubscriptions() []Subscription {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, err := os.ReadFile(m.subPath)
	if err != nil {
		return nil
	}
	var subs []Subscription
	if err := json.Unmarshal(data, &subs); err != nil {
		log.Printf("warning: loadSubscriptions unmarshal failed: %v", err)
		return nil
	}
	return subs
}

func (m *Monitor) saveSubscriptions(subs []Subscription) {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, err := json.MarshalIndent(subs, "", "  ")
	if err != nil {
		log.Printf("warning: saveSubscriptions marshal failed: %v", err)
		return
	}
	if err := os.WriteFile(m.subPath, data, 0644); err != nil {
		log.Printf("warning: saveSubscriptions write failed: %v", err)
	}
}

// AddSubscription 添加一个频道订阅
func (m *Monitor) AddSubscription(channelID, channelTitle string) (*Subscription, error) {
	subs := m.loadSubscriptions()
	for _, s := range subs {
		if s.ChannelID == channelID && s.Status == "active" {
			return &s, fmt.Errorf("频道 %s (%s) 已在监控列表中", channelID, s.ChannelTitle)
		}
	}

	// If it exists as inactive, reactivate it
	for i, s := range subs {
		if s.ChannelID == channelID && s.Status == "inactive" {
			subs[i].Status = "active"
			subs[i].Type = DetectType(channelID)
			subs[i].AddedAt = time.Now().Format(time.RFC3339)
			m.saveSubscriptions(subs)
			return &subs[i], nil
		}
	}

	sub := Subscription{
		ChannelID:    channelID,
		ChannelTitle: channelTitle,
		Type:         DetectType(channelID),
		AddedAt:      time.Now().Format(time.RFC3339),
		Status:       "active",
	}
	subs = append(subs, sub)
	m.saveSubscriptions(subs)
	return &sub, nil
}

// RemoveSubscription 移除一个频道订阅
func (m *Monitor) RemoveSubscription(channelID string) error {
	subs := m.loadSubscriptions()
	found := false
	for i, s := range subs {
		if s.ChannelID == channelID {
			subs = append(subs[:i], subs[i+1:]...)
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("频道 %s 不在监控列表中", channelID)
	}
	m.saveSubscriptions(subs)
	return nil
}

// ListSubscriptions 列出所有订阅
func (m *Monitor) ListSubscriptions() []Subscription {
	return m.loadSubscriptions()
}

// GetActiveSubscriptions 获取活跃订阅
func (m *Monitor) GetActiveSubscriptions() []Subscription {
	all := m.loadSubscriptions()
	var active []Subscription
	for _, s := range all {
		if s.Status == "active" {
			active = append(active, s)
		}
	}
	return active
}

// ─── Video Discovery ──────────────────────────────────────────────────────────

func (m *Monitor) loadVideos() []DiscoveredVideo {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, err := os.ReadFile(m.videoPath)
	if err != nil {
		return nil
	}
	var videos []DiscoveredVideo
	if err := json.Unmarshal(data, &videos); err != nil {
		log.Printf("warning: loadVideos unmarshal failed: %v", err)
		return nil
	}
	return videos
}

func (m *Monitor) saveVideos(videos []DiscoveredVideo) {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, err := json.MarshalIndent(videos, "", "  ")
	if err != nil {
		log.Printf("warning: saveVideos marshal failed: %v", err)
		return
	}
	if err := os.WriteFile(m.videoPath, data, 0644); err != nil {
		log.Printf("warning: saveVideos write failed: %v", err)
	}
}

// DiscoveredVideos 返回所有已发现的视频
func (m *Monitor) DiscoveredVideos() []DiscoveredVideo {
	return m.loadVideos()
}

// PendingVideos 返回待处理的视频
func (m *Monitor) PendingVideos() []DiscoveredVideo {
	all := m.loadVideos()
	var pending []DiscoveredVideo
	for _, v := range all {
		if v.Status == "new" {
			pending = append(pending, v)
		}
	}
	return pending
}

// ─── RSS Feed Sync ────────────────────────────────────────────────────────────

// FetchTitle 通过 YouTube RSS feed 获取频道/播放列表名称
// 失败时返回空字符串，调用方应回退到 ID 本身
func FetchTitle(id string) string {
	sub := Subscription{ChannelID: id, Type: DetectType(id)}
	feedURL := sub.feedURL()

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(feedURL)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}

	var feed YouTubeFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return ""
	}

	return strings.TrimSpace(feed.Title)
}

// SyncAll 同步所有活跃频道的 RSS feed
// lookbackDays: 仅处理 lookbackDays 天内发布的视频（0 = 不限制）
// callback: 可选回调函数，每发现一个新视频时调用（可用于自动提交）
func (m *Monitor) SyncAll(lookbackDays int, callback func(video *DiscoveredVideo) error) (int, error) {
	subs := m.GetActiveSubscriptions()
	if len(subs) == 0 {
		return 0, fmt.Errorf("没有活跃的频道订阅，请先用 channel add 添加")
	}

	totalNew := 0
	for _, sub := range subs {
		newCount, err := m.syncChannel(sub, lookbackDays, callback)
		if err != nil {
			fmt.Fprintf(os.Stderr, "⚠ 同步频道 %s 失败: %v\n", sub.ChannelID, err)
			continue
		}
		totalNew += newCount

		// Update last sync time
		allSubs := m.loadSubscriptions()
		for i, s := range allSubs {
			if s.ChannelID == sub.ChannelID {
				allSubs[i].LastSyncAt = time.Now().Format(time.RFC3339)
				break
			}
		}
		m.saveSubscriptions(allSubs)
	}

	return totalNew, nil
}

// SyncSubscription 同步单个订阅，返回时间范围内发现的新视频数
// 与 SyncAll 类似，但只处理指定订阅（例如刚添加的频道）
func (m *Monitor) SyncSubscription(sub Subscription, lookbackDays int, callback func(video *DiscoveredVideo) error) (int, error) {
	return m.syncChannel(sub, lookbackDays, callback)
}

func (m *Monitor) syncChannel(sub Subscription, lookbackDays int, callback func(video *DiscoveredVideo) error) (int, error) {
	feedURL := sub.feedURL()

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(feedURL)
	if err != nil {
		return 0, fmt.Errorf("请求 RSS feed 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("RSS feed 返回状态码 %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("读取 RSS feed 失败: %w", err)
	}

	var feed YouTubeFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return 0, fmt.Errorf("解析 RSS XML 失败: %w", err)
	}

	// 观测快照：记录本次 RSS 里所有视频的播放量（point-in-time，供 velocity 计算）
	var obs []Observation
	for _, e := range feed.Entries {
		if id := m.extractVideoID(e); id != "" {
			obs = append(obs, Observation{
				VideoID:    id,
				ChannelID:  sub.ChannelID,
				Title:      e.Title,
				Views:      e.MediaGroup.Community.Statistics.Views,
				ObservedAt: time.Now(),
			})
		}
	}
	if err := m.RecordObservation(obs); err != nil {
		fmt.Fprintf(os.Stderr, "⚠ 记录观测快照失败: %v\n", err)
	}

	cutoff := time.Now()
	if lookbackDays > 0 {
		cutoff = cutoff.Add(-time.Duration(lookbackDays) * 24 * time.Hour)
	}

	existing := m.loadVideos()
	existingMap := make(map[string]bool)
	for _, v := range existing {
		existingMap[v.VideoID] = true
	}

	newCount := 0

	for _, entry := range feed.Entries {
		videoID := m.extractVideoID(entry)
		if videoID == "" {
			continue
		}

		// Check if already discovered
		if existingMap[videoID] {
			continue
		}

		// Parse published time
		publishedAt, err := time.Parse(time.RFC3339, entry.Published)
		if err != nil {
			continue
		}

		// Check lookback window
		if lookbackDays > 0 && publishedAt.Before(cutoff) {
			continue
		}

		video := DiscoveredVideo{
			VideoID:      videoID,
			ChannelID:    sub.ChannelID,
			Title:        entry.Title,
			URL:          fmt.Sprintf("https://www.youtube.com/watch?v=%s", videoID),
			PublishedAt:  publishedAt.Format(time.RFC3339),
			DiscoveredAt: time.Now().Format(time.RFC3339),
			Views:        entry.MediaGroup.Community.Statistics.Views,
			Status:       "new",
		}

		existing = append(existing, video)
		existingMap[videoID] = true
		newCount++

		// Callback for auto-submit
		if callback != nil {
			if err := callback(&video); err != nil {
				fmt.Fprintf(os.Stderr, "⚠ 自动处理视频 %s 失败: %v\n", videoID, err)
			}
		}

		fmt.Printf("📺 [新视频] %s - %s\n", sub.ChannelTitle, entry.Title)
	}

	if newCount > 0 {
		m.saveVideos(existing)
	}

	return newCount, nil
}

func (m *Monitor) extractVideoID(entry YouTubeEntry) string {
	// Try <yt:videoId> first
	if entry.VideoID.Value != "" {
		return entry.VideoID.Value
	}
	// Fallback: extract from ID tag (yt:video:xxxxxx)
	if entry.ID != "" {
		parts := strings.Split(entry.ID, ":")
		if len(parts) > 2 {
			return parts[len(parts)-1]
		}
	}
	return ""
}

// MarkSubmitted 将视频标记为已提交
func (m *Monitor) MarkSubmitted(videoID string) error {
	videos := m.loadVideos()
	for i, v := range videos {
		if v.VideoID == videoID {
			videos[i].Status = "submitted"
			m.saveVideos(videos)
			return nil
		}
	}
	return fmt.Errorf("视频 %s 未找到", videoID)
}

// MarkSkipped 将视频标记为已跳过
func (m *Monitor) MarkSkipped(videoID string) {
	videos := m.loadVideos()
	for i, v := range videos {
		if v.VideoID == videoID {
			videos[i].Status = "skipped"
			m.saveVideos(videos)
			return
		}
	}
}

// ─── Watch (Continuous Sync) ──────────────────────────────────────────────────

// Watch 持续监控所有频道，每隔 interval 同步一次
// 返回一个 stop 函数用于停止监控
func (m *Monitor) Watch(interval time.Duration, lookbackDays int, callback func(video *DiscoveredVideo) error, statusCb func(msg string)) func() {
	stopChan := make(chan struct{})

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		// Run first sync immediately
		if statusCb != nil {
			statusCb("🔄 首次同步频道...")
		}
		newCount, err := m.SyncAll(lookbackDays, callback)
		if err != nil && statusCb != nil {
			statusCb(fmt.Sprintf("⚠ 首次同步: %v", err))
		} else if statusCb != nil {
			statusCb(fmt.Sprintf("✅ 首次同步完成，发现 %d 个新视频", newCount))
		}

		for {
			select {
			case <-ticker.C:
				if statusCb != nil {
					statusCb(fmt.Sprintf("🔄 定时同步 (%d 个活跃频道)...", len(m.GetActiveSubscriptions())))
				}
				newCount, err := m.SyncAll(lookbackDays, callback)
				if err != nil && statusCb != nil {
					statusCb(fmt.Sprintf("⚠ 同步: %v", err))
				} else if statusCb != nil {
					statusCb(fmt.Sprintf("✅ 同步完成，发现 %d 个新视频", newCount))
				}
			case <-stopChan:
				return
			}
		}
	}()

	return func() {
		close(stopChan)
	}
}

// GetStats 返回监控统计信息
func (m *Monitor) GetStats() map[string]int {
	subs := m.loadSubscriptions()
	videos := m.loadVideos()

	activeSubs := 0
	for _, s := range subs {
		if s.Status == "active" {
			activeSubs++
		}
	}

	newVideos := 0
	submittedVideos := 0
	for _, v := range videos {
		switch v.Status {
		case "new":
			newVideos++
		case "submitted":
			submittedVideos++
		}
	}

	return map[string]int{
		"total_subscriptions":   len(subs),
		"active_subscriptions":  activeSubs,
		"total_videos":          len(videos),
		"pending_videos":        newVideos,
		"submitted_videos":      submittedVideos,
	}
}

// ─── Task Creation Helper ─────────────────────────────────────────────────────

// CreateTaskFromVideo 从发现的视频创建一个提交任务
func CreateTaskFromVideo(video *DiscoveredVideo, taskStore *storage.TaskStore) *storage.Task {
	task := taskStore.Create(video.URL)
	task.Title = video.Title
	// Update task with channel info in result
	task.Result["channel_id"] = video.ChannelID
	task.Result["discovered_at"] = video.DiscoveredAt
	return task
}
