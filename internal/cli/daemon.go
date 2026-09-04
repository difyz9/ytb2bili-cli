package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/zolagz/ytb2bili-go/internal/config"
	"github.com/zolagz/ytb2bili-go/internal/pipeline"
	"github.com/zolagz/ytb2bili-go/internal/queue"
	"github.com/zolagz/ytb2bili-go/internal/search"
)

// ─── ytb daemon：持续自动搬运守护进程（替代 batch_loop.sh）─────────────

func newDaemonCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "持续自动搬运守护进程（替代 batch_loop.sh）",
		Long: `持续自动搬运守护进程：搜索 → 评分 → 去重 → 入队 → 串行处理，循环执行。

取代 batch_loop.sh 的全部 bash 逻辑，参数统一从 config.yaml 读取：
  - 关键词/评分/上传日期/时长上限/每批数量 → config.yaml 的 search 段
  - 批间间隔/重试上限/步骤超时/心跳/告警   → config.yaml 的 daemon 段

特性：
  - 失败任务自动重试（默认 3 次）后停止，不再无限重试堵队列
  - 步骤级超时（下载 30min / TTS 60min，可配），超时自动 kill 重试
  - 每批心跳写入 data/daemon/heartbeat.json，供外部监控
  - 收到 SIGTERM/SIGINT 后等当前任务完成再退出（优雅重启）

示例：
  ytb daemon                              # 无限循环（systemd 使用）
  ytb daemon --once                       # 只跑一批后退出
  ytb daemon --max-batches 10 --interval 30
  ytb daemon --keywords "AI tutorial" --dry-run   # 只搜索评分不入队`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			opts, err := parseDaemonOptions(cmd)
			if err != nil {
				return err
			}
			return runDaemon(cfg, opts)
		},
	}

	cmd.Flags().Int("max-batches", 0, "最多批次数（0=无限，默认取配置 daemon.max_batches）")
	cmd.Flags().Int("interval", 0, "批间休息秒数（默认取配置 daemon.interval_sec，默认60）")
	cmd.Flags().Bool("once", false, "只跑一批后退出（等价 --max-batches 1）")
	cmd.Flags().Bool("dry-run", false, "只搜索评分，不入队/不提交")
	cmd.Flags().StringSlice("keywords", nil, "覆盖搜索关键词（默认取配置 search.keywords）")
	cmd.Flags().String("scorer", "", "覆盖评分策略: popular/fresh/balanced/nowcast")
	cmd.Flags().String("upload-date", "", "覆盖上传日期: last_hour/today/this_week/this_month/this_year")
	cmd.Flags().Int("max-duration", 0, "覆盖最大视频时长（分钟）")
	cmd.Flags().Int("max-videos", 0, "覆盖每批最多处理视频数")
	cmd.Flags().Int("min-views", 0, "覆盖最低播放量过滤")
	cmd.Flags().Int("consume-per-batch", 0, "覆盖每批消费排队任务上限")
	cmd.Flags().Bool("skip-translate", false, "跳过翻译")

	cmd.AddCommand(newDaemonStatusCmd(), newDaemonCheckCmd())
	return cmd
}

// daemonOptions daemon 运行参数（flag 覆盖配置）
type daemonOptions struct {
	maxBatches      int
	intervalSec     int
	dryRun          bool
	keywords        []string
	scorer          string
	uploadDate      string
	maxDuration     int
	maxVideos       int
	minViews        int
	consumePerBatch int
	skipTranslate   bool
	taskTimeoutSec  int // 单任务整体超时（0=不限）
}

