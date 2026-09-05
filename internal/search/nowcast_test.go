package search

import (
	"math"
	"testing"
	"time"
)

// ─── 年龄曲线 ──────────────────────────────────────────────────

func TestAgeCurveFraction48h(t *testing.T) {
	cases := []struct {
		age  float64
		want float64
	}{
		{0, 0.03},
		{4, 0.30},        // 4h → 0.6*4/8 = 0.3
		{8, 0.60},        // 8h 边界 → 0.6
		{24, 0.74},       // 24h → 0.6+0.35*(16/40) = 0.74
		{48, 0.95},       // 48h → 0.6+0.35 = 0.95
		{100, 0.95},      // 平台期
	}
	for _, c := range cases {
		got := ageCurveFraction48h(c.age)
		if math.Abs(got-c.want) > 0.001 {
			t.Errorf("ageCurveFraction48h(%.1f) = %.3f, want %.3f", c.age, got, c.want)
		}
	}
}

func TestAgeCurveExpectedSlope(t *testing.T) {
	if got := ageCurveExpectedSlope(4); got != 0.075 {
		t.Errorf("slope(4h) = %v, want 0.075", got)
	}
	if got := ageCurveExpectedSlope(24); got != 0.00875 {
		t.Errorf("slope(24h) = %v, want 0.00875", got)
	}
	if got := ageCurveExpectedSlope(100); got != 0.001 {
		t.Errorf("slope(100h) = %v, want 0.001", got)
	}
}

// ─── 归一化 ────────────────────────────────────────────────────

func TestNormRatio(t *testing.T) {
	if got := normRatio(0, 6); got != 0 {
		t.Errorf("normRatio(0) = %v, want 0", got)
	}
	// value = cap → 1.0
	if got := normRatio(6, 6); math.Abs(got-1.0) > 0.001 {
		t.Errorf("normRatio(6,6) = %v, want 1.0", got)
	}
	// 单调递增
	if normRatio(2, 6) <= normRatio(1, 6) {
		t.Error("normRatio 应单调递增")
	}
}

func TestNormReach(t *testing.T) {
	// reach=0 → 0
	if got := normReach(0); got != 0 {
		t.Errorf("normReach(0) = %v, want 0", got)
	}
	// reach=0.1 → sqrt(1) = 1.0（播放 = 粉丝数的 10% 即满分）
	if got := normReach(0.1); math.Abs(got-1.0) > 0.001 {
		t.Errorf("normReach(0.1) = %v, want 1.0", got)
	}
	// 封顶
	if got := normReach(5); got > 1.0 {
		t.Errorf("normReach(5) = %v, want capped at 1.0", got)
	}
}

// ─── 置信度 ────────────────────────────────────────────────────

func TestConfidenceMultiplier(t *testing.T) {
	now := time.Now()
	v := Video{DurationSec: 600, PublishTime: "3 hours ago"}

	// 完整数据 + 新鲜基线 → 高分
	c := confidenceMultiplier(v, now.Add(-1*time.Hour), now, true)
	if c < 1.0 {
		t.Errorf("新鲜基线置信度 = %v, want >= 1.0", c)
	}

	// 过期基线（>7天）→ 降权
	c2 := confidenceMultiplier(v, now.Add(-10*24*time.Hour), now, true)
	if c2 >= c {
		t.Errorf("过期基线置信度 = %v, want < 新鲜基线 %v", c2, c)
	}

	// 无基线 → 保守
	c3 := confidenceMultiplier(v, time.Time{}, now, false)
	if c3 >= 1.0 {
		t.Errorf("无基线置信度 = %v, want < 1.0", c3)
	}

	// 区间约束
	for _, c4 := range []float64{c, c2, c3} {
		if c4 < 0.75 || c4 > 1.05 {
			t.Errorf("置信度 %v 超出 [0.75, 1.05]", c4)
		}
	}
}

// ─── Early Breakout ────────────────────────────────────────────

func TestEarlyBreakoutBoost(t *testing.T) {
	// 新视频 + 双强 → 有加成
	got := earlyBreakoutBoost(10, 2.0, 2.0)
	if got <= 0 || got > 0.12 {
		t.Errorf("breakout(10h, 2x, 2x) = %v, want (0, 0.12]", got)
	}
	// 老视频 → 0
	if got := earlyBreakoutBoost(48, 2.0, 2.0); got != 0 {
		t.Errorf("breakout(48h) = %v, want 0", got)
	}
	// nowcast 弱 → 0
	if got := earlyBreakoutBoost(10, 1.0, 2.0); got != 0 {
		t.Errorf("breakout(weak nowcast) = %v, want 0", got)
	}
	// velocity 弱 → 0
	if got := earlyBreakoutBoost(10, 2.0, 1.0); got != 0 {
		t.Errorf("breakout(weak velocity) = %v, want 0", got)
	}
}

// ─── 完整评分 ──────────────────────────────────────────────────

func TestScoreVideosNowcastFull(t *testing.T) {
	now := time.Now()
	videos := []Video{
		{ID: "hot", Title: "爆款", ViewCount: 100000, DurationSec: 900,
			PublishTime: "2 hours ago", ChannelID: "ch1"},
		{ID: "normal", Title: "常态", ViewCount: 10000, DurationSec: 900,
			PublishTime: "2 hours ago", ChannelID: "ch1"},
		{ID: "old", Title: "老视频", ViewCount: 20000, DurationSec: 900,
			PublishTime: "30 days ago", ChannelID: "ch1"},
	}
	baselines := map[string]NowcastBaseline{
		"ch1": {Baseline: 10000, Subscribers: 500000, UpdatedAt: now.Add(-time.Hour), HasBaseline: true},
	}

	scored := ScoreVideosNowcastFull(videos, baselines)
	if len(scored) != 3 {
		t.Fatalf("len(scored) = %d, want 3", len(scored))
	}

	// 排序：爆款 > 常态 > 老视频
	if scored[0].ID != "hot" {
		t.Errorf("top1 = %s, want hot（爆款应第一）", scored[0].ID)
	}
	if scored[2].ID != "old" {
		t.Errorf("top3 = %s, want old（老视频应最后）", scored[2].ID)
	}

	// 爆款应有 breakout 加成（新 + nowcast 强 + velocity 强）
	if scored[0].BreakoutBoost <= 0 {
		t.Error("爆款应有 Early Breakout 加成")
	}

	// 置信度字段应填充
	if scored[0].Confidence < 0.75 || scored[0].Confidence > 1.05 {
		t.Errorf("confidence = %v 超出范围", scored[0].Confidence)
	}
	// ExpectedViews 应 > 0
	if scored[0].ExpectedViews <= 0 {
		t.Error("ExpectedViews 应 > 0")
	}
	// 老视频预测 48h = 当前播放
	if scored[2].Predicted48h != float64(scored[2].ViewCount) {
		t.Errorf("old Predicted48h = %v, want %d", scored[2].Predicted48h, scored[2].ViewCount)
	}
}

func TestScoreVideosNowcastCompat(t *testing.T) {
	// 旧签名（map[string]float64）应仍可用
	videos := []Video{
		{ID: "a", ViewCount: 5000, DurationSec: 900, PublishTime: "1 hour ago", ChannelID: "c1"},
	}
	scored := ScoreVideosNowcast(videos, map[string]float64{"c1": 1000})
	if len(scored) != 1 {
		t.Fatalf("len(scored) = %d, want 1", len(scored))
	}
	if scored[0].NowcastScore <= 0 {
		t.Error("NowcastScore 应 > 0")
	}
}
