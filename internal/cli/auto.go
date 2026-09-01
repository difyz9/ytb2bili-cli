package cli

// // 自主模式：auto（搜索→评分→去重→入队/处理）+ nowcast 评分辅助

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/zolagz/ytb2bili-go/internal/channel"
	"github.com/zolagz/ytb2bili-go/internal/queue"
	"github.com/zolagz/ytb2bili-go/internal/search"
)

// ─── Auto ──────────────────────────────────────────────────────────────────

func newAutoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auto <keyword1> [keyword2 ...]",
		Short: "自主模式：自动搜索高价值视频并批量处理",
		Long: `自主模式自动搜索关键词、多维评分筛选、接入任务队列。

支持多种评分策略（popular / fresh / balanced / nowcast）：

  popular   播放量优先（默认），适合追求热门内容
  fresh     时效优先，仅取近期发布视频
  balanced  均衡评分，兼顾播放量、时效和内容时长
  nowcast   ytsubs 式：播放 vs 频道基线，捕捉超出常态/正在起势的视频（需先 channel rank 生成基线缓存）

示例：
  ytb auto "flutter tutorial"                              # 默认评分，入队
  ytb auto --scorer balanced --min-views 1000 "AI"         # 均衡评分 + 播放量门槛
  ytb auto --dry-run --scorer fresh "golang tutorial"      # 仅查看评分结果
  ytb auto --submit --max-videos 5 "machine learning"      # 直接提交处理`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()

			// 关键词：CLI 参数优先，无参数时读 config.yaml search.keywords（单一来源）
			if len(args) == 0 {
				if cfg.Search != nil && len(cfg.Search.Keywords) > 0 {
					args = cfg.Search.Keywords
				} else {
					args = flattenStandardKeywords()
				}
			}

			maxVideos, _ := cmd.Flags().GetInt("max-videos")
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			submit, _ := cmd.Flags().GetBool("submit")
			minViews, _ := cmd.Flags().GetInt("min-views")
			duration, _ := cmd.Flags().GetString("duration")
			maxDuration, _ := cmd.Flags().GetInt("max-duration")
			uploadDate, _ := cmd.Flags().GetString("upload-date")
			skipTranslate, _ := cmd.Flags().GetBool("skip-translate")
			scorerStr, _ := cmd.Flags().GetString("scorer")

			// 配置兜底：flag 未指定时取 config.yaml search 段（P1 收敛单一来源）
			if cfg.Search != nil {
				if maxVideos <= 0 {
					maxVideos = cfg.Search.MaxVideos
				}
				if minViews <= 0 {
					minViews = cfg.Search.MinViews
				}
				if maxDuration <= 0 {
					maxDuration = cfg.Search.MaxDuration
				}
				if uploadDate == "" {
					uploadDate = cfg.Search.UploadDate
				}
				if scorerStr == "" {
					scorerStr = cfg.Search.Scorer
				}
			}
			if maxVideos <= 0 {
				maxVideos = 3
			}
			if scorerStr == "" {
				scorerStr = string(search.ScorerPopular)
			}

			scorer := search.ScorerType(scorerStr)
			searcher := search.New(20)
			seen := make(map[string]bool) // 跨关键词全局去重
			var allVideos []search.Video

			fmt.Printf("🤖 自主模式启动（评分策略: %s）\n", scorer)

			// ── Step 1: 搜索所有关键词 ──
			for _, kw := range args {
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
				if duration != "" {
					opts = append(opts, search.WithDuration(duration))
				}

				result, err := searcher.SearchWithOptions(expandedKW, opts...)
				if err != nil {
					fmt.Fprintf(os.Stderr, "  ⚠ 搜索失败: %v\n", err)
					continue
				}

				// ApplySafeSearch: 去重、黑名单、min-views、时长上限（max-duration 分钟→秒）
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
				return nil
			}

			// ── Step 2: 多维评分 ──
			fmt.Printf("\n📊 评分中... (共 %d 个候选视频)\n", len(allVideos))
			var scored []search.ScoredVideo
			if scorer == search.ScorerNowcast {
				baselines := loadChannelBaselines(cfg.DataDir)
				scored = search.ScoreVideosNowcastFull(allVideos, baselines)
			} else {
				scored = search.ScoreVideos(allVideos, scorer)
			}
			// 放宽截取：多留候选供去重后补充（重复视频会被跳过，直到凑满 maxVideos）
			if len(scored) > maxVideos*3 {
				scored = scored[:maxVideos*3]
			}

			// ── Step 3: 打印评分表格 ──
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

			if dryRun {
				fmt.Printf("\n🔍 预览模式，共 %d 个视频\n", len(scored))
				fmt.Println("   移除 --dry-run 入队，或加 --submit 直接提交处理")
				return nil
			}

			// ── Step 4: 接入 Queue ──
			q := queue.New(cfg.DataDir)
			queued := 0
			for _, sv := range scored {
				if queued >= maxVideos {
					break
				}
				added, err := q.Add(sv.ID, sv.URL, sv.Title, sv.ChannelID, "auto")
				if err != nil {
					fmt.Fprintf(os.Stderr, "  ⚠ 入队失败 [%s]: %v\n", sv.Title, err)
					continue
				}
				if added {
					queued++
					fmt.Printf("  📥 已入队 [%d/%d]: %s\n", queued, maxVideos, sv.Title)
				} else {
					fmt.Printf("  ⏭️ 已在队列/历史中: %s\n", sv.Title)
				}
			}

			stats := q.Stats()
			fmt.Printf("\n📊 队列状态:\n")
			fmt.Printf("   ⏳ 排队中: %d\n", stats["queued"])
			fmt.Printf("   🔄 处理中: %d\n", stats["claimed"])
			fmt.Printf("   ✅ 已完成: %d\n", stats["completed"])
			fmt.Printf("   ❌ 已失败: %d\n", stats["failed"])
			fmt.Println("\n💡 使用 'queue work' 消费队列，或 'queue status' 查看进度")

			// ── Step 5: --submit 模式：直接处理 ──
			// 注意：--submit 是快捷方式，跳过 queue work 直接处理
			// queue.Add 已经记录了发现记录，history 记录实际提交
			if submit {
				fmt.Println("\n🚀 --submit 模式，开始处理...")
				processed := 0
				for processed < len(scored) {
					// 从队列认领下一个待处理视频（queued → claimed）
					item, err := q.Next("auto")
					if err != nil {
						fmt.Fprintf(os.Stderr, "  ⚠ 队列认领失败: %v\n", err)
						break
					}
					if item == nil {
						// 没有更多 queued 任务（可能全部已 claimed/处理中）
						break
					}
					processed++
					fmt.Printf("\n[%d/%d] %s\n", processed, len(scored), item.Title)
					res, processErr := processSingle(cfg, item.URL, skipTranslate)
					if processErr != nil {
						fmt.Printf("  ❌ %v\n", processErr)
						// 队列状态: claimed → failed（自动重试或最终失败）
						_ = q.Fail(item.VideoID, processErr.Error())
					} else {
						fmt.Printf("  ✅ 处理完成\n")
						// 队列状态: claimed → completed（携带投稿 BVID）
						bvid := ""
						if res != nil {
							bvid = res.BVID
						}
						if err := q.Complete(item.VideoID, bvid); err != nil {
							fmt.Printf("  ⚠ 队列状态更新失败: %v\n", err)
						}
					}
				}
			}

			return nil
		},
	}

	cmd.Flags().Int("max-videos", 3, "最多提交视频数")
	cmd.Flags().Bool("dry-run", false, "仅搜索不入队/提交")
	cmd.Flags().Bool("submit", false, "入队后直接处理（默认只入队到 queue）")
	cmd.Flags().Int("min-views", 0, "最低播放量过滤")
	cmd.Flags().String("duration", "", "时长过滤: short(<4m) / medium(4-20m) / long(>20m)")
	cmd.Flags().Int("max-duration", 0, "最大视频时长（分钟），0=不限（例: 40 = 仅搬运40分钟以内视频）")
	cmd.Flags().String("upload-date", "", "上传日期: last_hour / today / this_week / this_month / this_year")
	cmd.Flags().Bool("skip-translate", false, "跳过翻译")
	cmd.Flags().String("scorer", "popular", "评分策略: popular / fresh / balanced / nowcast")

	return cmd
}

