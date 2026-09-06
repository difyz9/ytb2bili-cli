package cli

// // 作业队列：queue add/status/work/list/remove/clear/retry-failed

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/zolagz/ytb2bili-go/internal/pipeline"
	"github.com/zolagz/ytb2bili-go/internal/queue"
)

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

	auditCmd := &cobra.Command{
		Use:   "audit",
		Short: "审计事件与失败分类统计（来自 data/audit/events.jsonl）",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			a := queue.OpenAudit(cfg.DataDir)
			recent, _ := cmd.Flags().GetInt("recent")
			sum, err := a.SummarizeFailures(recent)
			if err != nil {
				return err
			}
			fmt.Print(sum.String())
			if recent > 0 && len(sum.RecentErrors) > 0 {
				fmt.Printf("最近 %d 条失败:\n", len(sum.RecentErrors))
				for _, ev := range sum.RecentErrors {
					errText := ev.Error
					if len(errText) > 160 {
						errText = errText[:160] + "..."
					}
					fmt.Printf("  ❌ [%s] %s %s: %s\n", ev.ErrorClass, ev.VideoID, ev.Ts, errText)
				}
			}
			return nil
		},
	}
	auditCmd.Flags().Int("recent", 10, "额外打印最近 N 条失败详情(0=不打印)")

	queueCmd.AddCommand(addCmd, statusCmd, workCmd, listCmd, removeCmd, clearCmd, retryFailedCmd, auditCmd)
	return queueCmd
}

// ─── Task ──────────────────────────────────────────────────────────────────