func parseDaemonOptions(cmd *cobra.Command) (daemonOptions, error) {
	var opts daemonOptions
	var err error
	if opts.maxBatches, err = cmd.Flags().GetInt("max-batches"); err != nil {
		return opts, err
	}
	if opts.intervalSec, err = cmd.Flags().GetInt("interval"); err != nil {
		return opts, err
	}
	if opts.dryRun, err = cmd.Flags().GetBool("dry-run"); err != nil {
		return opts, err
	}
	if opts.keywords, err = cmd.Flags().GetStringSlice("keywords"); err != nil {
		return opts, err
	}
	if opts.scorer, err = cmd.Flags().GetString("scorer"); err != nil {
		return opts, err
	}
	if opts.uploadDate, err = cmd.Flags().GetString("upload-date"); err != nil {
		return opts, err
	}
	if opts.maxDuration, err = cmd.Flags().GetInt("max-duration"); err != nil {
		return opts, err
	}
	if opts.maxVideos, err = cmd.Flags().GetInt("max-videos"); err != nil {
		return opts, err
	}
	if opts.minViews, err = cmd.Flags().GetInt("min-views"); err != nil {
		return opts, err
	}
	if opts.consumePerBatch, err = cmd.Flags().GetInt("consume-per-batch"); err != nil {
		return opts, err
	}
	if opts.skipTranslate, err = cmd.Flags().GetBool("skip-translate"); err != nil {
		return opts, err
	}
	if once, _ := cmd.Flags().GetBool("once"); once {
		opts.maxBatches = 1
	}
	return opts, nil
}

// effectiveSearch 合并 flag 与 config 得到实际搜索参数
func (o *daemonOptions) effectiveSearch(cfg *config.Config) (keywords []string, scorer string, uploadDate string, maxDuration, maxVideos, minViews int) {
	s := cfg.Search
	if s == nil {
		s = &config.SearchConfig{}
	}
	keywords = o.keywords
	if len(keywords) == 0 {
		keywords = s.Keywords
	}
	if len(keywords) == 0 {
		keywords = flattenStandardKeywords()
	}
	scorer = o.scorer
	if scorer == "" {
		scorer = s.Scorer
	}
	if scorer == "" {
		scorer = string(search.ScorerNowcast)
	}
	uploadDate = o.uploadDate
	if uploadDate == "" {
		uploadDate = s.UploadDate
	}
	maxDuration = o.maxDuration
	if maxDuration <= 0 {
		maxDuration = s.MaxDuration
	}
	maxVideos = o.maxVideos
	if maxVideos <= 0 {
		maxVideos = s.MaxVideos
	}
	if maxVideos <= 0 {
		maxVideos = 5
	}
	minViews = o.minViews
	if minViews <= 0 {
		minViews = s.MinViews
	}
	return
}

// flattenStandardKeywords 兜底：config 未配置关键词时用 search.go 标准库
func flattenStandardKeywords() []string {
	var out []string
	for _, cat := range search.StandardKeywords {
		out = append(out, cat.Keywords...)
	}
	return out
}

