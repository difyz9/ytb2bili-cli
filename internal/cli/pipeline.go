package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/zolagz/ytb2bili-go/internal/audiosync"
	"github.com/zolagz/ytb2bili-go/internal/auth"
	"github.com/zolagz/ytb2bili-go/internal/bili"
	"github.com/zolagz/ytb2bili-go/internal/config"
	"github.com/zolagz/ytb2bili-go/internal/channel"
	"github.com/zolagz/ytb2bili-go/internal/server"
	"github.com/zolagz/ytb2bili-go/internal/cdp"
	"github.com/zolagz/ytb2bili-go/internal/pipeline"
	"github.com/zolagz/ytb2bili-go/internal/queue"
	"github.com/zolagz/ytb2bili-go/internal/search"
	"github.com/zolagz/ytb2bili-go/internal/storage"
	"github.com/zolagz/ytb2bili-go/internal/workflow"
)

// ─── Chain ─────────────────────────────────────────────────────────────────

func newChainCmd() *cobra.Command {
	chain := &cobra.Command{
		Use:   "chain",
		Short: "任务链管理",
	}

	runCmd := &cobra.Command{
		Use:   "run <step1,step2,...> <YouTube URL>",
		Short: "运行自定义任务链",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 2 {
				return fmt.Errorf("用法: ytb chain run <step1,step2,...> <YouTube URL>")
			}
			asJSON, _ := cmd.Flags().GetBool("json")
			jsonMode = asJSON
			defer func() { jsonMode = false }()

			cfg := loadConfig()
			chainSteps := workflow.ParseChain(args[0])
			url := args[1]

			outf("🔗 任务链: %s\n", strings.Join(chainSteps, " → "))
			outf("📺 %s\n\n", url)

			dryRun, _ := cmd.Flags().GetBool("dry-run")
			skipTrans, _ := cmd.Flags().GetBool("skip-translate")
			tid, _ := cmd.Flags().GetInt("tid")
			if tid == 0 {
				tid = cfg.BiliTid
			}
			// 语言参数：默认 en → 配置目标语言。修复历史 bug：chain run 未传
			// source-lang 导致语言判定短路，英文原文被当"翻译结果"（BV1GKMr6fEZy）。
			sourceLang, _ := cmd.Flags().GetString("source-lang")
			if sourceLang == "" {
				sourceLang = "en"
			}
			targetLang, _ := cmd.Flags().GetString("target-lang")
			if targetLang == "" {
				targetLang = cfg.EffectiveTranslationTargetLang()
			}

			processor := &pipeline.Processor{Config: cfg, Reporter: pipelineReporter()}
			result, err := processor.Process(context.Background(), pipeline.Request{
				URL: url, Chain: chainSteps, DryRun: dryRun,
				SkipTranslate: skipTrans, Tid: tid, Source: "manual",
				SourceLang: sourceLang, TargetLang: targetLang,
			})
			if result != nil && result.BVID != "" {
				outf("\n📺 https://www.bilibili.com/video/%s\n", result.BVID)
			}
			if err != nil {
				return err
			}
			if asJSON && result != nil {
				return emitJSON(struct {
					OK      bool     `json:"ok"`
					Step    string   `json:"step"`
					TaskID  string   `json:"task_id"`
					VideoID string   `json:"video_id"`
					BVID    string   `json:"bvid"`
					Plan    []string `json:"plan"`
				}{true, "chain", result.TaskID, result.VideoID, result.BVID, result.Plan})
			}
			return nil
		},
	}
	runCmd.Flags().Bool("dry-run", false, "仅处理不上传")
	runCmd.Flags().Bool("skip-translate", false, "跳过翻译")
	runCmd.Flags().Int("tid", 0, "B站分区ID")
	runCmd.Flags().String("source-lang", "", "源语言（默认 en）")
	runCmd.Flags().String("target-lang", "", "目标语言（默认读取配置）")
	runCmd.Flags().Bool("json", false, "以 JSON 输出结果（stdout 仅含 JSON）")

	planCmd := &cobra.Command{
		Use:   "plan <step1,step2,...> <YouTube URL>",
		Short: "查看任务链规划（不执行）",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 2 {
				return fmt.Errorf("用法: ytb chain plan <step1,step2,...> <YouTube URL>")
			}
			cfg := loadConfig()
			chainSteps := workflow.ParseChain(args[0])
			url := args[1]

			processor := &pipeline.Processor{Config: cfg}
			result, err := processor.Process(context.Background(), pipeline.Request{
				URL: url, Chain: chainSteps, PlanOnly: true, Source: "manual",
			})
			if err != nil {
				return err
			}
			fmt.Printf("🔗 任务链规划:\n")
			for i, step := range result.Plan {
				fmt.Printf("  %d. %s\n", i+1, step)
			}
			return nil
		},
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "列出所有可用步骤",
		RunE: func(cmd *cobra.Command, args []string) error {
			steps := []struct{ name, desc string }{
				{"download", "下载视频"},
				{"transcribe", "BCut ASR 语音转字幕"},
				{"translate", "LLM 翻译字幕"},
				{"tts", "IndexTTS 合成分段配音"},
				{"audio-sync", "对齐配音并替换音轨"},
				{"metadata", "AI 生成标题、简介和标签"},
				{"upload", "投稿到 B站"},
			}
			fmt.Println("📋 可用步骤:")
			for _, s := range steps {
				fmt.Printf("  %-14s %s\n", s.name, s.desc)
			}
			return nil
		},
	}

	chain.AddCommand(runCmd, planCmd, listCmd)
	return chain
}

