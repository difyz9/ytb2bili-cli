package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

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
			cfg := loadConfig()
			chainSteps := workflow.ParseChain(args[0])
			url := args[1]

			fmt.Printf("🔗 任务链: %s\n", strings.Join(chainSteps, " → "))
			fmt.Printf("📺 %s\n\n", url)

			dryRun, _ := cmd.Flags().GetBool("dry-run")
			skipTrans, _ := cmd.Flags().GetBool("skip-translate")
			tid, _ := cmd.Flags().GetInt("tid")
			if tid == 0 {
				tid = cfg.BiliTid
			}

			processor := &pipeline.Processor{Config: cfg, Reporter: func(event pipeline.Event) {
				if event.Status == "running" {
					fmt.Printf("[%d/%d] %s... ", event.Position, event.Total, event.Step)
				} else if event.Err != nil {
					fmt.Printf("❌ %v\n", event.Err)
				} else {
					fmt.Println("✅")
				}
			}}
			result, err := processor.Process(context.Background(), pipeline.Request{
				URL: url, Chain: chainSteps, DryRun: dryRun,
				SkipTranslate: skipTrans, Tid: tid, Source: "manual",
			})
			if result != nil && result.BVID != "" {
				fmt.Printf("\n📺 https://www.bilibili.com/video/%s\n", result.BVID)
			}
			return err
		},
	}
	runCmd.Flags().Bool("dry-run", false, "仅处理不上传")
	runCmd.Flags().Bool("skip-translate", false, "跳过翻译")
	runCmd.Flags().Int("tid", 0, "B站分区ID")

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

// ─── Submit ────────────────────────────────────────────────────────────────

func newSubmitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "submit <YouTube URL>",
		Short: "提交搬运任务",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("请输入 YouTube URL")
			}
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

			processor := &pipeline.Processor{Config: cfg, Reporter: func(event pipeline.Event) {
				if event.Status == "running" {
					fmt.Printf("[%d/%d] %s... ", event.Position, event.Total, event.Step)
				} else if event.Err != nil {
					fmt.Printf("❌ %v\n", event.Err)
				} else {
					fmt.Println("✅")
				}
			}}
			result, err := processor.Process(context.Background(), pipeline.Request{
				URL: url, SourceLang: sourceLang, TargetLang: targetLang,
				Tid: tid, DryRun: dryRun, SkipTranslate: skipTrans,
				Source: "manual", Chain: workflow.ParseChain(chainStr),
				PlanOnly: showPlan,
			})
			if result != nil {
				fmt.Printf("🔗 任务链: %s\n", strings.Join(result.Plan, " → "))
				if result.BVID != "" {
					fmt.Printf("📺 https://www.bilibili.com/video/%s\n", result.BVID)
				}
			}
			return err
		},
	}
	cmd.Flags().String("source-lang", "en", "源语言")
	cmd.Flags().String("target-lang", "", "目标语言")
	cmd.Flags().Int("tid", 0, "B站分区ID")
	cmd.Flags().Bool("dry-run", false, "仅处理不上传")
	cmd.Flags().Bool("skip-translate", false, "跳过翻译")
	cmd.Flags().Bool("show-plan", false, "只显示规划不执行")
	cmd.Flags().String("chain", "", "自定义任务链")
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

	queueCmd.AddCommand(addCmd, statusCmd, workCmd)
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
			if len(tasks) == 0 {
				fmt.Println("暂无任务")
				return nil
			}
			for _, t := range tasks {
				icon := map[string]string{
					"pending": "\u23f3", "running": "\U0001f504",
					"completed": "\u2705", "failed": "\u274c",
				}[t.Status]
				if icon == "" {
					icon = "\u2753"
				}
				stepsDone := 0
				for _, s := range t.Steps {
					if s.Status == "completed" {
						stepsDone++
					}
				}
				url := t.SourceURL
				if len(url) > 60 {
					url = url[:60] + "..."
				}
				fmt.Printf("%s %s [%d/5] %s %s\n",
					icon, t.ID, stepsDone, url, t.UpdatedAt[:19])
			}
			return nil
		},
	}

	cmd.AddCommand(listCmd)
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
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("请输入频道 ID")
			}
			cfg := loadConfig()
			monitor := channel.NewMonitor(cfg.DataDir)
			title, _ := cmd.Flags().GetString("title")
			sub, err := monitor.AddSubscription(args[0], title)
			if err != nil && sub == nil {
				return err
			}
			if sub != nil {
				fmt.Printf("✅ 已添加频道: %s (%s)\n", sub.ChannelTitle, sub.ChannelID)
			}
			return nil
		},
	}
	addCmd.Flags().StringP("title", "t", "", "频道名称")

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

	ch.AddCommand(addCmd, listCmd, removeCmd)
	return ch
}

// ─── Search ────────────────────────────────────────────────────────────────

func newSearchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "搜索 YouTube 视频",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("请输入搜索关键词")
			}
			cfg := loadConfig()
			query := strings.Join(args, " ")
			maxResults, _ := cmd.Flags().GetInt("max")
			if maxResults <= 0 {
				maxResults = 10
			}

			searcher := search.New(maxResults)
			result, err := searcher.SearchPaginated(query, nil, "")
			if err != nil {
				return fmt.Errorf("搜索失败: %w", err)
			}

			history := storage.NewHistoryStore(filepath.Join(cfg.DataDir, "history"))
			fmt.Printf("🔍 搜索: %s\n", query)
			fmt.Printf("📺 找到 %d 个视频:\n\n", len(result.Videos))
			for i, v := range result.Videos {
				status := ""
				if history.IsSubmitted(v.ID) {
					status = " ✅已提交"
				}
				fmt.Printf("%d. %s%s\n", i+1, v.Title, status)
				fmt.Printf("   频道: %s\n", v.Channel)
				fmt.Printf("   链接: %s\n\n", v.URL)
			}
			return nil
		},
	}
	cmd.Flags().Int("max", 10, "最大结果数")
	return cmd
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

	cmd.AddCommand(statusCmd)
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
			cookiesFile := cfg.YouTubeCookies
			if cookiesFile == "" {
				cookiesFile = filepath.Join(cfg.DataDir, "cookies", "youtube_cookies.txt")
			}
			os.MkdirAll(filepath.Dir(cookiesFile), 0755)

			fmt.Printf("🍪 刷新 YouTube cookies → %s\n", cookiesFile)
			fmt.Println("🔗 连接到 Chrome...")

			cm := cdp.NewChromeManager(cdp.WithPort(9222))
			ctx, cancel, err := cm.ConnectExisting(9222)
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
				fmt.Printf("⚠️ 验证失败: %v\n", err)
				return nil
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
			cookiesFile := cfg.YouTubeCookies
			if cookiesFile == "" {
				cookiesFile = filepath.Join(cfg.DataDir, "cookies", "youtube_cookies.txt")
			}
			if _, err := os.Stat(cookiesFile); err != nil {
				return fmt.Errorf("cookies 文件不存在: %s", cookiesFile)
			}
			if err := cdp.TestYouTubeCookies(cookiesFile); err != nil {
				fmt.Printf("❌ cookies 无效: %v\n", err)
				return nil
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
		Short: "自主模式：自动搜索高价值视频并批量提交",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("请提供搜索关键词")
			}
			cfg := loadConfig()
			maxVideos, _ := cmd.Flags().GetInt("max-videos")
			dryRun, _ := cmd.Flags().GetBool("dry-run")

			history := storage.NewHistoryStore(filepath.Join(cfg.DataDir, "history"))
			searcher := search.New(20)

			fmt.Printf("🤖 自主模式启动\n")
			for _, kw := range args {
				safeQuery := search.BuildSearchQuery(kw)
				fmt.Printf("🔍 搜索: %s\n", safeQuery[:min(len(safeQuery), 100)]+"...")

				result, err := searcher.SearchWithOptions(kw, search.WithSortBy("view_count"))
				if err != nil {
					fmt.Fprintf(os.Stderr, "  ⚠ 搜索失败: %v\n", err)
					continue
				}

				for _, v := range result.Videos {
					if history.IsSubmitted(v.ID) {
						continue
					}
					fmt.Printf("  %s (%s views)\n", v.Title, v.Views)

					if dryRun {
						continue
					}

					_, submitErr := (&pipeline.Processor{Config: cfg}).Process(context.Background(), pipeline.Request{
						URL: v.URL, Source: "auto",
					})
					if submitErr != nil {
						fmt.Printf("  ❌ %v\n", submitErr)
					} else {
						fmt.Printf("  ✅ 已提交\n")
					}
				}

				if len(result.Videos) >= maxVideos {
					break
				}
			}
			return nil
		},
	}
	cmd.Flags().Int("max-videos", 3, "最多提交视频数")
	cmd.Flags().Bool("dry-run", false, "仅搜索不上传")
	return cmd
}

// ─── Server helpers ────────────────────────────────────────────────────────

func newStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "以后台守护进程方式启动 HTTP API 服务器",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
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

			selfPath, _ := os.Executable()
			cmdObj := exec.Command(selfPath, "server")
			cmdObj.Env = os.Environ()
			cmdObj.Stdin = nil
			cmdObj.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

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
			return nil
		},
	}
}

func newStopCmd() *cobra.Command {
	return &cobra.Command{
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
			return nil
		},
	}
}

func newRestartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restart",
		Short: "重启 HTTP API 服务器",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			pidFile := filepath.Join(cfg.DataDir, "server.pid")
			logFile := filepath.Join(cfg.DataDir, "server.log")

			// stop
			if pidData, err := os.ReadFile(pidFile); err == nil {
				var pid int
				fmt.Sscanf(string(pidData), "%d", &pid)
				if proc, err := os.FindProcess(pid); err == nil {
					proc.Signal(syscall.SIGTERM)
					time.Sleep(500 * time.Millisecond)
				}
				os.Remove(pidFile)
			}

			// start
			selfPath, _ := os.Executable()
			cmdObj := exec.Command(selfPath, "server")
			cmdObj.Env = os.Environ()
			cmdObj.Stdin = nil
			cmdObj.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			logF, _ := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			defer logF.Close()
			cmdObj.Stderr = logF
			if err := cmdObj.Start(); err != nil {
				return fmt.Errorf("重启失败: %w", err)
			}
			os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", cmdObj.Process.Pid)), 0644)
			fmt.Printf("🚀 服务已重启 (PID: %d)\n", cmdObj.Process.Pid)
			return nil
		},
	}
}

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
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
}

func newServerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "server",
		Short:  "启动 HTTP API 服务器（前台运行）",
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
	cmd.Flags().String("addr", "127.0.0.1:8096", "监听地址")
	return cmd
}

func newDebugCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "debug",
		Short: "调试模式",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("🔍 调试模式 - 待实现")
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
