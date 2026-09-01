package cli

// // 频道监控：channel add/list/remove/sync/videos/rank/baseline

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/zolagz/ytb2bili-go/internal/channel"
	"github.com/zolagz/ytb2bili-go/internal/queue"
)

// ─── Channel ───────────────────────────────────────────────────────────────

func newChannelCmd() *cobra.Command {
	ch := &cobra.Command{
		Use:   "channel",
		Short: "YouTube 频道监控管理",
	}

	addCmd := &cobra.Command{
		Use:   "add <channel_id>",
		Short: "添加频道订阅",
		Long: `添加频道(UC...)或播放列表(PL...)订阅，并在时间范围内将发现的视频加入任务队列。

示例:
  ytb channel add UCBJcsmduvYEL83R_U4JriQ
  ytb channel add --lookback 14 PLlYbQHffs-L9VmQDOMgRb9ASHPCmieBlK   # 同步最近 14 天
  ytb channel add --lookback 0 <id>                                  # 不限制时间范围`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("请输入频道 ID")
			}
			cfg := loadConfig()
			lookback, _ := cmd.Flags().GetInt("lookback")
			monitor := channel.NewMonitor(cfg.DataDir)
			title, _ := cmd.Flags().GetString("title")
			if title == "" {
				// 未指定 --title 时从 RSS feed 自动获取名称
				title = channel.FetchTitle(args[0])
			}
			if title == "" {
				title = args[0]
			}
			sub, err := monitor.AddSubscription(args[0], title)
			if err != nil && sub == nil {
				return err
			}
			if err != nil && sub != nil {
				fmt.Printf("ℹ️ %v\n", err)
			} else {
				fmt.Printf("✅ 已添加频道: %s (%s)\n", sub.ChannelTitle, sub.ChannelID)
			}

			// 同步该订阅：时间范围内的新视频加入任务队列（队列自身去重）
			fmt.Printf("🔄 同步新视频 (lookback=%d 天)...\n", lookback)
			q := queue.New(cfg.DataDir)
			newCount, serr := monitor.SyncSubscription(*sub, lookback, func(v *channel.DiscoveredVideo) error {
				_, qerr := q.Add(v.VideoID, v.URL, v.Title, v.ChannelID, "channel")
				return qerr
			})
			if serr != nil {
				fmt.Fprintf(os.Stderr, "⚠ 同步失败: %v\n", serr)
				return nil
			}
			if newCount > 0 {
				fmt.Printf("✅ 发现 %d 个新视频并已加入任务队列\n", newCount)
			} else {
				fmt.Println("ℹ️ 时间范围内没有新视频")
			}
			return nil
		},
	}
	addCmd.Flags().StringP("title", "t", "", "频道名称")
	addCmd.Flags().Int("lookback", 7, "同步最近 N 天发布的视频 (0=不限)")

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "列出频道订阅",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			monitor := channel.NewMonitor(cfg.DataDir)
			subs := monitor.ListSubscriptions()
			if len(subs) == 0 {
				fmt.Println("📭 暂无频道订阅")
				return nil
			}
			fmt.Printf("📺 共 %d 个频道订阅:\n\n", len(subs))
			for _, s := range subs {
				lastSync := "从未同步"
				if s.LastSyncAt != "" {
					lastSync = s.LastSyncAt[:19]
				}
				fmt.Printf("  %s\n", s.ChannelTitle)
				fmt.Printf("     Channel ID: %s\n", s.ChannelID)
				fmt.Printf("     上次同步: %s\n", lastSync)
				fmt.Println()
			}
			return nil
		},
	}

	removeCmd := &cobra.Command{
		Use:   "remove <channel_id>",
		Short: "移除频道订阅",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("请输入频道 ID")
			}
			cfg := loadConfig()
			monitor := channel.NewMonitor(cfg.DataDir)
			if err := monitor.RemoveSubscription(args[0]); err != nil {
				return err
			}
			fmt.Printf("✅ 已移除频道: %s\n", args[0])
			return nil
		},
	}

	syncCmd := &cobra.Command{
		Use:   "sync",
		Short: "同步频道 RSS 更新",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			lookback, _ := cmd.Flags().GetInt("lookback")
			enqueue, _ := cmd.Flags().GetBool("queue")
			minDur := cfg.MinDurationSec
			if cmd.Flags().Changed("min-duration") {
				if v, _ := cmd.Flags().GetInt("min-duration"); v >= 0 {
					minDur = v
				}
			}
			monitor := channel.NewMonitor(cfg.DataDir)

			fmt.Printf("🔄 同步频道更新 (lookback=%d 天)...\n", lookback)
			if enqueue && minDur > 0 {
				fmt.Printf("⏭ 入队时跳过低于 %d 秒的短视频\n", minDur)
			}
			newCount, err := monitor.SyncAll(lookback, func(v *channel.DiscoveredVideo) error {
				if !enqueue {
					return nil
				}
				// 过滤 Short/短视频
				skip, derr := channel.ShouldSkipAsShort(context.Background(), cfg, v.VideoID, minDur)
				if derr != nil {
					fmt.Fprintf(os.Stderr, "   ⚠ 查询视频时长失败（仍入队）: %s: %v\n", v.VideoID, derr)
				} else if skip {
					fmt.Printf("   ⏭ 跳过短视频 (%s): %s\n", v.VideoID, v.Title)
					monitor.MarkSkipped(v.VideoID)
					return nil
				}
				q := queue.New(cfg.DataDir)
				_, qerr := q.Add(v.VideoID, v.URL, v.Title, v.ChannelID, "channel")
				return qerr
			})
			if err != nil {
				return err
			}
			fmt.Printf("\n✅ 同步完成，发现 %d 个新视频\n", newCount)
			if newCount > 0 && !enqueue {
				fmt.Println("💡 使用 --queue 自动入队，或 'channel videos' 查看")
			}
			return nil
		},
	}
	syncCmd.Flags().Int("lookback", 7, "仅处理最近 N 天发布的视频 (0=不限)")
	syncCmd.Flags().Int("min-duration", 0, "入队时长下限（秒，覆盖 config 的 min_duration_sec；0=用配置）")
	syncCmd.Flags().Bool("queue", false, "自动将新视频加入处理队列")

	videosCmd := &cobra.Command{
		Use:   "videos",
		Short: "查看发现的视频（按相对频道基线的表现评分排序）",
		Long: `列出监控频道发现的视频，并按"播放量 vs 频道基线"的表现分排序（ytsubs nowcast 思路）。
表现分 = 视频播放量 / 频道基线（来自 channel rank 的 channel_scores.json，缺省用全部视频中位数参照）。

示例:
  ytb channel videos                # 全部，按表现分排序
  ytb channel videos --top 30       # 只看表现最好的 30 个
  ytb channel videos --status new   # 只看待处理`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			top, _ := cmd.Flags().GetInt("top")
			statusFilter, _ := cmd.Flags().GetString("status")
			monitor := channel.NewMonitor(cfg.DataDir)

			videos := monitor.DiscoveredVideos()
			if statusFilter != "" {
				filtered := videos[:0]
				for _, v := range videos {
					if v.Status == statusFilter {
						filtered = append(filtered, v)
					}
				}
				videos = filtered
			}
			if len(videos) == 0 {
				fmt.Println("📭 暂无发现视频")
				return nil
			}

			baselines := loadChannelBaselines(cfg.DataDir)
			// 播放量优先取观测快照（最新一次），否则用发现时记录的 Views
			obsViews := channel.LatestObservationViews(cfg.DataDir)

			// 候选集中位数作缺省参照
			var views []float64
			for _, v := range videos {
				views = append(views, float64(effectiveViews(v.Views, obsViews[v.VideoID])))
			}
			ref := medianFloats(views)

			// 计算表现分并按分数降序
			type scored struct {
				channel.DiscoveredVideo
				Views int
				Score float64
			}
			scoredList := make([]scored, 0, len(videos))
			for _, v := range videos {
				vv := effectiveViews(v.Views, obsViews[v.VideoID])
				bl := baselines[v.ChannelID]
				base := bl.Baseline
				if base <= 0 {
					base = ref
				}
				ratio := 0.0
				if base > 0 {
					ratio = float64(vv+1) / base
				}
				scoredList = append(scoredList, scored{DiscoveredVideo: v, Views: vv, Score: ratio})
			}
			sort.Slice(scoredList, func(i, j int) bool { return scoredList[i].Score > scoredList[j].Score })

			if top > 0 && len(scoredList) > top {
				scoredList = scoredList[:top]
			}

			fmt.Printf("📺 共 %d 个发现视频（按表现分排序）:\n\n", len(scoredList))
			for _, s := range scoredList {
				fmt.Printf("  [%s] 表现 %5.1fx 播放 %7d | %s\n",
					s.Status, s.Score, s.Views, s.Title)
				fmt.Printf("    ID:   %s\n", s.VideoID)
				fmt.Printf("    链接: %s\n", s.URL)
				fmt.Printf("    发布: %s\n\n", truncateTime(s.PublishedAt))
			}
			return nil
		},
	}
	videosCmd.Flags().Int("top", 0, "只显示前 N 个（0=全部）")
	videosCmd.Flags().String("status", "", "按状态过滤: new/queued/submitted/skipped")

	rankCmd := &cobra.Command{
		Use:   "rank",
		Short: "按 ytsubs 式基线评分排名频道质量（仅 RSS，无需 OAuth）",
		Long: `抓取各订阅频道的 RSS，用播放基线评分频道质量（活跃度/基线健康/播放稳定/内容契合），
评分缓存在 data/channel_scores.json。数据源仅 RSS 自带的播放量。

示例:
  ytb channel rank                          # 全部排名
  ytb channel rank --top 50                 # 只看前 50
  ytb channel rank --window 7               # 近 7 天窗口
  ytb channel rank --keywords "ai,flutter,go"
  ytb channel rank --prune-below 40         # 移除低于 40 分的频道`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			windowDays, _ := cmd.Flags().GetInt("window")
			top, _ := cmd.Flags().GetInt("top")
			pruneBelow, _ := cmd.Flags().GetFloat64("prune-below")
			keywordsStr, _ := cmd.Flags().GetString("keywords")

			var keywords []string
			if keywordsStr != "" {
				for _, k := range strings.Split(keywordsStr, ",") {
					if k = strings.TrimSpace(k); k != "" {
						keywords = append(keywords, k)
					}
				}
			}

			monitor := channel.NewMonitor(cfg.DataDir)
			subs := monitor.GetActiveSubscriptions()
			if len(subs) == 0 {
				return fmt.Errorf("没有活跃的频道订阅")
			}
			fmt.Printf("📊 频道质量评分 (window=%d 天, %d 个频道)...\n", windowDays, len(subs))

			// 并发抓取（8 并发）
			var mu sync.Mutex
			var results []*channel.ChannelStats
			sem := make(chan struct{}, 8)
			var wg sync.WaitGroup
			for i := range subs {
				wg.Add(1)
				sem <- struct{}{}
				go func(sub channel.Subscription) {
					defer wg.Done()
					defer func() { <-sem }()
					ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
					defer cancel()
					st, err := channel.ChannelBaseline(ctx, sub, windowDays, keywords)
					if err != nil {
						fmt.Fprintf(os.Stderr, "  ⚠ 抓取失败 %s: %v\n", sub.ChannelTitle, err)
						return
					}
					mu.Lock()
					results = append(results, st)
					mu.Unlock()
				}(subs[i])
			}
			wg.Wait()

			if len(results) == 0 {
				return fmt.Errorf("所有频道抓取失败，请检查网络")
			}
			sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })

			// 缓存评分
			cachePath := filepath.Join(cfg.DataDir, "channel_scores.json")
			if data, err := json.MarshalIndent(results, "", "  "); err == nil {
				os.WriteFile(cachePath, data, 0644)
			}

			fmt.Printf("\n🏆 频道质量排名 (共 %d 个，缓存 %s):\n", len(results), cachePath)
			for i, st := range results {
				if top > 0 && i >= top {
					break
				}
				title := st.ChannelTitle
				if len([]rune(title)) > 30 {
					title = string([]rune(title)[:30]) + "…"
				}
				fmt.Printf("%3d. %-30s 基线%10.0f 活跃%3d 分%5.1f\n",
					i+1, title, st.Baseline, st.Activity, st.Score)
			}

			// 修剪低分频道
			if pruneBelow > 0 {
				removed := 0
				for _, st := range results {
					if st.Score < pruneBelow {
						if err := monitor.RemoveSubscription(st.ChannelID); err == nil {
							fmt.Printf("  ✂ 移除低分频道: %s (%.1f)\n", st.ChannelTitle, st.Score)
							removed++
						}
					}
				}
				fmt.Printf("✅ 已移除 %d 个低分频道\n", removed)
			}
			return nil
		},
	}
	rankCmd.Flags().Int("window", 30, "统计窗口天数")
	rankCmd.Flags().Int("top", 0, "只显示前 N 个（0=全部）")
	rankCmd.Flags().Float64("prune-below", 0, "移除低于此分数的频道（0=不修剪）")
	rankCmd.Flags().String("keywords", "", "内容契合关键词（逗号分隔，空=中性）")

	// ─── channel baseline：yt-dlp 抓最近 N 视频 → trimmed mean 基线 ───
	baselineCmd := &cobra.Command{
		Use:   "baseline <channel_id|@handle>",
		Short: "用 yt-dlp 抓频道最近 N 个视频，计算 trimmed mean 基线（抗爆款污染）",
		Long: `抓取频道最近 N 个视频的播放量，去掉最高/最低各 trim 个后取平均，
得到抗爆款污染的 48h 基线（对标 ytsubs）。同时记录频道粉丝数和采集时间，
供 nowcast 评分的 reach 分量与置信度使用。结果写入 data/channel_scores.json。

示例:
  ytb channel baseline UCgscS8mBsQZ5sFRkJIFWD7Q     # 默认 30 个视频，去 3 个
  ytb channel baseline @RoboNuggets                 # 支持 @handle
  ytb channel baseline --samples 50 --trim 5 <id>   # 自定义参数
  ytb channel baseline --all                        # 所有订阅频道`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			samples, _ := cmd.Flags().GetInt("samples")
			trim, _ := cmd.Flags().GetInt("trim")
			all, _ := cmd.Flags().GetBool("all")
			if samples <= 0 {
				samples = 30
			}
			if trim < 0 {
				trim = 3
			}

			monitor := channel.NewMonitor(cfg.DataDir)

			// 解析目标频道列表
			var subs []channel.Subscription
			if all {
				subs = monitor.GetActiveSubscriptions()
				if len(subs) == 0 {
					return fmt.Errorf("没有活跃的频道订阅")
				}
			} else {
				if len(args) == 0 {
					return fmt.Errorf("请指定频道 ID 或 @handle（或 --all）")
				}
				target := args[0]
				// @handle → channel_id 解析
				if strings.HasPrefix(target, "@") {
					bctx, bcancel := context.WithTimeout(context.Background(), 90*time.Second)
					defer bcancel()
					channelID, err := channel.ResolveHandle(bctx, target)
					if err != nil {
						return fmt.Errorf("解析 @handle 失败: %w", err)
					}
					target = channelID
				}
				// 在订阅列表中查找（未订阅也允许直接计算）
				sub := channel.Subscription{
					ChannelID:    target,
					ChannelTitle: target,
					Type:         channel.DetectType(target),
				}
				for _, s := range monitor.ListSubscriptions() {
					if s.ChannelID == target {
						sub = s
						break
					}
				}
				subs = []channel.Subscription{sub}
			}

			fmt.Printf("🎯 计算频道基线 (samples=%d, trim=%d)...\n", samples, trim)
			var results []*channel.ChannelStats
			for _, sub := range subs {
				bctx, bcancel := context.WithTimeout(context.Background(), 120*time.Second)
				stats, subsCount, err := channel.FetchChannelVideos(bctx, sub.ChannelURL(), samples)
				bcancel()
				if err != nil {
					fmt.Fprintf(os.Stderr, "  ⚠ 抓取失败 %s: %v\n", sub.ChannelTitle, err)
					continue
				}
				var views []float64
				for _, v := range stats {
					views = append(views, float64(v.ViewCount))
				}
				st := &channel.ChannelStats{
					ChannelID:    sub.ChannelID,
					ChannelTitle: sub.ChannelTitle,
					Baseline:     meanFloat(views),
					Baseline48h:  channel.TrimmedMean(views, trim),
					Subscribers:  subsCount,
					UpdatedAt:    time.Now(),
					Samples:      len(views),
				}
				results = append(results, st)
				fmt.Printf("  ✅ %-25s 样本%3d 平均%10.0f trimmed%10.0f 粉丝%8d\n",
					sub.ChannelTitle, len(views), st.Baseline, st.Baseline48h, subsCount)
			}
			if len(results) == 0 {
				return fmt.Errorf("所有频道抓取失败")
			}

			// 合并写回缓存（保留已有频道的其他字段）
			cachePath := filepath.Join(cfg.DataDir, "channel_scores.json")
			var existing []channel.ChannelStats
			if data, err := os.ReadFile(cachePath); err == nil {
				json.Unmarshal(data, &existing)
			}
			byID := make(map[string]channel.ChannelStats, len(existing))
			for _, e := range existing {
				byID[e.ChannelID] = e
			}
			for _, st := range results {
				if old, ok := byID[st.ChannelID]; ok {
					st.Score = old.Score // 保留质量分
				}
				byID[st.ChannelID] = *st
			}
			var merged []channel.ChannelStats
			for _, st := range byID {
				merged = append(merged, st)
			}
			if data, err := json.MarshalIndent(merged, "", "  "); err == nil {
				os.WriteFile(cachePath, data, 0644)
				fmt.Printf("\n✅ 基线已写入 %s (%d 个频道)\n", cachePath, len(merged))
			}
			return nil
		},
	}
	baselineCmd.Flags().Int("samples", 30, "抓取视频数")
	baselineCmd.Flags().Int("trim", 3, "去尾数（最高/最低各 N 个）")
	baselineCmd.Flags().Bool("all", false, "所有订阅频道")

	ch.AddCommand(
		addCmd, listCmd, removeCmd, syncCmd, videosCmd, rankCmd, baselineCmd,
		newChannelImportCmd(), newChannelWatchCmd(),
		newChannelLoginCmd(), newChannelStatusCmd(), newChannelLogoutCmd(),
	)
	return ch
}

// meanFloat 简单平均。
func meanFloat(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := 0.0
	for _, x := range xs {
		s += x
	}
	return s / float64(len(xs))
}

// ─── Search ────────────────────────────────────────────────────────────────