// ─── Audio-Sync ─────────────────────────────────────────────────────────────

func newAudioSyncCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audio-sync <videoId>",
		Short: "对已有下载产物执行音画同步",
		Long: `直接使用 data/downloads/<videoId>/ 下的已有产物做音画同步：
视频 + 译文字幕（优先 zh-Hans，回退源字幕）+ voice/ 配音目录，输出 <videoId>.synced.mp4。

示例:
  ytb audio-sync yn4MSHbKgmo

配合幂等续跑：audio-sync 生成 synced.mp4 后，再执行 submit <videoId> 会跳过
已完成的 download/transcribe/translate/tts/audio-sync，直接做元数据生成和投稿。`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("请输入 videoId 或视频路径")
			}
			asJSON, _ := cmd.Flags().GetBool("json")
			jsonMode = asJSON
			defer func() { jsonMode = false }()

			cfg := loadConfig()
			videoDir := pipeline.ResolveVideoDir(cfg, args[0])
			videoID := filepath.Base(videoDir)

			video, subtitle, voiceDir, err := pipeline.ResolveSyncArtifacts(videoDir, videoID)
			if err != nil {
				return err
			}
			output := filepath.Join(filepath.Dir(video), videoID+".synced.mp4")

			missing, _ := cmd.Flags().GetString("missing")
			noSpeed, _ := cmd.Flags().GetBool("no-speed-adjust")

			outf("🎬 音画同步: %s\n", filepath.Base(video))
			outf("   📄 字幕: %s\n", filepath.Base(subtitle))
			outf("   🎤 配音: %s\n", filepath.Base(voiceDir))

			start := time.Now()
			result, err := audiosync.Sync(context.Background(), audiosync.Options{
				VideoPath: video, SubtitlePath: subtitle, AudioDir: voiceDir, OutputPath: output,
				DisableSpeedAdjust: noSpeed, MissingMode: missing,
			})
			if err != nil {
				return err
			}
			outf("✅ 音画同步完成 (耗时 %v): %s\n", time.Since(start).Round(time.Second), result.Output)
			outf("   📦 时长 %.0fs | 片段 %d | 调整 %d | 缺失 %d\n",
				result.Duration, result.Clips, result.Adjusted, result.Missing)
			if asJSON {
				return emitJSON(struct {
					OK       bool    `json:"ok"`
					Step     string  `json:"step"`
					VideoID  string  `json:"video_id"`
					Output   string  `json:"output"`
					Duration float64 `json:"duration"`
					Clips    int     `json:"clips"`
					Adjusted int     `json:"adjusted"`
					Missing  int     `json:"missing"`
				}{true, "audio-sync", videoID, result.Output, result.Duration, result.Clips, result.Adjusted, result.Missing})
			}
			return nil
		},
	}
	cmd.Flags().String("missing", "", "缺失配音处理: error(默认) / silence")
	cmd.Flags().Bool("no-speed-adjust", false, "不调整配音语速")
	cmd.Flags().Bool("json", false, "以 JSON 输出结果（stdout 仅含 JSON）")
	return cmd
}

// ─── Submit ────────────────────────────────────────────────────────────────

func newSubmitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "submit <YouTube URL>",
		Short: "提交搬运任务",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("请输入 YouTube URL")
			}
			asJSON, _ := cmd.Flags().GetBool("json")
			jsonMode = asJSON
			defer func() { jsonMode = false }()

			cfg := loadConfig()
			url := args[0]

			sourceLang, _ := cmd.Flags().GetString("source-lang")
			targetLang, _ := cmd.Flags().GetString("target-lang")
			if targetLang == "" {
				targetLang = cfg.EffectiveTranslationTargetLang()
			}
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			skipTrans, _ := cmd.Flags().GetBool("skip-translate")
			tid, _ := cmd.Flags().GetInt("tid")
			if tid == 0 {
				tid = cfg.BiliTid
			}
			showPlan, _ := cmd.Flags().GetBool("show-plan")
			chainStr, _ := cmd.Flags().GetString("chain")

			processor := &pipeline.Processor{Config: cfg, Reporter: pipelineReporter()}
			result, err := processor.Process(context.Background(), pipeline.Request{
				URL: url, SourceLang: sourceLang, TargetLang: targetLang,
				Tid: tid, DryRun: dryRun, SkipTranslate: skipTrans,
				Source: "manual", Chain: workflow.ParseChain(chainStr),
				PlanOnly: showPlan,
			})
			if result != nil {
				outf("🔗 任务链: %s\n", strings.Join(result.Plan, " → "))
				if result.BVID != "" {
					outf("📺 https://www.bilibili.com/video/%s\n", result.BVID)
				}
			}
			if err != nil {
				return err
			}
			if asJSON && result != nil {
				return emitJSON(struct {
					OK      bool     `json:"ok"`
					Step    string   `json:"step"`
					TaskID  string   `json:"task_id"`
					VideoID string   `json:"video_id"`
					BVID    string   `json:"bvid"`
					Plan    []string `json:"plan"`
				}{true, "submit", result.TaskID, result.VideoID, result.BVID, result.Plan})
			}
			return nil
		},
	}
	cmd.Flags().String("source-lang", "en", "源语言")
	cmd.Flags().String("target-lang", "", "目标语言")
	cmd.Flags().Int("tid", 0, "B站分区ID")
	cmd.Flags().Bool("dry-run", false, "仅处理不上传")
	cmd.Flags().Bool("skip-translate", false, "跳过翻译")
	cmd.Flags().Bool("show-plan", false, "只显示规划不执行")
	cmd.Flags().String("chain", "", "自定义任务链")
	cmd.Flags().Bool("json", false, "以 JSON 输出结果（stdout 仅含 JSON）")
	return cmd
}