// runDaemon daemon 主循环
func runDaemon(cfg *config.Config, opts daemonOptions) error {
	d := cfg.Daemon
	if d == nil {
		d = &config.DaemonConfig{}
	}

	// 参数落位（flag > config > 默认）
	if opts.maxBatches <= 0 {
		opts.maxBatches = d.MaxBatches
	}
	if opts.intervalSec <= 0 {
		opts.intervalSec = d.EffectiveInterval()
	}
	if opts.consumePerBatch <= 0 {
		opts.consumePerBatch = d.EffectiveConsumePerBatch()
	}
	opts.taskTimeoutSec = d.TaskTimeoutSec

	keywords, scorer, uploadDate, maxDuration, maxVideos, minViews := opts.effectiveSearch(cfg)

	q := queue.New(cfg.DataDir)
	workerID := queue.WorkerID()

	// 崩溃恢复：把上次运行遗留的 claimed 任务重置回 queued 续跑
	if n, err := q.RequeueClaimed(); err == nil && n > 0 {
		fmt.Printf("♻️ 恢复 %d 个遗留任务（claimed → queued）\n", n)
	}

	// 信号处理：SIGTERM/SIGINT → 等当前任务完成后退出（优雅重启）
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	hb := &heartbeatState{}
	hb.set(daemonHeartbeat{PID: os.Getpid(), StartedAt: time.Now().Format(time.RFC3339)})
	hb.update(func(h daemonHeartbeat) daemonHeartbeat { return h.withStatus("running").withQueue(q.Stats()) })
	writeDaemonHeartbeat(cfg, hb.get())

	// 后台心跳刷新：长任务（如 TTS 40 分钟）期间持续更新 UpdatedAt，
	// 避免监控把"忙碌但健康"的 daemon 误判为卡死。
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone) // goroutine 退出即通知主流程，避免关停死锁
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cur := hb.get()
				cur.UpdatedAt = time.Now().Format(time.RFC3339)
				writeDaemonHeartbeat(cfg, cur)
			}
		}
	}()

	fmt.Printf("🚀 daemon 启动 (worker=%s, 关键词=%d, scorer=%s, 每批≤%d 个)\n",
		workerID, len(keywords), scorer, maxVideos)
	fmt.Printf("   ⏱ 批间休息 %ds | 每批消费排队 ≤%d | 重试上限 %d\n",
		opts.intervalSec, opts.consumePerBatch, d.EffectiveMaxRetries())

	// 失败告警去重（每个任务只告警一次）
	alerted := make(map[string]bool)

	batch := 0
	for {
		if ctx.Err() != nil {
			fmt.Println("🛑 收到退出信号，结束循环")
			break
		}
		if opts.maxBatches > 0 && batch >= opts.maxBatches {
			break
		}
		batch++

		fmt.Printf("\n========== 第 %d 批开始 ==========\n", batch)
		hb.update(func(h daemonHeartbeat) daemonHeartbeat { return h.withBatch(batch).withKeyword("") })

		// 1. 消费排队任务（queued → 串行处理，每批最多 consumePerBatch 个）
		consumed := daemonConsumeQueued(ctx, cfg, q, workerID, opts, hb, alerted)

		// 2. 搜索+评分+去重+入队+直接处理（--submit 直处理模式）
		enqueued := daemonRunAutoBatch(ctx, cfg, q, workerID, keywords, scorer, uploadDate, maxDuration, maxVideos, minViews, opts, hb, alerted)

		// 3. 队列统计 + 心跳
		stats := q.Stats()
		hb.update(func(h daemonHeartbeat) daemonHeartbeat { return h.withQueue(stats).withStatus("running") })
		writeDaemonHeartbeat(cfg, hb.get())
		fmt.Printf("📊 队列: 待处理=%d 处理中=%d 已完成=%d 失败=%d\n",
			stats["queued"], stats["claimed"], stats["completed"], stats["failed"])

		// 4. 无新入队且无待处理 → 轮换关键词（避免空转）
		if enqueued == 0 && consumed == 0 && stats["queued"] == 0 && stats["claimed"] == 0 {
			keywords = rotateKeywords(keywords)
			fmt.Printf("🔁 本批无新视频，轮换关键词 → 下一批: %s\n", keywords[0])
		}

		if opts.maxBatches > 0 && batch >= opts.maxBatches {
			break
		}
		if ctx.Err() != nil {
			break
		}

		fmt.Printf("========== 第 %d 批结束，休息 %ds ==========\n", batch, opts.intervalSec)
		select {
		case <-ctx.Done():
		case <-time.After(time.Duration(opts.intervalSec) * time.Second):
		}
	}

	writeDaemonHeartbeat(cfg, hb.get().withStatus("stopped"))
	stop() // 主动取消 ctx → 心跳 goroutine 退出并 close(heartbeatDone)（SIGTERM 已到时幂等）
	<-heartbeatDone
	fmt.Println("👋 daemon 已退出")
	return nil
}

// daemonConsumeQueued 消费已排队的任务（queued → claimed → 处理 → completed/failed）
func daemonConsumeQueued(ctx context.Context, cfg *config.Config, q *queue.Queue, workerID string, opts daemonOptions, hb *heartbeatState, alerted map[string]bool) int {
	consumed := 0
	for consumed < opts.consumePerBatch {
		if ctx.Err() != nil {
			break // 收到退出信号，不再认领新任务（当前任务由 daemonProcessOne 完成）
		}
		item, err := q.Next(workerID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  ⚠ 队列认领失败: %v\n", err)
			break
		}
		if item == nil {
			break
		}
		consumed++
		hb.update(func(h daemonHeartbeat) daemonHeartbeat { return h.withTask(item.VideoID) })
		writeDaemonHeartbeat(cfg, hb.get())
		daemonProcessOne(ctx, cfg, q, item, opts, alerted)
		hb.update(func(h daemonHeartbeat) daemonHeartbeat { return h.withTask("") })
	}
	return consumed
}

