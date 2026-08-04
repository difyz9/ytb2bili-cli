package search

import (
	"testing"
	"time"
)

func TestNowcastComponents(t *testing.T) {
	now := time.Now()
	t.Run("views equals baseline → nowcast 0.5", func(t *testing.T) {
		v := Video{ViewCount: 50000, PublishTime: "3 hours ago"}
		nowcast, _ := nowcastComponents(v, 50000, 1.0, now)
		if nowcast < 0.49 || nowcast > 0.51 {
			t.Fatalf("nowcast = %f, want ~0.5", nowcast)
		}
	})
	t.Run("views 2x baseline → nowcast 1.0", func(t *testing.T) {
		v := Video{ViewCount: 200000, PublishTime: "3 hours ago"}
		nowcast, _ := nowcastComponents(v, 100000, 1.0, now)
		if nowcast != 1.0 {
			t.Fatalf("nowcast = %f, want 1.0", nowcast)
		}
	})
	t.Run("no baseline defaults to 1", func(t *testing.T) {
		v := Video{ViewCount: 100, PublishTime: "3 hours ago"}
		nowcast, _ := nowcastComponents(v, 0, 1.0, now)
		if nowcast < 0 {
			t.Fatalf("negative nowcast: %f", nowcast)
		}
	})
}

func TestScoreVideosNowcast(t *testing.T) {
	// 同播放量下：低基线频道的视频（超常）应排在高基线频道视频前面
	videos := []Video{
		{ID: "small", ChannelID: "smallCh", ViewCount: 50000, PublishTime: "3 hours ago", DurationSec: 600},
		{ID: "big", ChannelID: "bigCh", ViewCount: 50000, PublishTime: "3 hours ago", DurationSec: 600},
	}
	baselines := map[string]float64{
		"smallCh": 10000, // 5 倍基线 → 超常
		"bigCh":   100000, // 0.5 倍基线 → 低于常态
	}
	scored := ScoreVideosNowcast(videos, baselines)
	if len(scored) != 2 {
		t.Fatalf("got %d", len(scored))
	}
	if scored[0].ID != "small" {
		t.Fatalf("expected small-channel video to rank first (outperforms baseline), got %s", scored[0].ID)
	}
	if scored[0].NowcastScore <= scored[1].NowcastScore {
		t.Fatalf("small nowcast=%f should exceed big nowcast=%f", scored[0].NowcastScore, scored[1].NowcastScore)
	}
}

func TestScoreVideosNowcastEmpty(t *testing.T) {
	if got := ScoreVideosNowcast(nil, nil); got != nil {
		t.Fatalf("got %v, want nil", got)
	}
}