// ─── Queue ─────────────────────────────────────────────────────────────────

func newQueueCmd() *cobra.Command {
	queueCmd := &cobra.Command{
		Use:   "queue",
		Short: "作业队列管理",
	}

	addCmd := &cobra.Command{
		Use:   "add <YouTube URL>",
		Short: "添加视频到队列",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("请输入 YouTube URL")
			}
			cfg := loadConfig()
			url := args[0]
			videoID := queue.ExtractVideoID(url)
			if videoID == "" {
				return fmt.Errorf("无法提取视频 ID")
			}
			q := queue.New(cfg.DataDir)
			added, err := q.Add(videoID, url, "", "", "manual")
			if err != nil {
				return err
			}
			if added {
				fmt.Printf("✅ 已加入队列: %s\n", url)
			} else {
				fmt.Printf("⏭️ 已在队列中: %s\n", url)
			}
			return nil
		},
	}

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "查看队列状态",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			q := queue.New(cfg.DataDir)
			stats := q.Stats()
			fmt.Printf("📊 队列统计:\n")
			fmt.Printf("   总任务:  %d\n", stats["total"])
			fmt.Printf("   ⏳ 排队中: %d\n", stats["queued"])
			fmt.Printf("   🔄 处理中: %d\n", stats["claimed"])
			fmt.Printf("   ✅ 已完成: %d\n", stats["completed"])
			fmt.Printf("   ❌ 已失败: %d\n", stats["failed"])
			fmt.Printf("   ⏭️ 已跳过: %d\n", stats["skipped"])
			return nil
		},
	}

	workCmd := &cobra.Command{
		Use:   "work",
		Short: "启动工作进程",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			q := queue.New(cfg.DataDir)
			workerID := queue.WorkerID()
			once, _ := cmd.Flags().GetBool("once")

			fmt.Printf("🚀 工作进程启动 (Worker: %s)\n", workerID)
			for {
				video, err := q.Next(workerID)
				if err != nil {
					return err
				}
				if video == nil {
					if once {
						fmt.Println("📭 队列为空")
						return nil
					}
					time.Sleep(30 * time.Second)
					continue
				}

				result, submitErr := (&pipeline.Processor{Config: cfg}).Process(context.Background(), pipeline.Request{
					URL: video.URL, Source: video.Source,
				})
				if submitErr != nil {
					q.Fail(video.VideoID, submitErr.Error())
					fmt.Printf("❌ 处理失败: %v\n", submitErr)
				} else {
					bvid := ""
					if result != nil {
						bvid = result.BVID
					}
					q.Complete(video.VideoID, bvid)
					fmt.Printf("✅ 处理完成: %s\n", bvid)
				}

				if once {
					break
				}
			}
			return nil
		},
	}
	workCmd.Flags().Bool("once", false, "只处理一个视频后退出")

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "列出队列中的视频",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			q := queue.New(cfg.DataDir)
			data, err := q.Status()
			if err != nil {
				return err
			}
			if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(data)
			}
			if len(data.Videos) == 0 {
				fmt.Println("📭 队列为空")
				return nil
			}
			fmt.Printf("📋 共 %d 个视频:\n\n", len(data.Videos))
			for _, v := range data.Videos {
				icon := map[string]string{
					queue.StatusQueued: "⏳", queue.StatusClaimed: "🔄", queue.StatusCompleted: "✅",
					queue.StatusFailed: "❌", queue.StatusSkipped: "⏭️", queue.StatusDiscovered: "🔍",
				}[v.Status]
				if icon == "" {
					icon = "❓"
				}
				errInfo := ""
				if v.Status == queue.StatusFailed && v.Error != "" {
					errInfo = "  " + v.Error
				}
				fmt.Printf("  %s %s [%s] %s%s\n", icon, v.VideoID, v.Status, v.Title, errInfo)
			}
			return nil
		},
	}
	listCmd.Flags().Bool("json", false, "以 JSON 输出")

	removeCmd := &cobra.Command{
		Use:   "remove <videoID>",
		Short: "从队列移除视频",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("请输入 videoID")
			}
			cfg := loadConfig()
			q := queue.New(cfg.DataDir)
			if err := q.Remove(args[0]); err != nil {
				return err
			}
			fmt.Printf("🗑 已移除: %s\n", args[0])
			return nil
		},
	}

	clearCmd := &cobra.Command{
		Use:   "clear",
		Short: "清空整个队列",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			q := queue.New(cfg.DataDir)
			data, _ := q.Status()
			if err := q.Clear(); err != nil {
				return err
			}
			fmt.Printf("🗑 已清空队列（%d 条）\n", len(data.Videos))
			return nil
		},
	}

	retryFailedCmd := &cobra.Command{
		Use:   "retry-failed",
		Short: "将失败的视频重新排队",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			q := queue.New(cfg.DataDir)
			data, err := q.Status()
			if err != nil {
				return err
			}
			reset := 0
			for _, v := range data.Videos {
				if v.Status == queue.StatusFailed {
					if err := q.Reset(v.VideoID); err == nil {
						reset++
						fmt.Printf("  🔁 %s\n", v.Title)
					}
				}
			}
			fmt.Printf("✅ 已重新排队 %d 个失败视频\n", reset)
			return nil
		},
	}

	queueCmd.AddCommand(addCmd, statusCmd, workCmd, listCmd, removeCmd, clearCmd, retryFailedCmd)
	return queueCmd
}