// daemonRunAutoBatch 一批：搜索所有关键词 → 评分 → 去重 → 入队 → 直接处理
func daemonRunAutoBatch(ctx context.Context, cfg *config.Config, q *queue.Queue, workerID string,
	keywords []string, scorer, uploadDate string, maxDuration, maxVideos, minViews int,
	opts daemonOptions, hb *heartbeatState, alerted map[string]bool) int {

	scored, err := searchAndScore(cfg, keywords, scorer, uploadDate, maxDuration, minViews, opts.dryRun)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  ⚠ 搜索失败: %v\n", err)
		return 0
	}
	if len(scored) == 0 {
		fmt.Println("📭 没有找到符合条件的视频")
		return 0
	}
	if opts.dryRun {
		return 0
	}

	// 入队（最多 maxVideos 个，去重由 q.Add 保证）
	enqueued := 0
	for _, sv := range scored {
		if enqueued >= maxVideos {
			break
		}
		added, err := q.Add(sv.ID, sv.URL, sv.Title, sv.ChannelID, "auto")
		if err != nil {
			fmt.Fprintf(os.Stderr, "  ⚠ 入队失败 [%s]: %v\n", sv.Title, err)
			continue
		}
		if added {
			enqueued++
			fmt.Printf("  📥 已入队 [%d/%d]: %s\n", enqueued, maxVideos, sv.Title)
		} else {
			fmt.Printf("  ⏭️ 已在队列/历史中: %s\n", sv.Title)
		}
	}
	if enqueued == 0 {
		return 0
	}

	// 直接处理（--submit 模式，串行）
	fmt.Printf("🚀 开始处理 %d 个新任务...\n", enqueued)
	processed := 0
	for processed < enqueued {
		if ctx.Err() != nil {
			break // 收到退出信号，不再认领新任务
		}
		item, err := q.Next(workerID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  ⚠ 队列认领失败: %v\n", err)
			break
		}
		if item == nil {
			break // 没有更多 queued（可能已被消费）
		}
		processed++
		fmt.Printf("\n[%d/%d] %s\n", processed, enqueued, item.Title)
		hb.update(func(h daemonHeartbeat) daemonHeartbeat { return h.withTask(item.VideoID) })
		writeDaemonHeartbeat(cfg, hb.get())
		daemonProcessOne(ctx, cfg, q, item, opts, alerted)
		hb.update(func(h daemonHeartbeat) daemonHeartbeat { return h.withTask("") })
	}
	return enqueued
}

// daemonProcessOne 处理单个队列任务并更新状态（claimed → completed/failed）。
// 任务使用独立 context（不随 SIGTERM 取消）：收到信号后等当前任务完成再退出（优雅重启）。
func daemonProcessOne(ctx context.Context, cfg *config.Config, q *queue.Queue, item *queue.Video, opts daemonOptions, alerted map[string]bool) {
	taskCtx := context.Background() // 任务不随信号中断，只受超时限制
	cancel := func() {}
	if opts.taskTimeoutSec > 0 {
		taskCtx, cancel = context.WithTimeout(taskCtx, time.Duration(opts.taskTimeoutSec)*time.Second)
	}
	defer cancel()

	res, processErr := processSingleContext(taskCtx, cfg, item.URL, opts.skipTranslate)
	if processErr != nil {
		fmt.Printf("  ❌ %v\n", processErr)
		_ = q.Fail(item.VideoID, processErr.Error())
		// 达到重试上限 → 最终失败：飞书告警（每个任务仅一次）
		if v, err := q.GetByID(item.VideoID); err == nil && v.Status == queue.StatusFailed {
			fmt.Printf("  ⛔ 任务 %s 重试达上限，停止自动重试（需人工介入）\n", item.VideoID)
			if !alerted[item.VideoID] {
				alerted[item.VideoID] = true
				sendFailedAlert(cfg, v)
			}
		}
		return
	}
	bvid := ""
	if res != nil {
		bvid = res.BVID
	}
	if err := q.Complete(item.VideoID, bvid); err != nil {
		fmt.Printf("  ⚠ 队列状态更新失败: %v\n", err)
	}
	fmt.Printf("  ✅ 处理完成: %s\n", bvid)
}

