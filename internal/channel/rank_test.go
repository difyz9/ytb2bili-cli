package channel

import (
	"encoding/xml"
	"testing"
)

// 用真实 RSS 片段验证 media:statistics views 解析
func TestRSSViewsParsing(t *testing.T) {
	xmlStr := `<?xml version="1.0"?>
<feed xmlns:media="http://search.yahoo.com/mrss/" xmlns="http://www.w3.org/2005/Atom">
 <entry>
  <yt:videoId>abc123</yt:videoId>
  <title>Hello world</title>
  <published>2026-08-01T00:00:00+00:00</published>
  <media:group>
   <media:community>
    <media:statistics views="659575"/>
   </media:community>
  </media:group>
 </entry>
</feed>`
	var feed YouTubeFeed
	if err := xml.Unmarshal([]byte(xmlStr), &feed); err != nil {
		t.Fatal(err)
	}
	if len(feed.Entries) != 1 {
		t.Fatalf("got %d entries", len(feed.Entries))
	}
	if feed.Entries[0].MediaGroup.Community.Statistics.Views != 659575 {
		t.Fatalf("views = %d, want 659575", feed.Entries[0].MediaGroup.Community.Statistics.Views)
	}
}

func TestScoreChannel(t *testing.T) {
	c := &ChannelStats{
		Activity: 8,
		Baseline: 50000,
		Samples:  8,
		Fit:      0.5,
		views:    []float64{48000, 52000, 45000, 60000, 51000, 47000, 49000, 55000},
	}
	// 全部样本中位数略低于窗口均值 → 基线健康
	all := []float64{48000, 52000, 45000, 60000, 51000, 47000, 49000, 55000, 42000, 38000}
	score := scoreChannel(c, all)
	if score <= 0 || score > 100 {
		t.Fatalf("score out of range: %f", score)
	}
	if score < 40 {
		t.Fatalf("active consistent channel should score decently, got %f", score)
	}
}

func TestFitRatio(t *testing.T) {
	titles := []string{"AI coding with Flutter", "Go tutorial for beginners", "cooking pasta"}
	if got := fitRatio(titles, []string{"ai", "flutter", "go"}); got != 2.0/3.0 {
		t.Fatalf("fitRatio = %f, want %f", got, 2.0/3.0)
	}
	if got := fitRatio(titles, nil); got != 0.5 {
		t.Fatalf("fitRatio(nil kw) = %f, want 0.5", got)
	}
}

func TestMedian(t *testing.T) {
	if got := median([]float64{3, 1, 2}); got != 2 {
		t.Fatalf("median(odd) = %f", got)
	}
	if got := median([]float64{4, 1, 3, 2}); got != 2.5 {
		t.Fatalf("median(even) = %f", got)
	}
	if got := median(nil); got != 0 {
		t.Fatalf("median(nil) = %f", got)
	}
}