// ─── Task ──────────────────────────────────────────────────────────────────

func newTaskCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "task", Short: "任务管理"}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "列出任务",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			ts := storage.NewTaskStore(filepath.Join(cfg.DataDir, "tasks"))
			tasks := ts.List()
			if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(tasks)
			}
			if len(tasks) == 0 {
				fmt.Println("暂无任务")
				return nil
			}
			for _, t := range tasks {
				fmt.Println(formatTaskSummary(t))
			}
			return nil
		},
	}
	listCmd.Flags().Bool("json", false, "以 JSON 输出")

	showCmd := &cobra.Command{
		Use:   "show <task_id>",
		Short: "\u67e5\u770b\u4efb\u52a1\u8be6\u60c5",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("\u8bf7\u8f93\u5165\u4efb\u52a1 ID")
			}
			cfg := loadConfig()
			ts := storage.NewTaskStore(filepath.Join(cfg.DataDir, "tasks"))
			task, err := ts.Get(args[0])
			if err != nil {
				return fmt.Errorf("\u4efb\u52a1 %s \u4e0d\u5b58\u5728\u6216\u8bfb\u53d6\u5931\u8d25: %w", args[0], err)
			}
			fmt.Print(formatTaskDetail(task))
			return nil
		},
	}

	retryCmd := &cobra.Command{
		Use:   "retry <task_id>",
		Short: "重试失败的任务（从失败步骤续跑）",
		Long: `重跑指定任务。幂等步骤会跳过已有产物，只重新执行失败及后续步骤。

示例:
  ytb task retry task_xxx
  ytb task retry --dry-run task_xxx`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("请输入任务 ID")
			}
			cfg := loadConfig()
			ts := storage.NewTaskStore(filepath.Join(cfg.DataDir, "tasks"))
			task, err := ts.Get(args[0])
			if err != nil {
				return fmt.Errorf("任务 %s 不存在: %w", args[0], err)
			}
			if task.SourceURL == "" {
				return fmt.Errorf("任务 %s 没有来源 URL，无法重试", args[0])
			}
			dryRun, _ := cmd.Flags().GetBool("dry-run")

			fmt.Printf("🔁 重试任务 %s\n", args[0])
			fmt.Printf("   📺 %s\n", task.SourceURL)
			processor := &pipeline.Processor{Config: cfg, Reporter: pipelineReporter()}
			result, err := processor.Process(context.Background(), pipeline.Request{
				URL: task.SourceURL, TaskID: task.ID, DryRun: dryRun, Source: "retry",
			})
			if err != nil {
				return err
			}
			if result != nil && result.BVID != "" {
				fmt.Printf("\n📺 https://www.bilibili.com/video/%s\n", result.BVID)
			}
			return nil
		},
	}
	retryCmd.Flags().Bool("dry-run", false, "仅处理不上传")

	cmd.AddCommand(listCmd, showCmd, retryCmd)
	return cmd
}

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

func newSearchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "搜索 YouTube 视频",
		Long: `搜索 YouTube 视频，支持过滤器和输出格式。

示例:
  ytb search "Flutter tutorial"
  ytb search --sort view_count --duration long "AI tutorial"
  ytb search --json --max 5 "Go programming"
  ytb search --submit 1 "Flutter tutorial"    # 直接提交第 1 个结果
  ytb search --history                        # 查看已提交历史`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()

			if history, _ := cmd.Flags().GetBool("history"); history {
				return runSearchHistory(cfg)
			}
			if len(args) == 0 {
				return fmt.Errorf("请输入搜索关键词")
			}

			query := strings.Join(args, " ")
			maxResults, _ := cmd.Flags().GetInt("max")
			if maxResults <= 0 {
				maxResults = 10
			}
			sortBy, _ := cmd.Flags().GetString("sort")
			uploadDate, _ := cmd.Flags().GetString("upload-date")
			duration, _ := cmd.Flags().GetString("duration")
			asJSON, _ := cmd.Flags().GetBool("json")

			searcher := search.New(maxResults)
			result, err := searcher.SearchPaginated(query, searchFilter(sortBy, uploadDate, duration), "")
			if err != nil {
				return fmt.Errorf("搜索失败: %w", err)
			}

			if asJSON {
				return printSearchJSON(result)
			}

			fmt.Print(formatSearchResults(result, submittedSet(cfg)))

			submitN, _ := cmd.Flags().GetInt("submit")
			if submitN > 0 {
				if submitN > len(result.Videos) {
					return fmt.Errorf("--submit %d 超出结果数 %d", submitN, len(result.Videos))
				}
				v := result.Videos[submitN-1]
				fmt.Printf("\n🚀 提交第 %d 个结果: %s\n", submitN, v.Title)
				_, err := processSingle(cfg, v.URL, false)
				return err
			}
			return nil
		},
	}
	cmd.Flags().Int("max", 10, "最大结果数")
	cmd.Flags().String("sort", "", "排序: relevance / upload_date / view_count / rating")
	cmd.Flags().String("upload-date", "", "上传时间: last_hour / today / this_week / this_month / this_year")
	cmd.Flags().String("duration", "", "时长: short(<4m) / medium(4-20m) / long(>20m)")
	cmd.Flags().Bool("json", false, "以 JSON 输出搜索结果")
	cmd.Flags().Bool("history", false, "查看已提交历史（无需关键词）")
	cmd.Flags().Int("submit", 0, "直接提交第 N 个结果")
	return cmd
}

