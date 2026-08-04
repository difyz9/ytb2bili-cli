package channel

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
)

// ChannelStats 单个频道的基线数据与评分（来自 RSS，无需 OAuth）。
type ChannelStats struct {
	ChannelID    string    `json:"channel_id"`
	ChannelTitle string    `json:"channel_title"`
	Baseline     float64   `json:"baseline"`     // 窗口内平均播放
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