// ─── 搜索评分（auto / daemon 共用）────────────────────────────────────

// searchAndScore 搜索所有关键词 → 跨关键词去重 → 安全过滤 → 多维评分。
// dryRun 时仅打印评分表。返回按分数降序的候选列表。
func searchAndScore(cfg *config.Config, keywords []string, scorer, uploadDate string, maxDuration, minViews int, dryRun bool) ([]search.ScoredVideo, error) {
	scorerType := search.ScorerType(scorer)
	if scorerType == "" {
		scorerType = search.ScorerPopular
	}
	searcher := search.New(20)
	seen := make(map[string]bool)
	var allVideos []search.Video

	fmt.Printf("🤖 自主模式启动（评分策略: %s）\n", scorerType)

	for _, kw := range keywords {
		expandedKW := search.ExpandKeyword(kw)
		safeQuery := search.BuildSearchQuery(kw)
		queryDisplay := safeQuery
		if len(queryDisplay) > 100 {
			queryDisplay = queryDisplay[:100] + "..."
		}
		fmt.Printf("🔍 搜索: %s\n", queryDisplay)

		var opts []search.SearchOption
		opts = append(opts, search.WithSortBy("view_count"))
		if uploadDate != "" {
			opts = append(opts, search.WithUploadDate(uploadDate))
		}

		result, err := searcher.SearchWithOptions(expandedKW, opts...)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  ⚠ 搜索失败: %v\n", err)
			continue
		}

		maxSec := 0
		if maxDuration > 0 {
			maxSec = maxDuration * 60
		}
		filtered := search.ApplySafeSearch(result.Videos, int64(minViews), maxSec)
		for _, v := range filtered {
			if seen[v.ID] {
				continue
			}
			seen[v.ID] = true
			allVideos = append(allVideos, v)
		}
	}

	if len(allVideos) == 0 {
		fmt.Println("📭 没有找到符合条件的视频")
		return nil, nil
	}

	fmt.Printf("\n📊 评分中... (共 %d 个候选视频)\n", len(allVideos))
	var scored []search.ScoredVideo
	if scorerType == search.ScorerNowcast {
		baselines := loadChannelBaselines(cfg.DataDir)
		scored = search.ScoreVideosNowcastFull(allVideos, baselines)
	} else {
		scored = search.ScoreVideos(allVideos, scorerType)
	}

	// dry-run：打印评分表供人工确认（与 auto --dry-run 行为一致）
	if dryRun {
		printScoredTable(scored, scorerType)
		fmt.Printf("\n🔍 预览模式，共 %d 个视频\n", len(scored))
		fmt.Println("   移除 --dry-run 入队，或加 --submit 直接提交处理")
	}
	return scored, nil
}

// printScoredTable 打印评分结果表（auto / daemon 共用）
func printScoredTable(scored []search.ScoredVideo, scorer search.ScorerType) {
	fmt.Println("")
	if scorer == search.ScorerNowcast {
		fmt.Printf("%-3s %-42s %-6s %-10s %-6s %-6s %-6s\n", "#", "标题", "综合分", "播放量", "Nowcast", "Velocity", "时长")
		fmt.Println(strings.Repeat("─", 95))
	} else {
		fmt.Printf("%-3s %-46s %-8s %-10s %-6s %-6s\n", "#", "标题", "综合分", "播放量", "时效", "时长")
		fmt.Println(strings.Repeat("─", 85))
	}
	for i, sv := range scored {
		title := sv.Title
		if len([]rune(title)) > 42 {
			title = string([]rune(title)[:39]) + "..."
		}
		views := sv.Views
		if views == "" {
			views = fmt.Sprintf("%d", sv.ViewCount)
		}
		if scorer == search.ScorerNowcast {
			fmt.Printf("%-3d %-42s %6.2f  %-10s %5.2f  %6.2f  %5.2f\n",
				i+1, title, sv.Score, views, sv.NowcastScore, sv.VelocityScore, sv.DurationScore)
		} else {
			fmt.Printf("%-3d %-46s %6.2f  %-10s %5.2f  %5.2f\n",
				i+1, title, sv.Score, views, sv.RecencyScore, sv.DurationScore)
		}
	}
}