// newHistoryCmd 查看已提交的投稿历史。
func newHistoryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "history",
		Short: "查看已提交的投稿历史",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			history := storage.NewHistoryStore(filepath.Join(cfg.DataDir, "history"))
			videos, err := history.List()
			if err != nil {
				if os.IsNotExist(err) {
					videos = []storage.SubmittedVideo{}
				} else {
					return err
				}
			}
			if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
				if videos == nil {
					videos = []storage.SubmittedVideo{}
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(videos)
			}
			fmt.Print(renderHistory(videos))
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "以 JSON 输出")
	return cmd
}

// runSearchHistory 查看提交历史
func runSearchHistory(cfg *config.Config) error {
	history := storage.NewHistoryStore(filepath.Join(cfg.DataDir, "history"))
	videos, err := history.List()
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("📭 暂无提交历史")
			return nil
		}
		return err
	}
	fmt.Print(renderHistory(videos))
	return nil
}

// submittedSet 返回所有已提交的 YouTube ID 集合
func submittedSet(cfg *config.Config) map[string]bool {
	history := storage.NewHistoryStore(filepath.Join(cfg.DataDir, "history"))
	videos, err := history.List()
	if err != nil {
		return nil
	}
	set := make(map[string]bool, len(videos))
	for _, v := range videos {
		set[v.YouTubeID] = true
	}
	return set
}

// printSearchJSON 以 JSON 输出搜索结果
func printSearchJSON(result *search.SearchResult) error {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

// ─── Subtitle ──────────────────────────────────────────────────────────────

func newSubtitleCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "subtitle", Short: "字幕管理"}

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "查看字幕上传状态",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			entries, _ := os.ReadDir(filepath.Join(cfg.DataDir, "subtitles"))
			if len(entries) == 0 {
				fmt.Println("\U0001f4ed 暂无字幕记录")
				return nil
			}
			fmt.Printf("\U0001f4cb 共 %d 个字幕记录:\n\n", len(entries))
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				fmt.Printf("  %s\n", e.Name())
			}
			return nil
		},
	}

	uploadCmd := &cobra.Command{
		Use:   "upload <bvid> <subtitle.srt>",
		Short: "上传字幕到已发布的视频",
		Long: `上传 SRT 字幕文件到已发布的 B站视频（获取 CID → 转换 → 保存草稿）。
语言可用 --lang 指定（默认 zh）。

示例:
  ytb subtitle upload BV1xx123 subtitle.zh-Hans.srt
  ytb subtitle upload BV1xx123 subtitle.srt --lang en`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 2 {
				return fmt.Errorf("用法: ytb subtitle upload <bvid> <subtitle.srt>")
			}
			cfg := loadConfig()
			cred, err := loadCredential(cfg)
			if err != nil {
				return err
			}
			lang, _ := cmd.Flags().GetString("lang")
			if lang == "" {
				lang = "zh"
			}
			fmt.Printf("📝 上传字幕 %s → %s (lang=%s)\n", args[1], args[0], lang)
			if err := bili.UploadSubtitle(cred, args[0], args[1], lang); err != nil {
				return err
			}
			fmt.Printf("✅ 字幕上传成功: %s\n", args[0])
			return nil
		},
	}
	uploadCmd.Flags().String("lang", "zh", "字幕语言（如 zh / zh-Hans / en）")

	cmd.AddCommand(statusCmd, uploadCmd)
	return cmd
}

// ─── Cookies ───────────────────────────────────────────────────────────────

func newCookiesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cookies",
		Short: "管理 YouTube cookies",
	}

	refreshCmd := &cobra.Command{
		Use:   "refresh",
		Short: "从 Chrome 刷新 YouTube cookies",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			cookiesFile := cfg.EffectiveCookiesPath()
			os.MkdirAll(filepath.Dir(cookiesFile), 0755)

			port := readChromePort(cfg)
			fmt.Printf("🍪 刷新 YouTube cookies → %s\n", cookiesFile)
			fmt.Printf("🔗 连接到 Chrome (端口 %d)...\n", port)

			cm := cdp.NewChromeManager(cdp.WithPort(port))
			ctx, cancel, err := cm.ConnectExisting(port)
			if err != nil {
				return fmt.Errorf("连接 Chrome 失败: %w", err)
			}
			defer cancel()

			fmt.Println("🌐 打开 YouTube 获取 cookies...")
			count, err := cdp.RefreshYouTubeCookies(ctx, cookiesFile)
			if err != nil {
				return fmt.Errorf("刷新 cookies 失败: %w", err)
			}
			fmt.Printf("✅ 已刷新 %d 个 cookies\n", count)

			if err := cdp.TestYouTubeCookies(cookiesFile); err != nil {
				return fmt.Errorf("cookies 验证失败: %w", err)
			}
			fmt.Println("✅ cookies 有效！")
			return nil
		},
	}

	testCmd := &cobra.Command{
		Use:   "test",
		Short: "测试 cookies 是否有效",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			cookiesFile := cfg.EffectiveCookiesPath()
			if _, err := os.Stat(cookiesFile); err != nil {
				return fmt.Errorf("cookies 文件不存在: %s", cookiesFile)
			}
			if err := cdp.TestYouTubeCookies(cookiesFile); err != nil {
				return fmt.Errorf("cookies 无效: %w", err)
			}
			fmt.Println("✅ cookies 有效！")
			return nil
		},
	}

	cmd.AddCommand(refreshCmd, testCmd)
	return cmd
}

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

