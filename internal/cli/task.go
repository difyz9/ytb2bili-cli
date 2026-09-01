package cli

// // 任务管理：task list/show/retry

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/zolagz/ytb2bili-go/internal/pipeline"
	"github.com/zolagz/ytb2bili-go/internal/storage"
)

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
