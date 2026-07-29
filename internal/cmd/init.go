package cmd

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "检查并修复环境依赖",
		Long: `检查系统环境依赖并自动修复：
  - yt-dlp：检查安装状态，可选更新到最新版
  - ffmpeg：检查是否可用
  - Python 依赖：安装 requests（yt-dlp impersonation 支持）
  - deno：检查是否安装（可选）`,
		RunE: func(cmd *cobra.Command, args []string) error {
			updateFlag, _ := cmd.Flags().GetBool("update")
			pipFlag, _ := cmd.Flags().GetBool("pip")

			fmt.Println("🩺 环境依赖检查")
			fmt.Println(strings.Repeat("=", 40))

			// 1. ffmpeg
			fmt.Print("\n[1/4] ffmpeg... ")
			ffmpegPath, _ := exec.LookPath("ffmpeg")
			if ffmpegPath != "" {
				out, _ := exec.Command("ffmpeg", "-version").Output()
				version := strings.SplitN(string(out), " ", 4)
				if len(version) >= 3 {
					fmt.Printf("✅ %s %s\n", ffmpegPath, strings.TrimPrefix(version[2], "Copyright"))
				} else {
					fmt.Printf("✅ %s\n", ffmpegPath)
				}
			} else {
				fmt.Println("❌ 未安装")
				fmt.Println("   💡 安装: brew install ffmpeg")
			}

			// 2. yt-dlp
			fmt.Print("[2/4] yt-dlp... ")
			ytdlpPath, _ := exec.LookPath("yt-dlp")
			if ytdlpPath != "" {
				out, _ := exec.Command("yt-dlp", "--version").Output()
				fmt.Printf("✅ %s (%s)", ytdlpPath, strings.TrimSpace(string(out)))
				if updateFlag {
					fmt.Print(" → 更新中... ")
					if upd, err := exec.Command("yt-dlp", "-U").CombinedOutput(); err == nil {
						fmt.Printf("%s", strings.TrimSpace(string(upd)))
					} else {
						fmt.Print("❌ " + strings.TrimSpace(string(upd)))
					}
				}
				fmt.Println()
			} else {
				fmt.Println("❌ 未安装")
				fmt.Println("   💡 安装: brew install yt-dlp")
			}

			// 3. Python impersonation
			fmt.Print("[3/4] Python impersonation... ")
			pythonPath, _ := exec.LookPath("python3")
			if pythonPath == "" {
				pythonPath, _ = exec.LookPath("python")
			}
			if pythonPath != "" {
				out, _ := exec.Command(pythonPath, "-c", "import requests; print('ok')").CombinedOutput()
				if strings.TrimSpace(string(out)) == "ok" {
					fmt.Println("✅ requests 已安装")
				} else if pipFlag {
					fmt.Println("⚠️  安装中...")
					if install, err := exec.Command(pythonPath, "-m", "pip", "install", "yt-dlp[default,curl-cffi]").CombinedOutput(); err == nil {
						fmt.Printf("   ✅ %s\n", strings.TrimSpace(string(install)))
					} else {
						fmt.Printf("   ❌ %s\n", strings.TrimSpace(string(install)))
					}
				} else {
					fmt.Println("⚠️  缺少 requests")
					fmt.Println("   💡 运行: ytb init --pip 自动安装")
				}
			} else {
				fmt.Println("❌ 未找到 Python3")
			}

			// 4. deno
			fmt.Print("[4/4] deno... ")
			denoPath, _ := exec.LookPath("deno")
			if denoPath != "" {
				out, _ := exec.Command("deno", "--version").Output()
				firstLine := strings.SplitN(string(out), "\n", 2)[0]
				fmt.Printf("✅ %s\n", firstLine)
			} else {
				fmt.Println("⚠️  未安装（可选）")
				fmt.Println("   💡 安装: brew install deno")
			}

			fmt.Println()
			fmt.Println(strings.Repeat("=", 40))
			return nil
		},
	}
	cmd.Flags().Bool("update", false, "更新 yt-dlp 到最新版")
	cmd.Flags().Bool("pip", false, "自动安装 Python 依赖")
	return cmd
}
