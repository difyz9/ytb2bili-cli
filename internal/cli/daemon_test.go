package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zolagz/ytb2bili-go/internal/config"
	"github.com/zolagz/ytb2bili-go/internal/queue"
	"github.com/zolagz/ytb2bili-go/internal/workflow"
)

// ─── rotateKeywords ────────────────────────────────────────────────────

func TestRotateKeywords(t *testing.T) {
	t.Run("rotates first to last", func(t *testing.T) {
		in := []string{"a", "b", "c"}
		got := rotateKeywords(in)
		want := []string{"b", "c", "a"}
		if len(got) != len(want) {
			t.Fatalf("got %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("got %v, want %v", got, want)
			}
		}
		// 不修改原切片
		if in[0] != "a" {
			t.Fatalf("input mutated: %v", in)
		}
	})
	t.Run("single element no-op", func(t *testing.T) {
		if got := rotateKeywords([]string{"x"}); len(got) != 1 || got[0] != "x" {
			t.Fatalf("got %v", got)
		}
	})
	t.Run("empty no-op", func(t *testing.T) {
		if got := rotateKeywords(nil); got != nil {
			t.Fatalf("got %v", got)
		}
	})
}

// ─── heartbeat 写入/读取 ────────────────────────────────────────────────

func TestWriteDaemonHeartbeat(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.DataDir = dir

	hb := daemonHeartbeat{PID: 12345, Batch: 7, Status: "running", StartedAt: time.Now().Format(time.RFC3339)}
	hb = hb.withQueue(map[string]int{"queued": 3, "claimed": 1, "completed": 10, "failed": 2})
	if err := writeDaemonHeartbeat(cfg, hb); err != nil {
		t.Fatal(err)
	}

	path := heartbeatPath(cfg)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got daemonHeartbeat
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Batch != 7 || got.PID != 12345 || got.Status != "running" {
		t.Fatalf("heartbeat mismatch: %+v", got)
	}
	if got.Queue["queued"] != 3 || got.Queue["failed"] != 2 {
		t.Fatalf("queue stats mismatch: %+v", got.Queue)
	}
}

func TestHeartbeatPath(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = "/tmp/xyz"
	got := heartbeatPath(cfg)
	want := filepath.Join("/tmp/xyz", "daemon", "heartbeat.json")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// ─── effectiveSearch：config 兜底 ──────────────────────────────────────

func TestEffectiveSearch(t *testing.T) {
	cfg := config.Default()
	cfg.Search = &config.SearchConfig{
		Keywords:    []string{"kw1", "kw2"},
		Scorer:      "fresh",
		UploadDate:  "this_week",
		MaxDuration: 25,
		MaxVideos:   3,
		MinViews:    100,
	}

	// flag 未指定 → 全部取 config
	opts := daemonOptions{}
	kws, scorer, upload, maxDur, maxVids, minViews := opts.effectiveSearch(cfg)
	if len(kws) != 2 || kws[0] != "kw1" {
		t.Fatalf("keywords: %v", kws)
	}
	if scorer != "fresh" || upload != "this_week" || maxDur != 25 || maxVids != 3 || minViews != 100 {
		t.Fatalf("config fallback failed: scorer=%s upload=%s maxDur=%d maxVids=%d minViews=%d",
			scorer, upload, maxDur, maxVids, minViews)
	}

	// flag 覆盖 config
	opts = daemonOptions{keywords: []string{"cli"}, scorer: "popular", uploadDate: "today", maxDuration: 50, maxVideos: 7, minViews: 500}
	kws, scorer, upload, maxDur, maxVids, minViews = opts.effectiveSearch(cfg)
	if len(kws) != 1 || kws[0] != "cli" {
		t.Fatalf("flag keywords not respected: %v", kws)
	}
	if scorer != "popular" || upload != "today" || maxDur != 50 || maxVids != 7 || minViews != 500 {
		t.Fatalf("flag override failed")
	}

	// 全空 → 兜底标准库关键词 + nowcast + 5
	cfg.Search = nil
	opts = daemonOptions{}
	kws, scorer, _, _, maxVids, _ = opts.effectiveSearch(cfg)
	if len(kws) == 0 {
		t.Fatal("expected fallback keywords")
	}
	if scorer != "nowcast" {
		t.Fatalf("scorer fallback: %s", scorer)
	}
	if maxVids != 5 {
		t.Fatalf("maxVideos fallback: %d", maxVids)
	}
}

// ─── workflow Executor 步骤超时 ─────────────────────────────────────────

func TestExecutorStepTimeout(t *testing.T) {
	r, err := workflow.NewRegistry(
		workflow.Step{Name: "fast", Run: func(ctx context.Context, s *workflow.State) error { return nil }},
		workflow.Step{Name: "slow", Run: func(ctx context.Context, s *workflow.State) error {
			<-ctx.Done() // 尊重 ctx：超时后被取消
			return ctx.Err()
		}},
	)
	if err != nil {
		t.Fatal(err)
	}

	e := workflow.Executor{
		Registry:    r,
		StepTimeout: map[string]time.Duration{"slow": 50 * time.Millisecond},
	}
	start := time.Now()
	err = e.Run(context.Background(), []string{"fast", "slow"}, workflow.NewState())
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("timeout did not kill step promptly: %v", elapsed)
	}
	if !strings.Contains(err.Error(), "超时") {
		t.Fatalf("expected timeout message, got: %v", err)
	}
}