// loadChannelBaselines 读取 channel rank/baseline 的评分缓存（data/channel_scores.json），
// 返回 map[channel_id]NowcastBaseline 供 nowcast 评分使用。
// 基线优先取 baseline_48h（trimmed mean，抗爆款）；无则退化 baseline。
// 时间戳取缓存文件 mtime（近似），粉丝数取抓取值（reach 分量可用）。
func loadChannelBaselines(dataDir string) map[string]search.NowcastBaseline {
	path := filepath.Join(dataDir, "channel_scores.json")
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  ⚠ 未找到频道基线缓存 %s（请先运行 channel rank/baseline）：%v\n", path, err)
		return nil
	}
	var stats []channel.ChannelStats
	if err := json.Unmarshal(data, &stats); err != nil {
		fmt.Fprintf(os.Stderr, "  ⚠ 解析频道基线缓存失败: %v\n", err)
		return nil
	}
	// 缓存文件修改时间作为基线采集时间（近似）
	updatedAt := time.Time{}
	if fi, err := os.Stat(path); err == nil {
		updatedAt = fi.ModTime()
	}
	baselines := make(map[string]search.NowcastBaseline, len(stats))
	for _, s := range stats {
		base := s.Baseline48h
		if base <= 0 {
			base = s.Baseline
		}
		blUpdated := s.UpdatedAt
		if blUpdated.IsZero() {
			blUpdated = updatedAt
		}
		baselines[s.ChannelID] = search.NowcastBaseline{
			Baseline:    base,
			Subscribers: s.Subscribers,
			UpdatedAt:   blUpdated,
			HasBaseline: base > 0,
		}
	}
	return baselines
}

// effectiveViews 返回视频播放量：观测快照有值优先，否则用发现时记录的 Views。
func effectiveViews(stored int, observed int) int {
	if observed > 0 {
		return observed
	}
	return stored
}

// medianFloats 计算 []float64 的中位数。
func medianFloats(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	sorted := append([]float64(nil), xs...)
	sort.Float64s(sorted)
	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}
