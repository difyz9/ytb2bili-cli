package channel

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ChannelStats 单个频道的基线数据与评分（来自 RSS，无需 OAuth）。
type ChannelStats struct {
	ChannelID    string    `json:"channel_id"`
	ChannelTitle string    `json:"channel_title"`
	Baseline     float64   `json:"baseline"`     // 窗口内平均播放
	Baseline48h  float64   `json:"baseline_48h,omitempty"` // 48h 预期播放（trimmed mean，抗异常）
	Subscribers  int64     `json:"subscribers,omitempty"`  // 频道粉丝数
	UpdatedAt    time.Time `json:"updated_at,omitempty"`   // 基线采集时间（置信度用）
	Activity     int       `json:"activity"`     // 窗口内视频数
	LatestViews  int       `json:"latest_views"` // 窗口内最新视频播放
	Samples      int       `json:"samples"`      // 有效样本数
	Fit          float64   `json:"fit"`          // 内容契合 0-1（标题命中关键词比例）
	Score        float64   `json:"score"`        // 综合分 0-100
	views        []float64 `json:"-"`            // 窗口内播放样本（算标准差用，不序列化）
}

// ChannelBaseline 抓取单频道 RSS，计算窗口内（windowDays 天）的播放基线与评分。
// 数据来自 RSS 自带的 <media:statistics views="N"/>，无需 OAuth。
// keywords：内容契合度匹配用；为空则 fit 按中性 0.5 处理。
func ChannelBaseline(ctx context.Context, sub Subscription, windowDays int, keywords []string) (*ChannelStats, error) {
	feedURL := sub.feedURL()
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(feedURL)
	if err != nil {
		return nil, fmt.Errorf("请求 RSS 失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("RSS 返回状态码 %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取 RSS 失败: %w", err)
	}
	var feed YouTubeFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("解析 RSS 失败: %w", err)
	}

	cutoff := time.Now().Add(-time.Duration(windowDays) * 24 * time.Hour)
	var windowViews []float64 // 窗口内播放
	var allViews []float64    // 全部播放（供"频道自身历史"中位数参照）
	var windowTitles []string
	activity := 0
	latest := 0
	for _, e := range feed.Entries {
		v := float64(e.MediaGroup.Community.Statistics.Views)
		allViews = append(allViews, v)
		pub, perr := time.Parse(time.RFC3339, e.Published)
		if perr != nil || pub.Before(cutoff) {
			continue
		}
		if activity == 0 {
			latest = int(v) // 第一条窗口内（按 RSS 顺序）即最新
		}
		windowViews = append(windowViews, v)
		windowTitles = append(windowTitles, e.Title)
		activity++
	}

	st := &ChannelStats{
		ChannelID:    sub.ChannelID,
		ChannelTitle: sub.ChannelTitle,
		Activity:     activity,
		LatestViews:  latest,
		Samples:      len(windowViews),
		Fit:          fitRatio(windowTitles, keywords),
		views:        windowViews,
	}
	if len(windowViews) > 0 {
		st.Baseline = mean(windowViews)
	}
	st.Score = scoreChannel(st, allViews)
	return st, nil
}

// scoreChannel 计算频道质量分（0-100）。
// ytsubs 哲学：用"频道基线"而非订阅数判断质量；同时保留真实触达（基线规模）。
//   - 活跃度 25%：窗口内更新数（封顶 10，RSS 样本有限）
//   - 基线触达 30%：log 缩放的窗口平均播放（搬运价值：播放规模越大越好）
//   - 基线健康 20%：窗口平均播放 / 频道历史中位数（最近掉线 → 频道衰退）
//   - 播放稳定 15%：1 - 变异系数（靠一条爆款撑的伪高质量降权）
//   - 内容契合 10%：标题命中关键词比例
func scoreChannel(c *ChannelStats, allViews []float64) float64 {
	activity := math.Min(1, float64(c.Activity)/10) * 25

	reach := 0.0
	if c.Baseline > 0 {
		reach = math.Min(1, math.Log10(c.Baseline+1)/7) * 30 // 10→14, 1k→43, 100k→71, 10M→100
	}

	medianVal := median(allViews)
	health := 1.0
	if c.Baseline > 0 && medianVal > 0 {
		health = math.Min(1, c.Baseline/medianVal)
	}

	stability := 0.0
	if c.Samples >= 2 && c.Baseline > 0 {
		// 变异系数 CV = stddev/mean，越低越稳
		cv := stddev(c.views) / c.Baseline
		stability = math.Max(0, 1-cv)
	} else if c.Samples == 1 {
		stability = 0.5
	}

	score := activity + reach + health*20 + stability*15 + c.Fit*10

	// 置信度修正：样本太少降权（RSS 解析不稳时）
	if c.Samples < 3 {
		score *= 0.8
	} else if c.Samples < 6 {
		score *= 0.9
	}
	return math.Round(score*10) / 10
}

