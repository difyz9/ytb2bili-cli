package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/difyz9/bilibili-go-sdk/bilibili"
	"github.com/spf13/cobra"

	"github.com/zolagz/ytb2bili-go/internal/auth"
	"github.com/zolagz/ytb2bili-go/internal/bili"
	"github.com/zolagz/ytb2bili-go/internal/config"
	"github.com/zolagz/ytb2bili-go/internal/storage"
)

// loadCredential 加载并校验 B站登录凭据。
// accountName 为空时加载默认账号（兼容旧版单账号）。
func loadCredential(cfg *config.Config, accountName ...string) (*auth.LoginInfo, error) {
	acct := ""
	if len(accountName) > 0 {
		acct = accountName[0]
	}
	cs := storage.NewCredentialStore(filepath.Join(cfg.DataDir, "cookies"))
	if !cs.Exists(acct) {
		if acct != "" {
			return nil, fmt.Errorf("账号 %q 未登录，请先执行: ytb login --account %q", acct, acct)
		}
		return nil, fmt.Errorf("未登录，请先执行: ytb login")
	}
	var cred auth.LoginInfo
	if err := cs.Load(&cred, acct); err != nil {
		return nil, fmt.Errorf("读取登录凭据失败: %w", err)
	}
	valid, err := auth.ValidateLogin(&cred)
	if err != nil || !valid {
		if acct != "" {
			return nil, fmt.Errorf("账号 %q 登录已过期，请重新登录: ytb login --account %q", acct, acct)
		}
		return nil, fmt.Errorf("登录已过期，请重新登录: ytb login")
	}
	return &cred, nil
}

// newPublishCmd 直接投稿本地视频到 B站（不经 YouTube 流水线）。
func newPublishCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "publish <video-file>",
		Aliases: []string{"upload"},
		Short:   "直接投稿本地视频到 B站",
		Long: `直接投稿本地视频文件到 B站，无需经过 YouTube 下载流水线。

示例:
  ytb publish video.mp4 --title "我的视频" --tags "科技,评测"
  ytb publish data/downloads/yn4MSHbKgmo/yn4MSHbKgmo.synced.mp4 --title "..."`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("请输入视频文件路径")
			}
			cfg := loadConfig()
			acct, _ := cmd.Flags().GetString("account")
			cred, err := loadCredential(cfg, acct)
			if err != nil {
				return err
			}
			if acct != "" {
				fmt.Printf("👤 投稿账号: %s\n", acct)
			}
			videoPath := args[0]

			title, _ := cmd.Flags().GetString("title")
			if title == "" {
				title = filepath.Base(videoPath)
			}
			desc, _ := cmd.Flags().GetString("desc")
			tagsStr, _ := cmd.Flags().GetString("tags")
			tid, _ := cmd.Flags().GetInt("tid")
			if tid == 0 {
				tid = cfg.BiliTid
			}
			cover, _ := cmd.Flags().GetString("cover")
			source, _ := cmd.Flags().GetString("source")

			var tags []string
			for _, t := range strings.Split(tagsStr, ",") {
				if t = strings.TrimSpace(t); t != "" {
					tags = append(tags, t)
				}
			}

			fmt.Printf("📤 投稿到 B站: %s\n", videoPath)
			fmt.Printf("   📌 标题: %s | 分区: %d\n", title, tid)
			bvid, err := bili.UploadContext(context.Background(), cred, &bili.UploadParams{
				VideoPath: videoPath, Title: title, Desc: desc, Tags: tags,
				Source: source, Tid: tid, CoverPath: cover,
			})
			if err != nil {
				return err
			}
			fmt.Printf("✅ 投稿成功: https://www.bilibili.com/video/%s\n", bvid)
			return nil
		},
	}
	cmd.Flags().String("title", "", "标题（默认用文件名）")
	cmd.Flags().String("desc", "", "简介")
	cmd.Flags().String("tags", "", "标签（逗号分隔）")
	cmd.Flags().Int("tid", 0, "B站分区ID（默认读取配置）")
	cmd.Flags().String("cover", "", "封面图片路径")
	cmd.Flags().String("source", "", "源站 URL")
	cmd.Flags().String("account", "", "投稿账号名（多账号，默认按配置路由）")
	return cmd
}

// newReviewCmd 查看视频投稿审核状态。
func newReviewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "review <bvid>",
		Short: "查看视频投稿审核状态",
		Long: `查看已投稿视频的审核状态。--wait 时持续轮询直到审核通过。

示例:
  ytb review BV1xx123
  ytb review --wait BV1xx123`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("请输入 BVID")
			}
			cfg := loadConfig()
			cred, err := loadCredential(cfg)
			if err != nil {
				return err
			}
			wait, _ := cmd.Flags().GetBool("wait")
			bvid := args[0]

			var status *bilibili.VideoReviewStatus
			if wait {
				fmt.Println("⏳ 等待审核通过（每3分钟轮询，最长24小时）...")
				status, err = bili.WaitForReviewPassed(cred, bvid)
			} else {
				status, err = bili.CheckReviewStatus(cred, bvid)
			}
			if err != nil {
				return err
			}
			fmt.Print(renderReviewStatus(status))
			return nil
		},
	}
	cmd.Flags().Bool("wait", false, "持续轮询直到审核通过")
	return cmd
}
