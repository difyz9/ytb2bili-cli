package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/zolagz/ytb2bili-go/internal/config"
)

var (
	cfg        *config.Config
	configPath string // 已解析的配置文件路径（"" = 使用默认配置）
	Version    = "dev"
)

// Execute 启动 CLI
func Execute() {
	rootCmd := newRootCmd()
	if err := rootCmd.Execute(); err != nil {
		// SilenceErrors 为 true，cobra 不打印错误，这里统一输出
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:     "ytb",
		Short:   "YouTube → Bilibili 视频搬运工具",
		Version: Version,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// 子命令共享的 config 初始化
			if cfg != nil {
				return nil
			}
			flagVal, _ := cmd.Flags().GetString("config")
			configPath = resolveConfigPath(flagVal)
			loaded, err := loadConfigAt(configPath)
			if err != nil {
				return fmt.Errorf("加载配置 %s 失败: %w", configPath, err)
			}
			cfg = loaded
			return nil
		},
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	root.PersistentFlags().String("config", "", "配置文件路径（默认 ./config.yaml 或 $YTB2BILI_CONFIG）")

	// 命令分组（--help 按逻辑分区展示，不改命令可调用性）
	root.AddGroup(
		&cobra.Group{ID: "workflow", Title: "核心流程"},
		&cobra.Group{ID: "subscribe", Title: "频道与订阅"},
		&cobra.Group{ID: "steps", Title: "流水线步骤"},
		&cobra.Group{ID: "bili", Title: "B站管理"},
		&cobra.Group{ID: "system", Title: "系统与工具"},
	)

	// 注册子命令
	root.AddCommand(
		// 核心流程
		group(newSubmitCmd(), "workflow"),
		group(newAutoCmd(), "workflow"),
		group(newSearchCmd(), "workflow"),
		group(newQueueCmd(), "workflow"),

		// 频道与订阅
		group(newChannelCmd(), "subscribe"),
		group(newYtOAuthCmd(), "subscribe"),
		group(newCookiesCmd(), "subscribe"),

		// 流水线步骤
		group(newDownloadCmd(), "steps"),
		group(newBcutCmd(), "steps"),
		group(newWhisperCmd(), "steps"),
		group(newTranscribeCmd(), "steps"),
		group(newMetadataCmd(), "steps"),
		group(newTranslateCmd(), "steps"),
		group(newTencentTTSCmd(), "steps"),
		group(newAudioSyncCmd(), "steps"),

		// B站管理
		group(newLoginCmd(), "bili"),
		group(newWhoamiCmd(), "bili"),
		group(newPublishCmd(), "bili"),
		group(newReviewCmd(), "bili"),
		group(newSubtitleCmd(), "bili"),
		group(newHistoryCmd(), "bili"),
		group(newTaskCmd(), "bili"),

		// 系统与工具
		group(newInitCmd(), "system"),
		group(newServerCmd(), "system"),
		group(newChainCmd(), "system"),
		group(newDebugCmd(), "system"),
	)

	return root
}

// group 设置命令所属分组（用于 --help 逻辑分区）。
func group(c *cobra.Command, id string) *cobra.Command {
	c.GroupID = id
	return c
}

// resolveConfigPath 按优先级解析配置文件路径：
// 1. --config flag  2. $YTB2BILI_CONFIG  3. 当前目录的 config.yaml
// 全部不存在时返回空字符串（调用方回退到默认配置）。
func resolveConfigPath(flagVal string) string {
	if flagVal != "" {
		return flagVal
	}
	if env := os.Getenv("YTB2BILI_CONFIG"); env != "" {
		return env
	}
	if _, err := os.Stat("config.yaml"); err == nil {
		return "config.yaml"
	}
	return ""
}

// loadConfigAt 从指定路径加载配置；path 为空时返回带环境变量的默认配置。
func loadConfigAt(path string) (*config.Config, error) {
	if path == "" {
		c := config.Default()
		c.Init()
		return c, nil
	}
	return config.LoadYAML(path)
}

// loadConfig 让子命令可以延迟加载配置
func loadConfig() *config.Config {
	if cfg != nil {
		return cfg
	}
	loaded, err := loadConfigAt(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠ 加载配置 %s 失败: %v\n", configPath, err)
		loaded = config.Default()
		loaded.Init()
	}
	cfg = loaded
	return cfg
}