func TestExecutorNoTimeoutByDefault(t *testing.T) {
	r, _ := workflow.NewRegistry(
		workflow.Step{Name: "ok", Run: func(ctx context.Context, s *workflow.State) error { return nil }},
	)
	e := workflow.Executor{Registry: r}
	if err := e.Run(context.Background(), []string{"ok"}, workflow.NewState()); err != nil {
		t.Fatal(err)
	}
}

// ─── queue RequeueClaimed（只回收 daemon 遗留认领） ───────────────────────

func TestRequeueClaimed(t *testing.T) {
	dir := t.TempDir()
	q := queue.New(dir)

	q.Add("v1", "https://www.youtube.com/watch?v=11111111111", "t1", "", "auto")
	// daemon（带前缀）认领后崩溃：任务保持 claimed
	item, err := q.Next(queue.DaemonWorkerID() + ":oldpid")
	if err != nil || item == nil {
		t.Fatalf("claim failed: %v %v", item, err)
	}

	n, err := q.RequeueClaimed()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 requeued, got %d", n)
	}

	// 现在应该能再次认领
	item2, err := q.Next("worker2") // 非 daemon 消费者
	if err != nil || item2 == nil {
		t.Fatalf("re-claim failed: %v %v", item2, err)
	}
	if item2.VideoID != "v1" {
		t.Fatalf("got %s", item2.VideoID)
	}
	// worker2 不是 daemon：活跃认领不应被 RequeueClaimed 抢走
	if n, _ := q.RequeueClaimed(); n != 0 {
		t.Fatalf("非 daemon 活跃认领不应被回收, got %d", n)
	}
}

// ─── keywordWindow / rotateKeywordsN（每批关键词窗口）───────────────────

func TestKeywordWindow(t *testing.T) {
	kws := []string{"a", "b", "c", "d"}
	if got := keywordWindow(kws, 2); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("窗口取前 2 个，得到 %v", got)
	}
	if got := keywordWindow(kws, 0); len(got) != 4 {
		t.Errorf("0 = 全部关键词，得到 %v", got)
	}
	if got := keywordWindow(kws, 99); len(got) != 4 {
		t.Errorf("窗口大于总数应返回全部，得到 %v", got)
	}
}

func TestRotateKeywordsN(t *testing.T) {
	kws := []string{"a", "b", "c", "d"}
	got := rotateKeywordsN(kws, 2)
	want := []string{"c", "d", "a", "b"}
	if len(got) != len(want) {
		t.Fatalf("长度不一致: %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("rotateKeywordsN(%v, 2) = %v，期望 %v", kws, got, want)
		}
	}
	if got := rotateKeywordsN(kws, 0); len(got) != 4 || got[0] != "a" {
		t.Errorf("n<=0 应原样返回，得到 %v", got)
	}
	if got := rotateKeywordsN(kws, 4); got[0] != "a" {
		t.Errorf("n>=len 应原样返回，得到 %v", got)
	}
	if got := rotateKeywordsN([]string{"x"}, 1); got[0] != "x" {
		t.Errorf("单元素应原样返回，得到 %v", got)
	}
	// 窗口轮换应覆盖全部关键词（4 个关键词、窗口 2 → 两批一轮回）
	seen := map[string]bool{}
	cur := kws
	for i := 0; i < 2; i++ {
		for _, k := range keywordWindow(cur, 2) {
			seen[k] = true
		}
		cur = rotateKeywordsN(cur, 2)
	}
	if len(seen) != 4 {
		t.Errorf("两批（每批 2 个）应覆盖全部 4 个关键词，实际覆盖 %d", len(seen))
	}
}
