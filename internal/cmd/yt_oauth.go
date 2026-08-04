package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/zolagz/ytb2bili-go/internal/channel"
	"github.com/zolagz/ytb2bili-go/internal/config"
	"github.com/zolagz/ytb2bili-go/internal/queue"
	"github.com/zolagz/ytb2bili-go/internal/ytoauth"
)

// ytOAuthConfig 从应用配置构建 ytoauth.Config
func ytOAuthConfig(cfg *config.Config) ytoauth.Config {
	oc := ytoauth.Config{
		ClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
	}
	if cfg != nil && cfg.YouTubeOAuth != nil {
		if cfg.YouTubeOAuth.ClientID != "" {
			oc.ClientID = cfg.YouTubeOAuth.ClientID
		}
		if cfg.YouTubeOAuth.ClientSecret != "" {
			oc.ClientSecret = cfg.YouTubeOAuth.ClientSecret
		}
	}
	return oc
}

func newYtOAuthCmd() *cobra.Command {
	ch := &cobra.Command{
		Use:   "yt-oauth",
		Short: "YouTube OAuth 授权登录与订阅频道同步",
		Long: `YouTube OAuth 授权登录，获取用户关注的频道列表并同步到本地。

功能:
  login   设备码授权登录（浏览器打开 URL 输入代码）
  sync    拉取用户订阅频道列表到本地（可自动入队）
  watch   定时检测订阅频道更新，有更新自动加入任务队列
  status  查看登录状态
  logout  清除本地登录凭证

示例:
  ytb yt-oauth login
  ytb yt-oauth sync
  ytb yt-oauth sync --queue
  ytb yt-oauth watch --interval 24h`,
	}

	ch.AddCommand(
		newYtOAuthLoginCmd(),
		newYtOAuthSyncCmd(),
		newYtOAuthWatchCmd(),
		newYtOAuthStatusCmd(),
		newYtOAuthLogoutCmd(),
	)
	return ch
}

// ─── login ───────────────────────────────────────────────────────────────

func newYtOAuthLoginCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "设备码授权登录 YouTube",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			oc := ytOAuthConfig(cfg)
			if err := oc.Validate(); err != nil {
				return err
			}

			store := ytoauth.NewTokenStore(cfg.DataDir)
			if store.Exists() {
				fmt.Println("✅ 已登录（token 存在）。如需重新登录请先: ytb yt-oauth logout")
				return nil
			}

			fmt.Println("🔑 正在发起 YouTube 授权...")
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()

			device, err := ytoauth.StartDeviceFlow(ctx, oc)
			if err != nil {
				return err
			}

			fmt.Println("==============================================")
			fmt.Println("  请在浏览器打开以下地址并输入授权码：")
			fmt.Println()
			fmt.Printf("  🌐 %s\n", device.VerificationURL)
			fmt.Println()
			fmt.Printf("  🔢 授权码: %s\n", device.UserCode)
			fmt.Println("==============================================")
			fmt.Println("等待授权中...（10 分钟内有效，Ctrl+C 取消）")

			tok, err := ytoauth.PollToken(ctx, oc, device)
			if err != nil {
				return err
			}
			if err := store.Save(tok); err != nil {
				return err
			}
			fmt.Println("✅ 授权成功！token 已保存")
			fmt.Printf("   token 文件: %s\n", store.Path())
			fmt.Println("   下一步: ytb yt-oauth sync  拉取订阅频道列表")
			return nil
		},
	}
}

// ─── sync ────────────────────────────────────────────────────────────────

func newYtOAuthSyncCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "拉取用户订阅频道列表到本地",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			oc := ytOAuthConfig(cfg)
			store := ytoauth.NewTokenStore(cfg.DataDir)

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			tok, err := ytoauth.ValidToken(ctx, oc, store)
			if err != nil {
				return err
			}

			fmt.Println("🔄 正在拉取订阅频道列表...")
			subs, err := ytoauth.FetchSubscriptions(ctx, tok.AccessToken)
			if err != nil {
				return err
			}
			fmt.Printf("✅ 从 YouTube 获取到 %d 个订阅频道\n\n", len(subs))

			// 写入本地频道监控存储
			monitor := channel.NewMonitor(cfg.DataDir)
			enqueue, _ := cmd.Flags().GetBool("queue")
			added, updated := 0, 0
			for _, s := range subs {
				sub, err := monitor.AddSubscription(s.ChannelID, s.ChannelTitle)
				if err != nil && sub == nil {
					continue
				}
				if err != nil && sub != nil {
					updated++ // 已存在，更新信息
				} else {
					added++
				}

				if enqueue {
					// 将时间范围内的新视频加入任务队列
					q := queue.New(cfg.DataDir)
					lookback, _ := cmd.Flags().GetInt("lookback")
					monitor.SyncSubscription(*sub, lookback, func(v *channel.DiscoveredVideo) error {
						_, qerr := q.Add(v.VideoID, v.URL, v.Title, v.ChannelID, "yt-oauth")
						return qerr
					})
				}
			}

			fmt.Printf("📺 新增 %d 个频道，更新 %d 个已有频道\n", added, updated)
			fmt.Println("   查看: ytb channel list")
			if !enqueue {
				fmt.Println("   提示: 使用 --queue 同步新视频并入队，或 ytb channel sync --queue")
			}
			return nil
		},
	}
	cmd.Flags().Bool("queue", false, "同步新视频并加入任务队列")
	cmd.Flags().Int("lookback", 7, "同步最近 N 天发布的视频 (0=不限)")
	return cmd
}