// stddev 计算样本标准差。
func stddev(xs []float64) float64 {
	if len(xs) < 2 {
		return 0
	}
	m := mean(xs)
	sum := 0.0
	for _, x := range xs {
		d := x - m
		sum += d * d
	}
	return math.Sqrt(sum / float64(len(xs)-1))
}

func mean(xs []float64) float64 {
	sum := 0.0
	for _, x := range xs {
		sum += x
	}
	return sum / float64(len(xs))
}

func median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	sorted := append([]float64(nil), xs...)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j] < sorted[j-1]; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}
	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}

// TrimmedMean 去尾平均：排序后去掉最高/最低各 trim 个，再取平均。
// 对标 ytsubs：最近 30 视频去掉最高/最低 3 个（抗爆款污染）。
func TrimmedMean(xs []float64, trim int) float64 {
	if len(xs) == 0 {
		return 0
	}
	if trim < 0 {
		trim = 0
	}
	if len(xs) <= trim*2 {
		return mean(xs) // 样本太少，退化为普通平均
	}
	sorted := append([]float64(nil), xs...)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j] < sorted[j-1]; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}
	return mean(sorted[trim : len(sorted)-trim])
}

// VideoStat 抓取到的单视频播放数据（yt-dlp 输出）。
type VideoStat struct {
	VideoID   string
	ViewCount int64
}

// FetchChannelVideos 用 yt-dlp 抓取频道最近 N 个视频的播放量（含频道粉丝数）。
// 视频列表用 flat-playlist（快），粉丝数单独用一次非 flat 查询（flat 下粉丝数为 NA）。
func FetchChannelVideos(ctx context.Context, channelURL string, limit int) ([]VideoStat, int64, error) {
	if limit <= 0 {
		limit = 30
	}
	args := []string{
		"--flat-playlist",
		"--playlist-items", fmt.Sprintf("1:%d", limit),
		"--print", "%(view_count)s",
		"--no-warnings",
		channelURL,
	}
	cmd := exec.CommandContext(ctx, "yt-dlp", args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &bytes.Buffer{}
	if err := cmd.Run(); err != nil {
		return nil, 0, fmt.Errorf("yt-dlp 抓取频道视频失败: %w", err)
	}

	var stats []VideoStat
	for _, line := range strings.Split(out.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		views, err := strconv.ParseInt(line, 10, 64)
		if err != nil {
			continue // 跳过无播放量的（直播/会员专属）
		}
		stats = append(stats, VideoStat{ViewCount: views})
	}
	if len(stats) == 0 {
		return nil, 0, fmt.Errorf("yt-dlp 未返回有效视频数据")
	}

	// 单独查询频道粉丝数（非 flat 模式才有 channel_follower_count）
	// 用独立超时（WARP 网络下非 flat 查询可能较慢），避免与视频列表共享 ctx 超时
	subCtx, subCancel := context.WithTimeout(context.Background(), 180*time.Second)
	subscribers, err := fetchChannelSubscribers(subCtx, channelURL)
	subCancel()
	if err != nil {
		// 粉丝数拿不到不致命（reach 分量退化为绝对播放量）
		fmt.Fprintf(os.Stderr, "  ⚠ 粉丝数获取失败: %v\n", err)
		subscribers = 0
	}
	return stats, subscribers, nil
}

// fetchChannelSubscribers 查询频道粉丝数（channel_follower_count）。
func fetchChannelSubscribers(ctx context.Context, channelURL string) (int64, error) {
	cmd := exec.CommandContext(ctx, "yt-dlp",
		"--skip-download", "--no-warnings",
		"--print", "%(channel_follower_count)s",
		channelURL)
	var out bytes.Buffer
	var errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("yt-dlp 粉丝数失败: %w (stderr: %s)", err, strings.TrimSpace(errBuf.String()))
	}
	line := strings.TrimSpace(out.String())
	if line == "" || line == "NA" {
		return 0, fmt.Errorf("无粉丝数")
	}
	subs, err := strconv.ParseInt(line, 10, 64)
	if err != nil || subs <= 0 {
		return 0, fmt.Errorf("粉丝数解析失败: %q", line)
	}
	return subs, nil
}

// fitRatio 计算 titles 中命中任一 keyword 的比例（0-1）。keyword 为空返回 0.5（中性）。
func fitRatio(titles []string, keywords []string) float64 {
	if len(titles) == 0 {
		return 0
	}
	if len(keywords) == 0 {
		return 0.5
	}
	hits := 0
	for _, t := range titles {
		lower := strings.ToLower(t)
		for _, kw := range keywords {
			if kw = strings.TrimSpace(kw); kw != "" && strings.Contains(lower, strings.ToLower(kw)) {
				hits++
				break
			}
		}
	}
	return float64(hits) / float64(len(titles))
}
