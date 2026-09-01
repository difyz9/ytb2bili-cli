package cli

// // HTTP API 服务：server start/stop/status/restart/run

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/zolagz/ytb2bili-go/internal/server"
)

func newServerCmd() *cobra.Command {
	srvCmd := &cobra.Command{
		Use:   "server",
		Short: "HTTP API 服务器管理",
		Long:  "启动、停止、重启和查看 HTTP API 服务器状态。",
	}

	runCmd := &cobra.Command{
		Use:    "run",
		Short:  "前台运行 HTTP API 服务器（供后台模式调用）",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			addr, _ := cmd.Flags().GetString("addr")
			if addr == "" {
				addr = "127.0.0.1:8096"
			}

			srv := server.New(cfg)
			fmt.Printf("🚀 启动 ytb2bili HTTP 服务器\n")
			fmt.Printf("   地址: %s\n", addr)
			fmt.Printf("\n📡 API 端点:\n")
			fmt.Printf("   POST /api/v1/submit     - 提交视频\n")
			fmt.Printf("   GET  /api/v1/tasks      - 查看任务列表\n")
			fmt.Printf("   GET  /api/v1/history    - 查看历史记录\n")
			fmt.Printf("   GET  /health            - 健康检查\n")
			fmt.Println()

			return srv.Start(addr)
		},
	}
	runCmd.Flags().String("addr", "127.0.0.1:8096", "监听地址")

	startCmd := &cobra.Command{
		Use:   "start",
		Short: "以后台守护进程方式启动 HTTP API 服务器",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			addr, _ := cmd.Flags().GetString("addr")
			pidFile := filepath.Join(cfg.DataDir, "server.pid")
			logFile := filepath.Join(cfg.DataDir, "server.log")

			if pidData, err := os.ReadFile(pidFile); err == nil {
				var oldPid int
				fmt.Sscanf(string(pidData), "%d", &oldPid)
				if proc, err := os.FindProcess(oldPid); err == nil {
					if err := proc.Signal(syscall.Signal(0)); err == nil {
						return fmt.Errorf("⚠️ 服务已在运行 (PID: %d)", oldPid)
					}
				}
			}

			// 确保 data 目录存在
			if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
				return fmt.Errorf("无法创建数据目录: %w", err)
			}

			cmdObj := serverDaemonCommand(addr)
			logF, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if err != nil {
				return fmt.Errorf("无法创建日志文件: %w", err)
			}
			defer logF.Close()
			cmdObj.Stderr = logF

			if err := cmdObj.Start(); err != nil {
				return fmt.Errorf("启动失败: %w", err)
			}
			os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", cmdObj.Process.Pid)), 0644)
			fmt.Printf("🚀 服务已启动 (PID: %d)\n", cmdObj.Process.Pid)
			fmt.Printf("   日志: %s\n", logFile)

			// 启动 Chrome 调试进程（供 cookies refresh 使用）
			if started, cerr := startChromeDebug(cfg); cerr != nil {
				fmt.Printf("   ⚠ Chrome 调试进程启动失败: %v\n", cerr)
			} else if started {
				fmt.Printf("   🌐 Chrome 调试进程已启动 (端口 %d)\n", readChromePort(cfg))
			} else {
				fmt.Printf("   🌐 Chrome 调试进程已在运行\n")
			}
			return nil
		},
	}
	startCmd.Flags().String("addr", "", "监听地址（默认 127.0.0.1:8096）")

	stopCmd := &cobra.Command{
		Use:   "stop",
		Short: "停止后台 HTTP API 服务器",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			pidFile := filepath.Join(cfg.DataDir, "server.pid")
			pidData, err := os.ReadFile(pidFile)
			if err != nil {
				return fmt.Errorf("未找到运行中的服务")
			}
			var pid int
			fmt.Sscanf(string(pidData), "%d", &pid)
			proc, err := os.FindProcess(pid)
			if err != nil {
				os.Remove(pidFile)
				return fmt.Errorf("无法找到进程 %d", pid)
			}
			if err := proc.Signal(syscall.SIGTERM); err != nil {
				os.Remove(pidFile)
				return fmt.Errorf("停止失败: %w", err)
			}
			time.Sleep(500 * time.Millisecond)
			os.Remove(pidFile)
			fmt.Printf("✅ 服务已停止 (PID: %d)\n", pid)

			// 同步关闭 Chrome 调试进程
			if stopped, cerr := stopChromeDebug(cfg); cerr != nil {
				fmt.Printf("   ⚠ Chrome 调试进程停止失败: %v\n", cerr)
			} else if stopped {
				fmt.Printf("   🌐 Chrome 调试进程已关闭\n")
			}
			return nil
		},
	}

	restartCmd := &cobra.Command{
		Use:   "restart",
		Short: "重启 HTTP API 服务器",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			addr, _ := cmd.Flags().GetString("addr")
			pidFile := filepath.Join(cfg.DataDir, "server.pid")
			logFile := filepath.Join(cfg.DataDir, "server.log")

			// stop（服务 + Chrome 调试进程）
			if pidData, err := os.ReadFile(pidFile); err == nil {
				var pid int
				fmt.Sscanf(string(pidData), "%d", &pid)
				if proc, err := os.FindProcess(pid); err == nil {
					proc.Signal(syscall.SIGTERM)
					time.Sleep(500 * time.Millisecond)
				}
				os.Remove(pidFile)
			}
			if stopped, cerr := stopChromeDebug(cfg); cerr != nil {
				fmt.Printf("   ⚠ Chrome 调试进程停止失败: %v\n", cerr)
			} else if stopped {
				fmt.Printf("   🌐 Chrome 调试进程已关闭\n")
			}

			// start（服务 + Chrome 调试进程）
			cmdObj := serverDaemonCommand(addr)
			logF, _ := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			defer logF.Close()
			cmdObj.Stderr = logF
			if err := cmdObj.Start(); err != nil {
				return fmt.Errorf("重启失败: %w", err)
			}
			os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", cmdObj.Process.Pid)), 0644)
			fmt.Printf("🚀 服务已重启 (PID: %d)\n", cmdObj.Process.Pid)

			if started, cerr := startChromeDebug(cfg); cerr != nil {
				fmt.Printf("   ⚠ Chrome 调试进程启动失败: %v\n", cerr)
			} else if started {
				fmt.Printf("   🌐 Chrome 调试进程已启动 (端口 %d)\n", readChromePort(cfg))
			} else {
				fmt.Printf("   🌐 Chrome 调试进程已在运行\n")
			}
			return nil
		},
	}
	restartCmd.Flags().String("addr", "", "监听地址（默认 127.0.0.1:8096）")

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "查看服务运行状态",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadConfig()
			pidFile := filepath.Join(cfg.DataDir, "server.pid")
			logFile := filepath.Join(cfg.DataDir, "server.log")

			pidData, err := os.ReadFile(pidFile)
			if err != nil {
				fmt.Println("❌ 服务未运行")
				return nil
			}
			var pid int
			fmt.Sscanf(string(pidData), "%d", &pid)

			proc, err := os.FindProcess(pid)
			if err != nil || proc.Signal(syscall.Signal(0)) != nil {
				fmt.Println("❌ 服务未运行 (PID 文件残留)")
				return nil
			}
			fmt.Println("✅ 服务正在运行")
			fmt.Printf("   PID:   %d\n", pid)
			fmt.Printf("   日志:  %s\n", logFile)
			return nil
		},
	}

	srvCmd.AddCommand(runCmd, startCmd, stopCmd, restartCmd, statusCmd)
	return srvCmd
}
