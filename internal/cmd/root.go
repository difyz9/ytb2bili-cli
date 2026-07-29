package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/zolagz/ytb2bili-go/internal/config"
)

var (
	cfg     *config.Config
	Version = "dev"
)

// Execute 启动 CLI
func Execute() {
	rootCmd := newRootCmd()
	if err := rootCmd.Execute(); err != nil {
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
			var err error
			cfg, err = config.LoadYAML("config.yaml")
			if err != nil {
				cfg = config.Default()
				cfg.Init()
			}
			// 检查是否有子命令注册了 PersistentPreRun，有则调用
			return nil
		},
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	// 注册子命令
	root.AddCommand(
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

// loadConfig 让子命令可以延迟加载配置
func loadConfig() *config.Config {
	if cfg != nil {
		return cfg
	}
	var err error
	cfg, err = config.LoadYAML("config.yaml")
	if err != nil {
		cfg = config.Default()
		cfg.Init()
	}
	return cfg
}

// echo 避免 fmt 报 unused import
var _ = fmt.Println
