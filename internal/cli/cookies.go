package cli

// // YouTube cookies：cookies test/refresh

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/zolagz/ytb2bili-go/internal/cdp"
	"github.com/zolagz/ytb2bili-go/internal/download"
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
			fmt.Printf("🍪 刷新 YouTube cookies → %s\n", cookiesFile)
			if err := refreshYouTubeCookiesFromDebugChrome(cfg); err != nil {
				return err
			}
			// 与下载流水线一致：先剔除已轮换的 PSIDTS 令牌再测
			sanitized := download.SanitizeCookieFile(cookiesFile)
			if err := cdp.TestYouTubeCookies(sanitized); err != nil {
				return fmt.Errorf("cookies 验证失败: %w", err)
			}
			fmt.Println("✅ cookies 已刷新且有效！")
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
			// 与下载流水线一致：先剔除已轮换的 PSIDTS 令牌再测，
			// 避免原文件合 st令牌失效而误报（下载时实际用的是净化副本）。
			cookiesFile = download.SanitizeCookieFile(cookiesFile)
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