// processSingle 处理单个视频的完整流水线（auto --submit 内部使用）
// pipelineReporter 返回流水线进度输出的统一回调。
func pipelineReporter() pipeline.Reporter {
	return func(event pipeline.Event) {
		if event.Status == "running" {
			fmt.Printf("[%d/%d] %s... ", event.Position, event.Total, event.Step)
		} else if event.Err != nil {
			fmt.Printf("❌ %v\n", event.Err)
		} else {
			fmt.Println("✅")
		}
	}
}

func processSingle(cfg *config.Config, url string, skipTranslate bool) (*pipeline.Result, error) {
	targetLang := cfg.EffectiveTranslationTargetLang()
	if skipTranslate {
		targetLang = ""
	}
	return (&pipeline.Processor{
		Config:   cfg,
		Reporter: pipelineReporter(),
	}).Process(context.Background(), pipeline.Request{
		URL:        url,
		Source:     "auto",
		TargetLang: targetLang,
	})
}

// ─── Server ────────────────────────────────────────────────────────────────

// serverDaemonCommand 构建以后台方式启动 HTTP 服务的 exec.Cmd（前台进程跑在 server run）。
func serverDaemonCommand(addr string) *exec.Cmd {
	selfPath, _ := os.Executable()
	args := []string{"server", "run"}
	if addr != "" {
		args = append(args, "--addr", addr)
	}
	cmd := exec.Command(selfPath, args...)
	cmd.Env = os.Environ()
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd
}

// ─── Chrome 调试进程管理 ──────────────────────────────────────────────────
// 供 ytb cookies refresh 连接 localhost:9222 提取 YouTube cookies。

func chromePidFile(dataDir string) string {
	return filepath.Join(dataDir, "chrome.pid")
}

func chromePortFile(dataDir string) string {
	return filepath.Join(dataDir, "chrome.port")
}

// readChromePort 读取上次启动记录的 Chrome 调试端口，无记录则用配置起始端口。
func readChromePort(cfg *config.Config) int {
	dataDir := cfg.DataDir
	if data, err := os.ReadFile(chromePortFile(dataDir)); err == nil {
		var port int
		if n, _ := fmt.Sscanf(string(data), "%d", &port); n == 1 && port > 0 {
			return port
		}
	}
	return cfg.EffectiveChromeDebugPort()
}

// startChromeDebug 以远程调试模式启动 Chrome 并记录其 PID 与端口。
// 已在运行则跳过；返回是否本次新启动。
// 说明：直接执行 Chrome 二进制，并用独立 --user-data-dir 启动一个单独的调试实例，
// 与用户日常的 Chrome 互不干扰，且能独立启停。端口用 FindAvailablePort 自动避开占用。
// 支持平台: macOS（/Applications/...）、Linux（google-chrome / chromium 等）。
func startChromeDebug(cfg *config.Config) (started bool, err error) {
	dataDir := cfg.DataDir
	// 已在运行（pid 文件 + 进程存活）则跳过
	if pidData, rerr := os.ReadFile(chromePidFile(dataDir)); rerr == nil {
		var pid int
		fmt.Sscanf(string(pidData), "%d", &pid)
		if proc, perr := os.FindProcess(pid); perr == nil && proc.Signal(syscall.Signal(0)) == nil {
			return false, nil
		}
	}

	chromeBin, err := findChromeBinary()
	if err != nil {
		return false, err
	}
	port := cdp.FindAvailablePort(cfg.EffectiveChromeDebugPort())
	profileDir, _ := filepath.Abs(filepath.Join(dataDir, "chrome-profile"))
	cmd := exec.Command(chromeBin,
		"--remote-debugging-port="+fmt.Sprint(port),
		"--user-data-dir="+profileDir,
		"--no-first-run", "--no-default-browser-check")
	if err := cmd.Start(); err != nil {
		return false, fmt.Errorf("启动 Chrome 失败: %w", err)
	}
	go cmd.Wait() // 回收进程，让 Chrome 独立存活
	os.WriteFile(chromePidFile(dataDir), []byte(fmt.Sprintf("%d", cmd.Process.Pid)), 0644)
	os.WriteFile(chromePortFile(dataDir), []byte(fmt.Sprintf("%d", port)), 0644)
	return true, nil
}

