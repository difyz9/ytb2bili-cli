package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zolagz/ytb2bili-go/internal/download"
	"github.com/zolagz/ytb2bili-go/internal/resource"
)

func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "检查并修复环境依赖",
		Long: `检查系统环境依赖并自动修复：
  - yt-dlp：检查安装状态，可选更新到最新版
  - ffmpeg：检查是否可用
  - Python 依赖：安装 requests（yt-dlp impersonation 支持）
  - deno：检查是否安装（可选）
  - whisper.cpp：检查 whisper-cli 与模型（可选，转录 provider=whisper 时必需）`,
		RunE: func(cmd *cobra.Command, args []string) error {
			updateFlag, _ := cmd.Flags().GetBool("update")
			pipFlag, _ := cmd.Flags().GetBool("pip")
			venvFlag, _ := cmd.Flags().GetBool("venv")

			fmt.Println("🩺 环境依赖检查")
			fmt.Println(strings.Repeat("=", 40))

			// 1. ffmpeg
			fmt.Print("\n[1/6] ffmpeg... ")
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

			// 2. yt-dlp（PATH + ~/.local/bin 自动安装位置）
			fmt.Print("[2/6] yt-dlp... ")
			ytdlpPath := download.YTDLPPath()
			if ytdlpPath != "" {
				out, _ := exec.Command(ytdlpPath, "--version").Output()
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
				fmt.Println("   💡 下载/投稿步骤会自动安装到 ~/.local/bin（无需 sudo）；或手动: brew install yt-dlp")
			}

			// 3. Python impersonation
			fmt.Print("[3/6] Python impersonation... ")
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
			fmt.Print("[4/6] deno... ")
			denoPath, _ := exec.LookPath("deno")
			if denoPath != "" {
				out, _ := exec.Command("deno", "--version").Output()
				firstLine := strings.SplitN(string(out), "\n", 2)[0]
				fmt.Printf("✅ %s\n", firstLine)
			} else {
				fmt.Println("⚠️  未安装（可选）")
				fmt.Println("   💡 安装: brew install deno")
			}

			// 5. audio-video-sync .venv
			fmt.Print("[5/6] audio-video-sync 配音环境... ")
			projectRoot := resource.ProjectRoot()
			venvDir := filepath.Join(projectRoot, ".venv")
			venvPython := filepath.Join(venvDir, "bin", "python3")
			requirements := resource.SkillScript(filepath.Join("audio-video-sync", "requirements.txt"))
			if _, err := os.Stat(venvPython); err != nil {
				fmt.Println("❌ 未创建 .venv")
				if venvFlag {
					fmt.Print("   ⏳ 创建中... ")
					if out, perr := exec.Command("python3", "-m", "venv", venvDir).CombinedOutput(); perr != nil {
						fmt.Printf("❌ %s\n", strings.TrimSpace(string(out)))
					} else if o, ierr := exec.Command(venvPython, "-m", "pip", "install", "-q", "-r", requirements).CombinedOutput(); ierr != nil {
						fmt.Printf("⚠️  pip 安装失败: %s\n", strings.TrimSpace(string(o)))
					} else {
						fmt.Println("✅ 已创建 .venv 并安装依赖")
					}
				} else {
					fmt.Println("   💡 运行: ytb init --venv 自动创建，或")
					fmt.Printf("   python3 -m venv %s && %s -m pip install -r %s\n", venvDir, venvPython, requirements)
				}
			} else {
				out, _ := exec.Command(venvPython, "-c", "import pydub, pysrt; print('ok')").CombinedOutput()
				if strings.TrimSpace(string(out)) == "ok" {
					fmt.Println("✅ .venv 已就绪 (pydub + pysrt)")
				} else {
					fmt.Println("⚠️  .venv 存在但缺少依赖 (pydub/pysrt)")
					fmt.Printf("   💡 运行: %s -m pip install -r %s\n", venvPython, requirements)
				}
			}

			// 6. whisper.cpp 本地转录
			fmt.Print("[6/6] whisper.cpp 转录... ")
			whisperPath, _ := exec.LookPath("whisper-cli")
			if whisperPath == "" {
				fmt.Println("❌ 未安装 whisper-cli")
				fmt.Println("   💡 安装: brew install whisper-cpp（Debian/Ubuntu: sudo apt install whisper-cpp）")
				fmt.Println("   💡 或配置 transcriber.provider: bcut 使用云转录")
			} else {
				model := "models/ggml-base.bin"
				if cfg := loadConfig(); cfg.Transcriber != nil && cfg.Transcriber.Whisper != nil && cfg.Transcriber.Whisper.Model != "" {
					model = cfg.Transcriber.Whisper.Model
				}
				if _, err := os.Stat(model); err != nil {
					fmt.Printf("⚠️  %s（模型缺失）\n", whisperPath)
					fmt.Printf("   💡 下载: curl -L -o %s https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-base.bin\n", model)
				} else {
					fmt.Printf("✅ %s (模型: %s)\n", whisperPath, model)
				}
			}

			fmt.Println()
			fmt.Println(strings.Repeat("=", 40))
			return nil
		},
	}
	cmd.Flags().Bool("update", false, "更新 yt-dlp 到最新版")
	cmd.Flags().Bool("pip", false, "自动安装 Python 依赖")
	cmd.Flags().Bool("venv", false, "自动创建 audio-video-sync .venv 并安装依赖")
	return cmd
}
