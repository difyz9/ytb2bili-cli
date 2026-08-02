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

	// 注册子命令
	root.AddCommand(
		newInitCmd(),
		newLoginCmd(),
		newWhoamiCmd(),
		newDownloadCmd(),
		newBcutCmd(),
		newTranslateCmd(),
		newTencentTTSCmd(),
		newChainCmd(),
		newSubmitCmd(),
		newSearchCmd(),
		newChannelCmd(),
		newQueueCmd(),
		newTaskCmd(),
		newSubtitleCmd(),
		newCookiesCmd(),
		newAutoCmd(),

		// server 类命令
		newStartCmd(),
		newStopCmd(),
		newRestartCmd(),
		newStatusCmd(),
		newServerCmd(),

		// 调试
		newDebugCmd(),
	)

	return root
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
