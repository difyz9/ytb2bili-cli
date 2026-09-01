package cli

// // YouTube cookies：cookies test/refresh

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/zolagz/ytb2bili-go/internal/cdp"
)

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
