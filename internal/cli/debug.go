package cli

// // 环境诊断：debug

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zolagz/ytb2bili-go/internal/auth"
	"github.com/zolagz/ytb2bili-go/internal/queue"
	"github.com/zolagz/ytb2bili-go/internal/storage"
)

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
