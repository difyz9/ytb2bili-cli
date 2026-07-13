package command

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v2"

	"github.com/zolagz/ytb2bili-go/internal/cdp"
	"github.com/zolagz/ytb2bili-go/internal/config"
)

// ─── Cookies ─────────────────────────────────────────────────────────────────

func cookiesCommand(cfg *config.Config) *cli.Command {
	return &cli.Command{
		Name:  "cookies",
		Usage: "🍪 管理 YouTube cookies",
		Subcommands: []*cli.Command{
			{
				Name:   "refresh",
				Usage:  "从 Chrome 刷新 YouTube cookies",
				Action: func(c *cli.Context) error {
					return refreshCookiesAction(cfg, c)
				},
			},
			{
				Name:   "test",
				Usage:  "测试 YouTube cookies 是否有效",
				Action: func(c *cli.Context) error {
					return testCookiesAction(cfg, c)
				},
			},
		},
	}
}

func refreshCookiesAction(cfg *config.Config, c *cli.Context) error {
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
		return fmt.Errorf("连接 Chrome 失败（请确保 Chrome 已开启 CDP 端口 9222）: %w", err)
	}
	defer cancel()

	fmt.Println("🌐 打开 YouTube 获取 cookies...")
	count, err := cdp.RefreshYouTubeCookies(ctx, cookiesFile)
	if err != nil {
		return fmt.Errorf("刷新 cookies 失败: %w", err)
	}

	fmt.Printf("✅ 已刷新 %d 个 YouTube cookies\n", count)

	fmt.Println("🔍 验证 cookies...")
	if err := cdp.TestYouTubeCookies(cookiesFile); err != nil {
		fmt.Printf("⚠️  验证失败（可能需要手动登录 YouTube）: %v\n", err)
		return nil
	}

	fmt.Println("✅ cookies 有效！现在可以正常下载 YouTube 视频了")
	return nil
}

func testCookiesAction(cfg *config.Config, c *cli.Context) error {
	cookiesFile := cfg.YouTubeCookies
	if cookiesFile == "" {
		cookiesFile = filepath.Join(cfg.DataDir, "cookies", "youtube_cookies.txt")
	}

	if _, err := os.Stat(cookiesFile); err != nil {
		return fmt.Errorf("cookies 文件不存在: %s", cookiesFile)
	}

	fmt.Printf("🔍 验证 cookies: %s\n", cookiesFile)
	if err := cdp.TestYouTubeCookies(cookiesFile); err != nil {
		fmt.Printf("❌ cookies 无效（需要重新刷新）: %v\n", err)
		return nil
	}

	fmt.Println("✅ cookies 有效！")
	return nil
}