// findChromeBinary 查找本机 Chrome/Chromium 可执行文件路径。
// 按平台顺序探测：macOS 固定路径 → Linux 常见命令/路径 → Windows 常见路径。
func findChromeBinary() (string, error) {
	candidates := []string{}
	switch runtime.GOOS {
	case "darwin":
		candidates = []string{"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"}
	case "windows":
		candidates = []string{
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
		}
	default: // linux 及类 unix
		candidates = []string{
			"google-chrome", "google-chrome-stable", "chromium", "chromium-browser",
			"/usr/bin/google-chrome",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
			"/snap/bin/chromium",
			"/opt/google/chrome/chrome",
		}
	}
	for _, c := range candidates {
		if strings.Contains(c, "/") {
			if _, err := os.Stat(c); err == nil {
				return c, nil
			}
		} else if p, err := exec.LookPath(c); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("未找到 Chrome/Chromium，请安装 Chrome 或 Chromium 浏览器")
}

// stopChromeDebug 停止 Chrome 调试进程并清理 pid/port 文件。
func stopChromeDebug(cfg *config.Config) (stopped bool, err error) {
	dataDir := cfg.DataDir
	pidFile := chromePidFile(dataDir)
	pidData, rerr := os.ReadFile(pidFile)
	os.Remove(pidFile)
	os.Remove(chromePortFile(dataDir))
	if rerr != nil {
		return false, nil // 无记录
	}
	var pid int
	fmt.Sscanf(string(pidData), "%d", &pid)
	if pid <= 0 {
		return false, nil
	}
	proc, perr := os.FindProcess(pid)
	if perr != nil {
		return false, nil
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return false, nil // 已退出
	}
	time.Sleep(300 * time.Millisecond)
	return true, nil
}

func newServerCmd() *cobra.Command {
	srvCmd := &cobra.Command{
		Use:   "server",
		Short: "HTTP API 服务器管理",
		Long:  "启动、停止、重启和查看 HTTP API 服务器状态。",
	}

	runCmd := &cobra.Command{
		Use:    "run",
		Short:  "前台运行 HTTP API 服务器（供后台模式调用）",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			addr, _ := cmd.Flags().GetString("addr")
			if addr == "" {
				addr = "127.0.0.1:8096"
			}

			srv := server.New(cfg)
			fmt.Printf("🚀 启动 ytb2bili HTTP 服务器\n")
			fmt.Printf("   地址: %s\n", addr)
			fmt.Printf("\n📡 API 端点:\n")
			fmt.Printf("   POST /api/v1/submit     - 提交视频\n")
			fmt.Printf("   GET  /api/v1/tasks      - 查看任务列表\n")
			fmt.Printf("   GET  /api/v1/history    - 查看历史记录\n")
			fmt.Printf("   GET  /health            - 健康检查\n")
			fmt.Println()

			return srv.Start(addr)
		},
	}
	runCmd.Flags().String("addr", "127.0.0.1:8096", "监听地址")

	startCmd := &cobra.Command{
		Use:   "start",
		Short: "以后台守护进程方式启动 HTTP API 服务器",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			addr, _ := cmd.Flags().GetString("addr")
			pidFile := filepath.Join(cfg.DataDir, "server.pid")
			logFile := filepath.Join(cfg.DataDir, "server.log")

			if pidData, err := os.ReadFile(pidFile); err == nil {
				var oldPid int
				fmt.Sscanf(string(pidData), "%d", &oldPid)
				if proc, err := os.FindProcess(oldPid); err == nil {
					if err := proc.Signal(syscall.Signal(0)); err == nil {
						return fmt.Errorf("⚠️ 服务已在运行 (PID: %d)", oldPid)
					}
				}
			}

			// 确保 data 目录存在
			if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
				return fmt.Errorf("无法创建数据目录: %w", err)
			}

			cmdObj := serverDaemonCommand(addr)
			logF, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if err != nil {
				return fmt.Errorf("无法创建日志文件: %w", err)
			}
			defer logF.Close()
			cmdObj.Stderr = logF

			if err := cmdObj.Start(); err != nil {
				return fmt.Errorf("启动失败: %w", err)
			}
			os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", cmdObj.Process.Pid)), 0644)
			fmt.Printf("🚀 服务已启动 (PID: %d)\n", cmdObj.Process.Pid)
			fmt.Printf("   日志: %s\n", logFile)

			// 启动 Chrome 调试进程（供 cookies refresh 使用）
			if started, cerr := startChromeDebug(cfg); cerr != nil {
				fmt.Printf("   ⚠ Chrome 调试进程启动失败: %v\n", cerr)
			} else if started {
				fmt.Printf("   🌐 Chrome 调试进程已启动 (端口 %d)\n", readChromePort(cfg))
			} else {
				fmt.Printf("   🌐 Chrome 调试进程已在运行\n")
			}
			return nil
		},
	}
	startCmd.Flags().String("addr", "", "监听地址（默认 127.0.0.1:8096）")

	stopCmd := &cobra.Command{
		Use:   "stop",
		Short: "停止后台 HTTP API 服务器",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			pidFile := filepath.Join(cfg.DataDir, "server.pid")
			pidData, err := os.ReadFile(pidFile)
			if err != nil {
				return fmt.Errorf("未找到运行中的服务")
			}
			var pid int
			fmt.Sscanf(string(pidData), "%d", &pid)
			proc, err := os.FindProcess(pid)
			if err != nil {
				os.Remove(pidFile)
				return fmt.Errorf("无法找到进程 %d", pid)
			}
			if err := proc.Signal(syscall.SIGTERM); err != nil {
				os.Remove(pidFile)
				return fmt.Errorf("停止失败: %w", err)
			}
			time.Sleep(500 * time.Millisecond)
			os.Remove(pidFile)
			fmt.Printf("✅ 服务已停止 (PID: %d)\n", pid)

			// 同步关闭 Chrome 调试进程
			if stopped, cerr := stopChromeDebug(cfg); cerr != nil {
				fmt.Printf("   ⚠ Chrome 调试进程停止失败: %v\n", cerr)
			} else if stopped {
				fmt.Printf("   🌐 Chrome 调试进程已关闭\n")
			}
			return nil
		},
	}

	restartCmd := &cobra.Command{
		Use:   "restart",
		Short: "重启 HTTP API 服务器",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			addr, _ := cmd.Flags().GetString("addr")
			pidFile := filepath.Join(cfg.DataDir, "server.pid")
			logFile := filepath.Join(cfg.DataDir, "server.log")

			// stop（服务 + Chrome 调试进程）
			if pidData, err := os.ReadFile(pidFile); err == nil {
				var pid int
				fmt.Sscanf(string(pidData), "%d", &pid)
				if proc, err := os.FindProcess(pid); err == nil {
					proc.Signal(syscall.SIGTERM)
					time.Sleep(500 * time.Millisecond)
				}
				os.Remove(pidFile)
			}
			if stopped, cerr := stopChromeDebug(cfg); cerr != nil {
				fmt.Printf("   ⚠ Chrome 调试进程停止失败: %v\n", cerr)
			} else if stopped {
				fmt.Printf("   🌐 Chrome 调试进程已关闭\n")
			}

			// start（服务 + Chrome 调试进程）
			cmdObj := serverDaemonCommand(addr)
			logF, _ := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			defer logF.Close()
			cmdObj.Stderr = logF
			if err := cmdObj.Start(); err != nil {
				return fmt.Errorf("重启失败: %w", err)
			}
			os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", cmdObj.Process.Pid)), 0644)
			fmt.Printf("🚀 服务已重启 (PID: %d)\n", cmdObj.Process.Pid)

			if started, cerr := startChromeDebug(cfg); cerr != nil {
				fmt.Printf("   ⚠ Chrome 调试进程启动失败: %v\n", cerr)
			} else if started {
				fmt.Printf("   🌐 Chrome 调试进程已启动 (端口 %d)\n", readChromePort(cfg))
			} else {
				fmt.Printf("   🌐 Chrome 调试进程已在运行\n")
			}
			return nil
		},
	}
	restartCmd.Flags().String("addr", "", "监听地址（默认 127.0.0.1:8096）")

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "查看服务运行状态",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			pidFile := filepath.Join(cfg.DataDir, "server.pid")
			logFile := filepath.Join(cfg.DataDir, "server.log")

			pidData, err := os.ReadFile(pidFile)
			if err != nil {
				fmt.Println("❌ 服务未运行")
				return nil
			}
			var pid int
			fmt.Sscanf(string(pidData), "%d", &pid)

			proc, err := os.FindProcess(pid)
			if err != nil || proc.Signal(syscall.Signal(0)) != nil {
				fmt.Println("❌ 服务未运行 (PID 文件残留)")
				return nil
			}
			fmt.Println("✅ 服务正在运行")
			fmt.Printf("   PID:   %d\n", pid)
			fmt.Printf("   日志:  %s\n", logFile)
			return nil
		},
	}

	srvCmd.AddCommand(runCmd, startCmd, stopCmd, restartCmd, statusCmd)
	return srvCmd
}

func newDebugCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "debug",
		Short: "输出环境与运行状态诊断",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			fmt.Println("🔍 诊断信息")
			fmt.Println(strings.Repeat("=", 40))
			fmt.Printf("版本:     %s\n", Version)
			fmt.Printf("配置:     %s (data_dir=%s)\n", orDefault(configPath, "<默认>"), cfg.DataDir)
			fmt.Printf("LLM:      %s (%s)\n", cfg.LLMModel, cfg.LLMBaseURL)
			fmt.Printf("目标语言: %s\n", cfg.EffectiveTranslationTargetLang())

			fmt.Println()
			fmt.Println("── 环境工具 ──")
			for _, tool := range []string{"yt-dlp", "ffmpeg", "ffprobe", "deno", "python3", "go"} {
				if p, err := exec.LookPath(tool); err == nil {
					fmt.Printf("  %-8s ✅ %s\n", tool, p)
				} else {
					fmt.Printf("  %-8s ❌ 未找到\n", tool)
				}
			}
			if p, err := os.Stat(".venv/bin/python3"); err == nil && !p.IsDir() {
				fmt.Printf("  venv     ✅ .venv/bin/python3\n")
			} else {
				fmt.Printf("  venv     ❌ 未创建（ytb init --venv）\n")
			}

			fmt.Println()
			fmt.Println("── B站登录 ──")
			cs := storage.NewCredentialStore(filepath.Join(cfg.DataDir, "cookies"))
			if cs.Exists() {
				var cred auth.LoginInfo
				if err := cs.Load(&cred); err == nil && cred.TokenInfo.Uname != "" {
					fmt.Printf("  ✅ %s (UID %d)\n", cred.TokenInfo.Uname, cred.TokenInfo.Mid)
				} else {
					fmt.Println("  ⚠ 凭据存在但读取失败")
				}
			} else {
				fmt.Println("  ❌ 未登录（ytb login）")
			}

			fmt.Println()
			fmt.Println("── 数据统计 ──")
			fmt.Printf("  任务:   %d\n", len(storage.NewTaskStore(filepath.Join(cfg.DataDir, "tasks")).List()))
			q := queue.New(cfg.DataDir)
			stats := q.Stats()
			fmt.Printf("  队列:   总%d 排队%d 处理中%d 完成%d 失败%d 跳过%d\n",
				stats["total"], stats["queued"], stats["claimed"], stats["completed"], stats["failed"], stats["skipped"])
			if entries, err := os.ReadDir(filepath.Join(cfg.DataDir, "history")); err == nil {
				fmt.Printf("  历史:   %d\n", len(entries))
			}
			fmt.Println()
			return nil
		},
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