// rotateKeywords 关键词轮换（第一个移到末尾）
func rotateKeywords(kws []string) []string {
	if len(kws) <= 1 {
		return kws
	}
	return append(append([]string{}, kws[1:]...), kws[0])
}

// processSingleContext 处理单个视频的完整流水线（支持超时 context）
func processSingleContext(ctx context.Context, cfg *config.Config, url string, skipTranslate bool) (*pipeline.Result, error) {
	targetLang := cfg.EffectiveTranslationTargetLang()
	if skipTranslate {
		targetLang = ""
	}
	return (&pipeline.Processor{
		Config:   cfg,
		Reporter: pipelineReporter(),
	}).Process(ctx, pipeline.Request{
		URL:        url,
		Source:     "auto",
		TargetLang: targetLang,
	})
}

// ─── 心跳 ──────────────────────────────────────────────────────────────

// daemonHeartbeat 心跳文件结构（data/daemon/heartbeat.json）
type daemonHeartbeat struct {
	Batch       int            `json:"batch"`
	PID         int            `json:"pid"`
	StartedAt   string         `json:"started_at"`
	UpdatedAt   string         `json:"updated_at"`
	Status      string         `json:"status"` // running / stopped
	Keyword     string         `json:"keyword,omitempty"`
	CurrentTask string         `json:"current_task,omitempty"`
	Queue       map[string]int `json:"queue,omitempty"`
	Version     string         `json:"version"`
}

// heartbeatState 线程安全的心跳状态（主循环更新 + 后台刷新协程读取）
type heartbeatState struct {
	mu sync.Mutex
	hb daemonHeartbeat
}

func (s *heartbeatState) get() daemonHeartbeat {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hb
}

func (s *heartbeatState) set(hb daemonHeartbeat) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hb = hb
}

func (s *heartbeatState) update(fn func(daemonHeartbeat) daemonHeartbeat) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hb = fn(s.hb)
}

func (h daemonHeartbeat) withBatch(b int) daemonHeartbeat {
	h.Batch = b
	return h
}

func (h daemonHeartbeat) withKeyword(k string) daemonHeartbeat {
	h.Keyword = k
	return h
}

func (h daemonHeartbeat) withTask(videoID string) daemonHeartbeat {
	h.CurrentTask = videoID
	return h
}

func (h daemonHeartbeat) withStatus(s string) daemonHeartbeat {
	h.Status = s
	h.UpdatedAt = time.Now().Format(time.RFC3339)
	return h
}

func (h daemonHeartbeat) withQueue(stats map[string]int) daemonHeartbeat {
	h.Queue = stats
	return h
}

// heartbeatPath 心跳文件绝对路径
func heartbeatPath(cfg *config.Config) string {
	d := cfg.Daemon
	rel := "daemon/heartbeat.json"
	if d != nil && strings.TrimSpace(d.EffectiveHeartbeatFile()) != "" {
		rel = d.EffectiveHeartbeatFile()
	}
	if filepath.IsAbs(rel) {
		return rel
	}
	return filepath.Join(cfg.DataDir, rel)
}