// ─── watch ───────────────────────────────────────────────────────────────

func newYtOAuthWatchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "watch",
		Short: "定时检测订阅频道更新，有更新自动加入任务队列",
		Long: `定时检测所有订阅频道的 RSS 更新，发现新视频自动加入任务队列。

默认每 24 小时检测一次（--interval 可调）。加入队列后可用
"ytb queue work" 消费队列执行完整搬运流程（下载→转录→翻译→
元数据→TTS→音画同步→上传B站→字幕）。

示例:
  ytb yt-oauth watch                 # 每 24 小时检测
  ytb yt-oauth watch --interval 12h  # 每 12 小时
  ytb yt-oauth watch --once          # 只检测一次后退出`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			interval, _ := cmd.Flags().GetDuration("interval")
			once, _ := cmd.Flags().GetBool("once")
			lookback, _ := cmd.Flags().GetInt("lookback")

			if interval <= 0 {
				interval = 24 * time.Hour
			}

			monitor := channel.NewMonitor(cfg.DataDir)
			fmt.Printf("🕐 频道更新检测启动（间隔 %s，lookback=%d 天）\n", interval, lookback)
			fmt.Println("   发现新视频将自动加入任务队列")
			fmt.Println("   Ctrl+C 停止")

			for {
				runChannelCheck(cfg, monitor, lookback)

				if once {
					fmt.Println("✅ 单次检测完成")
					return nil
				}
				time.Sleep(interval)
			}
		},
	}
	cmd.Flags().Duration("interval", 24*time.Hour, "检测间隔（如 24h、12h）")
	cmd.Flags().Bool("once", false, "只检测一次后退出")
	cmd.Flags().Int("lookback", 7, "检测最近 N 天发布的视频 (0=不限)")
	return cmd
}

// runChannelCheck 执行一次频道检测，新视频入队
func runChannelCheck(cfg *config.Config, monitor *channel.Monitor, lookback int) {
	q := queue.New(cfg.DataDir)
	newCount, err := monitor.SyncAll(lookback, func(v *channel.DiscoveredVideo) error {
		_, qerr := q.Add(v.VideoID, v.URL, v.Title, v.ChannelID, "channel-watch")
		return qerr
	})
	if err != nil {
		fmt.Printf("⚠ 频道检测失败: %v\n", err)
		return
	}
	if newCount > 0 {
		fmt.Printf("✅ [%s] 发现 %d 个新视频，已加入任务队列\n",
			time.Now().Format("2006-01-02 15:04:05"), newCount)
	} else {
		fmt.Printf("ℹ️ [%s] 无新视频\n", time.Now().Format("2006-01-02 15:04:05"))
	}
}

// ─── status ──────────────────────────────────────────────────────────────

func newYtOAuthStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "查看 YouTube OAuth 登录状态",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			oc := ytOAuthConfig(cfg)
			store := ytoauth.NewTokenStore(cfg.DataDir)

			if !store.Exists() {
				fmt.Println("❌ 未登录。运行: ytb yt-oauth login")
				return nil
			}
			tok, err := store.Load()
			if err != nil {
				return err
			}
			status := "✅ 有效"
			if !tok.Valid() {
				status = "⚠️ 已过期（运行 ytb yt-oauth sync 自动刷新）"
			}
			fmt.Printf("📺 YouTube OAuth 登录状态: %s\n", status)
			fmt.Printf("   token 文件: %s\n", store.Path())
			fmt.Printf("   过期时间: %s\n", tok.Expiry.Format("2006-01-02 15:04:05"))
			if oc.ClientID == "" {
				fmt.Println("   ⚠️ 未配置 client_id（config.yaml → youtube_oauth）")
			}
			return nil
		},
	}
}

// ─── logout ──────────────────────────────────────────────────────────────

func newYtOAuthLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "清除 YouTube OAuth 登录凭证",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			store := ytoauth.NewTokenStore(cfg.DataDir)
			if !store.Exists() {
				fmt.Println("ℹ️ 未登录，无需登出")
				return nil
			}
			if err := store.Clear(); err != nil {
				return err
			}
			fmt.Println("✅ 已登出，本地凭证已清除")
			return nil
		},
	}
}

// strings 保留导入（用于未来扩展）
var _ = strings.TrimSpace