// writeDaemonHeartbeat 原子写心跳文件
func writeDaemonHeartbeat(cfg *config.Config, hb daemonHeartbeat) error {
	hb.Version = Version
	hb.UpdatedAt = time.Now().Format(time.RFC3339)
	path := heartbeatPath(cfg)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(hb, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ─── 飞书告警 ─────────────────────────────────────────────────────────

// sendFeishuAlert 向飞书自定义机器人 webhook 发送文本消息
func sendFeishuAlert(webhook, text string) error {
	if strings.TrimSpace(webhook) == "" {
		return nil
	}
	payload, _ := json.Marshal(map[string]any{
		"msg_type": "text",
		"content":  map[string]string{"text": text},
	})
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(webhook, "application/json", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("飞书 webhook 返回 %s", resp.Status)
	}
	return nil
}

// sendFailedAlert 任务最终失败告警（飞书 + 日志）
func sendFailedAlert(cfg *config.Config, v *queue.Video) {
	webhook := ""
	if cfg.Daemon != nil {
		webhook = cfg.Daemon.AlertWebhook
	}
	title := v.Title
	if title == "" {
		title = v.VideoID
	}
	msg := fmt.Sprintf("⚠️ ytb2bili 任务失败（重试达上限，需人工介入）\n视频: %s\n链接: %s\n错误: %s",
		title, v.URL, truncateString(v.Error, 200))
	if webhook != "" {
		if err := sendFeishuAlert(webhook, msg); err != nil {
			fmt.Fprintf(os.Stderr, "  ⚠ 飞书告警失败: %v\n", err)
		} else {
			fmt.Println("  📣 已发送飞书告警")
		}
	}
}

func truncateString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// ─── daemon status / check-heartbeat（供 cron 监控）──────────────────

func newDaemonStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "查看 daemon 心跳状态",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			path := heartbeatPath(cfg)
			data, err := os.ReadFile(path)
			if err != nil {
				fmt.Printf("❌ 心跳文件不存在: %s（daemon 未运行?）\n", path)
				return err
			}
			var hb daemonHeartbeat
			if err := json.Unmarshal(data, &hb); err != nil {
				fmt.Printf("❌ 心跳文件解析失败: %v\n", err)
				return err
			}
			fmt.Printf("📡 daemon 心跳:\n")
			fmt.Printf("  状态: %s | 批次: %d | PID: %d\n", hb.Status, hb.Batch, hb.PID)
			fmt.Printf("  启动: %s | 更新: %s\n", hb.StartedAt, hb.UpdatedAt)
			if hb.CurrentTask != "" {
				fmt.Printf("  当前任务: %s\n", hb.CurrentTask)
			}
			if hb.Queue != nil {
				fmt.Printf("  队列: 待处理=%d 处理中=%d 已完成=%d 失败=%d\n",
					hb.Queue["queued"], hb.Queue["claimed"], hb.Queue["completed"], hb.Queue["failed"])
			}
			return nil
		},
	}
}

func newDaemonCheckCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check-heartbeat",
		Short: "检查心跳新鲜度（>N 分钟未更新则飞书告警，cron 用）",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			maxAgeMin, _ := cmd.Flags().GetInt("max-age")
			path := heartbeatPath(cfg)
			fi, err := os.Stat(path)
			if err != nil {
				msg := "🚨 ytb2bili daemon 心跳文件缺失（daemon 可能未运行）"
				alertOrPrint(cfg, msg)
				return fmt.Errorf("心跳文件缺失: %s", path)
			}
			age := time.Since(fi.ModTime())
			if age > time.Duration(maxAgeMin)*time.Minute {
				msg := fmt.Sprintf("🚨 ytb2bili daemon 心跳过期（%s 未更新，阈值 %d 分钟），服务可能卡死", age.Round(time.Minute), maxAgeMin)
				alertOrPrint(cfg, msg)
				return fmt.Errorf("心跳过期: %s", age.Round(time.Minute))
			}
			fmt.Printf("✅ 心跳正常（%s 前更新）\n", age.Round(time.Second))
			return nil
		},
	}
	cmd.Flags().Int("max-age", 15, "心跳最大允许分钟数（默认 15）")
	return cmd
}

func alertOrPrint(cfg *config.Config, msg string) {
	webhook := ""
	if cfg.Daemon != nil {
		webhook = cfg.Daemon.AlertWebhook
	}
	fmt.Println(msg)
	if webhook != "" {
		if err := sendFeishuAlert(webhook, msg); err != nil {
			fmt.Fprintf(os.Stderr, "  ⚠ 飞书告警失败: %v\n", err)
		}
	}
}
