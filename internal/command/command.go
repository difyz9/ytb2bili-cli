package command

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/urfave/cli/v2"

	"github.com/zolagz/ytb2bili-go/internal/auth"
	"github.com/zolagz/ytb2bili-go/internal/bili"
	"github.com/zolagz/ytb2bili-go/internal/cdp"
	"github.com/zolagz/ytb2bili-go/internal/channel"
	"github.com/zolagz/ytb2bili-go/internal/config"
	"github.com/zolagz/ytb2bili-go/internal/download"
	"github.com/zolagz/ytb2bili-go/internal/metadata"
	"github.com/zolagz/ytb2bili-go/internal/pipeline"
	"github.com/zolagz/ytb2bili-go/internal/queue"
	"github.com/zolagz/ytb2bili-go/internal/search"
	"github.com/zolagz/ytb2bili-go/internal/server"
	"github.com/zolagz/ytb2bili-go/internal/storage"
	"github.com/zolagz/ytb2bili-go/internal/transcriber"
	"github.com/zolagz/ytb2bili-go/internal/translator"
	"github.com/zolagz/ytb2bili-go/internal/workflow"
)

func NewApp(cfg *config.Config) *cli.App {
	return &cli.App{
		Name:     "ytb2bili",
		Usage:    "YouTube → Bilibili 视频搬运工具",
		Version:  "dev",
		Commands: commands(cfg),
	}
}

func commands(cfg *config.Config) []*cli.Command {
	return []*cli.Command{
		startCommand(cfg),
		stopCommand(cfg),
		restartCommand(cfg),
		statusCommand(cfg),
		loginCommand(cfg),
		searchCommand(cfg),
		submitCommand(cfg),
		taskCommand(cfg),
		channelCommand(cfg),
		queueCommand(cfg),
		serverCommand(cfg), // 保留但 Hidden=true
		debugCommand(cfg),
		subtitleCommand(cfg),
		autoCommand(cfg),
		cookiesCommand(cfg),
	}
}

// ─── Search ─────────────────────────────────────────────────────────────────

func searchCommand(cfg *config.Config) *cli.Command {
	return &cli.Command{
		Name:  "search",
		Usage: "搜索 YouTube 视频",
		Flags: []cli.Flag{
			&cli.IntFlag{Name: "max", Value: 10, Usage: "最大结果数"},
			&cli.BoolFlag{Name: "json", Usage: "输出 JSON 格式"},
			&cli.StringFlag{Name: "sort", Usage: "排序方式: relevance, upload_date, view_count, rating"},
			&cli.StringFlag{Name: "date", Usage: "上传日期: last_hour, today, this_week, this_month, this_year"},
			&cli.StringFlag{Name: "duration", Usage: "时长过滤: short(<4m), medium(4-20m), long(>20m)"},
			&cli.StringFlag{Name: "type", Usage: "类型: video, channel, playlist, movie"},
			&cli.StringSliceFlag{Name: "features", Usage: "功能过滤: live, 4k, hd, subtitles, cc"},
			&cli.StringFlag{Name: "submit", Usage: "直接提交指定序号的视频 (如: --submit 1)"},
			&cli.BoolFlag{Name: "skip-translate", Usage: "提交时跳过翻译"},
			&cli.BoolFlag{Name: "dry-run", Usage: "提交时仅处理不上传"},
			&cli.BoolFlag{Name: "history", Usage: "显示已提交的历史记录"},
		},
		Action: func(c *cli.Context) error {
			// 显示历史记录
			if c.Bool("history") {
				historyDir := filepath.Join(cfg.DataDir, "history")
				history := storage.NewHistoryStore(historyDir)
				videos, err := history.List()
				if err != nil || len(videos) == 0 {
					fmt.Println("📭 暂无提交记录")
					return nil
				}
				fmt.Printf("📋 已提交 %d 个视频:\n\n", len(videos))
				for i, v := range videos {
					fmt.Printf("%d. %s\n", i+1, v.Title)
					fmt.Printf("   YouTube: https://www.youtube.com/watch?v=%s\n", v.YouTubeID)
					fmt.Printf("   B站: https://www.bilibili.com/video/%s\n", v.BVID)
					fmt.Printf("   提交时间: %s\n\n", v.SubmittedAt[:19])
				}
				return nil
			}

			query := strings.Join(c.Args().Slice(), " ")
			if query == "" {
				return fmt.Errorf("请提供搜索关键词")
			}

			searcher := search.New(c.Int("max"))

			// 构建过滤器
			filter := &search.SearchFilter{
				SortBy:     c.String("sort"),
				UploadDate: c.String("date"),
				Type:       c.String("type"),
				Duration:   c.String("duration"),
				Features:   c.StringSlice("features"),
			}

			result, err := searcher.SearchPaginated(query, filter, "")
			if err != nil {
				return fmt.Errorf("搜索失败: %w", err)
			}

			if c.Bool("json") {
				data, _ := json.MarshalIndent(result, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			if len(result.Videos) == 0 {
				fmt.Println("未找到结果")
				return nil
			}

			// 加载历史记录
			historyDir := filepath.Join(cfg.DataDir, "history")
			history := storage.NewHistoryStore(historyDir)

			fmt.Printf("🔍 搜索: %s\n", query)
			fmt.Printf("📺 找到 %d 个视频:\n\n", len(result.Videos))

			for i, v := range result.Videos {
				status := ""
				if history.IsSubmitted(v.ID) {
					status = " ✅已提交"
				}
				fmt.Printf("%d. %s%s\n", i+1, v.Title, status)
				fmt.Printf("   频道: %s\n", v.Channel)
				fmt.Printf("   时长: %s | 观看: %s | %s\n", v.Duration, v.Views, v.PublishTime)
				fmt.Printf("   链接: %s\n\n", v.URL)
			}

			// 处理提交请求
			if submitIdx := c.String("submit"); submitIdx != "" {
				var idx int
				if _, err := fmt.Sscanf(submitIdx, "%d", &idx); err != nil || idx < 1 || idx > len(result.Videos) {
					return fmt.Errorf("无效的序号: %s (有效范围: 1-%d)", submitIdx, len(result.Videos))
				}

				video := result.Videos[idx-1]

				// 检查是否已提交
				if history.IsSubmitted(video.ID) {
					submitted := history.GetSubmitted(video.ID)
					fmt.Printf("⚠️  该视频已提交过:\n")
					fmt.Printf("   B站链接: https://www.bilibili.com/video/%s\n", submitted.BVID)
					fmt.Printf("   提交时间: %s\n", submitted.SubmittedAt[:19])
					return nil
				}

				fmt.Printf("🚀 开始提交视频: %s\n", video.Title)
				fmt.Printf("   YouTube: %s\n", video.URL)

				// 执行提交
				url := video.URL
				taskDir := filepath.Join(cfg.DataDir, "tasks")
				credDir := filepath.Join(cfg.DataDir, "cookies")
				ts := storage.NewTaskStore(taskDir)
				cs := storage.NewCredentialStore(credDir)

				task := ts.Create(url)
				id := task.ID
				fmt.Printf("\n📋 任务 %s: %s\n", id, url)

				totalStart := time.Now()

				// Step 1: Download
				ts.UpdateStep(id, "download", "running")
				fmt.Print("\n⬇️ [1/5] 下载视频... ")
				dlDir := filepath.Join(cfg.DataDir, "downloads", id)
				cookiesPath := cfg.YouTubeCookies
				if cookiesPath == "" {
					cookiesPath = filepath.Join(cfg.DataDir, "youtube_cookies.txt")
				}
				dlResult, err := download.Video(url, dlDir, "en", cookiesPath)
				if err != nil {
					ts.UpdateStep(id, "download", "failed", err.Error())
					return fmt.Errorf("下载失败: %w", err)
				}
				ts.UpdateStep(id, "download", "completed")
				fmt.Printf("✅ %s\n", filepath.Base(dlResult.VideoPath))

				// Step 2: Transcribe
				ts.UpdateStep(id, "transcribe", "running")
				srtPath := dlResult.SubtitlePath
				if srtPath == "" {
					fmt.Print("🎙️ [2/5] Bcut 语音转字幕... ")
					srtPath, err = transcriber.BcutASR(dlResult.VideoPath, dlDir, id)
					if err != nil {
						ts.UpdateStep(id, "transcribe", "failed", err.Error())
						return fmt.Errorf("转写失败: %w", err)
					}
					fmt.Println("✅")
				} else {
					fmt.Printf("📝 [2/5] 已有字幕: %s\n", filepath.Base(srtPath))
				}
				ts.UpdateStep(id, "transcribe", "completed")

				// Step 3: Translate
				if !c.Bool("skip-translate") {
					ts.UpdateStep(id, "translate", "running")
					fmt.Printf("🌐 [3/5] AI 翻译 (en→zh)... ")
					_, err = translator.SRT(srtPath, "en", "zh", cfg)
					if err != nil {
						ts.UpdateStep(id, "translate", "failed", err.Error())
						return fmt.Errorf("翻译失败: %w", err)
					}
					fmt.Println("✅")
					ts.UpdateStep(id, "translate", "completed")
				}

				// Step 4: Metadata
				ts.UpdateStep(id, "metadata", "running")
				fmt.Print("🤖 [4/5] AI 生成元数据... ")
				meta, err := metadata.Generate(dlResult.Info, cfg)
				if err != nil {
					meta = &metadata.VideoMeta{Title: dlResult.Info.Title}
					fmt.Println("⚠ (回退到原始标题)")
				} else {
					fmt.Printf("✅ %s\n", meta.Title)
				}
				ts.UpdateStep(id, "metadata", "completed")
				task.Title = meta.Title

				// Step 5: Upload
				if c.Bool("dry-run") {
					fmt.Print("⏭️ [5/5] 跳过上传 (--dry-run)\n")
					task.Status = "completed"
					elapsed := time.Since(totalStart).Seconds()
					fmt.Printf("\n✨ 处理完成! 耗时: %.0fs\n", elapsed)
					return nil
				}

				ts.UpdateStep(id, "upload", "running")
				fmt.Print("📤 [5/5] 上传到 B站... ")

				var cred auth.LoginInfo
				if err := cs.Load(&cred); err != nil {
					ts.UpdateStep(id, "upload", "failed", "未登录")
					return fmt.Errorf("请先登录: y2b login")
				}

				bvid, err := bili.Upload(&cred, &bili.UploadParams{
					VideoPath: dlResult.VideoPath,
					Title:     meta.Title,
					Desc:      meta.Description,
					Tags:      meta.Tags,
					Source:    url,
					Tid:       122,
					CoverPath: dlResult.CoverPath,
				})
				if err != nil {
					ts.UpdateStep(id, "upload", "failed", err.Error())
					return fmt.Errorf("上传失败: %w", err)
				}

				task.BVID = bvid
				task.Status = "completed"
				ts.SetBVID(id, bvid)
				ts.SetCompleted(id)
				fmt.Printf("✅ https://www.bilibili.com/video/%s\n", bvid)

				// 记录到历史
				history.Add(&storage.SubmittedVideo{
					YouTubeID: video.ID,
					BVID:      bvid,
					Title:     video.Title,
					Channel:   video.Channel,
				})

				// 同步字幕追踪状态
				subStore := storage.NewSubtitleStore(filepath.Join(cfg.DataDir, "subtitles"))
				tracks, _ := subStore.SyncFromDownload(id, bvid, dlDir)
				pendingCount := 0
				for _, t := range tracks {
					if t.Status == storage.SubtitleStatusPending {
						pendingCount++
					}
				}
				if pendingCount > 0 {
					fmt.Printf("  📝 找到 %d 个字幕文件待上传\n", pendingCount)
					fmt.Printf("  💡 审核通过后执行: ytb subtitle retry %s\n", bvid)
				}

				elapsed := time.Since(totalStart).Seconds()
				fmt.Printf("\n✨ 总耗时: %.0fs\n", elapsed)
			}

			return nil
		},
	}
}

// ─── Server helpers ──────────────────────────────────────────────────────────

// startServer exec 自身以 server 子命令后台运行
func startServer(cfg *config.Config, addr string, port int) error {
	if port != 8096 {
		addr = fmt.Sprintf("127.0.0.1:%d", port)
	}

	pidFile := filepath.Join(cfg.DataDir, "server.pid")
	logFile := filepath.Join(cfg.DataDir, "server.log")

	// 检查是否已在运行
	if pidData, err := os.ReadFile(pidFile); err == nil {
		var oldPid int
		fmt.Sscanf(string(pidData), "%d", &oldPid)
		if proc, err := os.FindProcess(oldPid); err == nil {
			if err := proc.Signal(syscall.Signal(0)); err == nil {
				return fmt.Errorf("⚠️  服务已在运行 (PID: %d)\n  查看日志: tail -f %s\n  停止服务: %s stop", oldPid, logFile, os.Args[0])
			}
		}
	}

	os.MkdirAll(cfg.DataDir, 0755)

	selfPath, _ := os.Executable()
	cmd := exec.Command(selfPath, "server", "--addr", addr)
	cmd.Env = os.Environ()
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	logF, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("无法创建日志文件: %w", err)
	}
	defer logF.Close()
	cmd.Stderr = logF

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动失败: %w", err)
	}

	pid := cmd.Process.Pid
	os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", pid)), 0644)

	fmt.Printf("🚀 ytb2bili HTTP 服务已启动\n")
	fmt.Printf("   地址: http://localhost%s\n", addr)
	fmt.Printf("   PID: %d\n", pid)
	fmt.Printf("   日志: %s\n", logFile)
	fmt.Printf("\n📡 可用命令:\n")
	fmt.Printf("   tail -f %s  查看实时日志\n", logFile)
	fmt.Printf("   %s stop              停止服务\n", filepath.Base(selfPath))

	return nil
}

// stopServer 停止后台服务，返回 PID
func stopServer(cfg *config.Config) (int, error) {
	pidFile := filepath.Join(cfg.DataDir, "server.pid")
	pidData, err := os.ReadFile(pidFile)
	if err != nil {
		return 0, fmt.Errorf("未找到运行中的服务 (PID 文件不存在)")
	}

	var pid int
	fmt.Sscanf(string(pidData), "%d", &pid)

	proc, err := os.FindProcess(pid)
	if err != nil {
		os.Remove(pidFile)
		return 0, fmt.Errorf("无法找到进程 %d (可能已结束)", pid)
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		os.Remove(pidFile)
		return 0, fmt.Errorf("停止失败: %w", err)
	}

	time.Sleep(500 * time.Millisecond)
	os.Remove(pidFile)
	return pid, nil
}

// serverStatus 检查服务运行状态
func serverStatus(cfg *config.Config) (pid int, running bool, logFile string) {
	pidFile := filepath.Join(cfg.DataDir, "server.pid")
	logFile = filepath.Join(cfg.DataDir, "server.log")

	pidData, err := os.ReadFile(pidFile)
	if err != nil {
		return 0, false, logFile
	}

	fmt.Sscanf(string(pidData), "%d", &pid)

	proc, err := os.FindProcess(pid)
	if err != nil {
		return pid, false, logFile
	}

	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return pid, false, logFile
	}

	return pid, true, logFile
}

// ─── Start ──────────────────────────────────────────────────────────────────

func startCommand(cfg *config.Config) *cli.Command {
	return &cli.Command{
		Name:  "start",
		Usage: "以后台守护进程方式启动 HTTP API 服务器",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "addr", Value: "127.0.0.1:8096", Usage: "监听地址"},
			&cli.IntFlag{Name: "port", Value: 8096, Usage: "监听端口 (覆盖 addr)"},
		},
		Action: func(c *cli.Context) error {
			return startServer(cfg, c.String("addr"), c.Int("port"))
		},
	}
}

// ─── Stop ───────────────────────────────────────────────────────────────────

func stopCommand(cfg *config.Config) *cli.Command {
	return &cli.Command{
		Name:  "stop",
		Usage: "停止后台 HTTP API 服务器",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "force", Aliases: []string{"f"}, Usage: "强制终止"},
		},
		Action: func(c *cli.Context) error {
			if c.Bool("force") {
				// 强制终止：读 PID 文件直接 kill
				pidFile := filepath.Join(cfg.DataDir, "server.pid")
				pidData, err := os.ReadFile(pidFile)
				if err != nil {
					return fmt.Errorf("❌ 未找到运行中的服务 (PID 文件不存在)")
				}
				var pid int
				fmt.Sscanf(string(pidData), "%d", &pid)
				proc, err := os.FindProcess(pid)
				if err != nil {
					os.Remove(pidFile)
					return fmt.Errorf("❌ 无法找到进程 %d (可能已结束)", pid)
				}
				if err := proc.Signal(syscall.SIGKILL); err != nil {
					os.Remove(pidFile)
					return fmt.Errorf("❌ 强制停止失败: %w", err)
				}
				time.Sleep(500 * time.Millisecond)
				os.Remove(pidFile)
				fmt.Printf("✅ 服务已强制终止 (PID: %d)\n", pid)
				return nil
			}

			pid, err := stopServer(cfg)
			if err != nil {
				return fmt.Errorf("❌ %v", err)
			}
			fmt.Printf("✅ 服务已停止 (PID: %d)\n", pid)
			return nil
		},
	}
}

// ─── Restart ─────────────────────────────────────────────────────────────────

func restartCommand(cfg *config.Config) *cli.Command {
	return &cli.Command{
		Name:  "restart",
		Usage: "重启 HTTP API 服务器",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "addr", Value: "127.0.0.1:8096", Usage: "监听地址"},
			&cli.IntFlag{Name: "port", Value: 8096, Usage: "监听端口 (覆盖 addr)"},
		},
		Action: func(c *cli.Context) error {
			// 先停止（存在则停，不存在则跳过）
			if pid, err := stopServer(cfg); err == nil {
				fmt.Printf("✅ 服务已停止 (PID: %d)\n", pid)
			} else {
				fmt.Printf("ℹ️  %v\n", err)
			}
			fmt.Println()
			// 再启动
			return startServer(cfg, c.String("addr"), c.Int("port"))
		},
	}
}

// ─── Status ──────────────────────────────────────────────────────────────────

func statusCommand(cfg *config.Config) *cli.Command {
	return &cli.Command{
		Name:  "status",
		Usage: "查看 HTTP API 服务器运行状态",
		Action: func(c *cli.Context) error {
			pid, running, logFile := serverStatus(cfg)
			if !running {
				if pid > 0 {
					fmt.Printf("❌ 服务未运行 (PID 文件残留: %d)\n", pid)
					fmt.Printf("   执行 %s stop 清理\n", os.Args[0])
				} else {
					fmt.Println("❌ 服务未运行")
				}
				return nil
			}

			// 获取进程信息
			proc, _ := os.FindProcess(pid)
			_ = proc

			fmt.Println("✅ 服务正在运行")
			fmt.Printf("   PID:   %d\n", pid)
			fmt.Printf("   日志:  %s\n", logFile)

			// 读取日志尾部
			if data, err := os.ReadFile(logFile); err == nil {
				lines := strings.Split(strings.TrimSpace(string(data)), "\n")
				n := len(lines)
				if n > 5 {
					lines = lines[n-5:]
				}
				fmt.Printf("\n📋 最近 %d 行日志:\n", len(lines))
				for _, l := range lines {
					if l != "" {
						fmt.Printf("   %s\n", l)
					}
				}
			}

			fmt.Printf("\n💡 命令提示:\n")
			fmt.Printf("   tail -f %s     查看实时日志\n", logFile)
			fmt.Printf("   %s stop                   停止服务\n", filepath.Base(os.Args[0]))
			fmt.Printf("   %s restart                重启服务\n", filepath.Base(os.Args[0]))
			return nil
		},
	}
}

// ─── Login ──────────────────────────────────────────────────────────────────

func loginCommand(cfg *config.Config) *cli.Command {
	return &cli.Command{
		Name:  "login",
		Usage: "B站扫码登录（默认 CDP 驱动 Chrome）",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "password", Usage: "使用密码登录"},
			&cli.StringFlag{Name: "username", Usage: "B站账号"},
			&cli.StringFlag{Name: "password-val", Usage: "B站密码"},
			&cli.BoolFlag{Name: "no-browser", Usage: "不打开浏览器（仅保存二维码文件）"},
			&cli.IntFlag{Name: "cdp-port", Value: 9222, Usage: "CDP 调试端口"},
		},
		Action: func(c *cli.Context) error {
			credDir := filepath.Join(cfg.DataDir, "cookies")
			store := storage.NewCredentialStore(credDir)

			if c.Bool("password") {
				user := c.String("username")
				pass := c.String("password-val")
				if user == "" || pass == "" {
					return fmt.Errorf("密码登录需要 --username 和 --password-val")
				}
				return fmt.Errorf("密码登录待实现")
			}

			// QR code login
			fmt.Println("📱 获取二维码...")
			qr, err := auth.GetQRCode()
			if err != nil {
				return err
			}

			// Generate QR code image + terminal display
			qrPath := filepath.Join(credDir, "bilibili_qrcode.png")
			if err := SaveQRCode(qr.URL, qrPath); err != nil {
				fmt.Fprintf(os.Stderr, "⚠ 无法生成二维码图片: %v\n", err)
			}
			// 终端二维码（兼容 Hermes+飞书：飞书忽略 ANSI，终端显示二维码）
			PrintQRCodeTerminal(qr.URL)

			// ── CDP 默认集成 — CDP → 系统打开 → 图片查看器 ──
			browserOpened := false
			cdpPort := c.Int("cdp-port")

			if !c.Bool("no-browser") {
				// 方式1: CDP — Connect() 自动处理已有/启动 Chrome
				cm := cdp.NewChromeManager(
					cdp.WithPort(cdpPort),
					cdp.WithUserDataDir(filepath.Join(cfg.DataDir, "browser_data")),
				)
				cdp.RegisterCleanup(cm)
				if ctx, cancel, err := cm.Connect(); err == nil {
					defer cancel()
					if err := cdp.OpenURL(ctx, qr.URL); err == nil {
						fmt.Fprintf(os.Stderr, "🌐 已在 Chrome 中打开扫码页面\n")
						browserOpened = true
					}
				}

				// 方式2: 系统命令打开浏览器
				if !browserOpened {
					if err := cdp.OpenURLSystem(qr.URL); err == nil {
						browserOpened = true
						fmt.Fprintf(os.Stderr, "🌐 已在浏览器中打开二维码页面\n")
					}
				}

				// 方式3: 系统图片查看器
				if !browserOpened {
					if err := cdp.ShowImageSystem(qrPath); err == nil {
						browserOpened = true
						fmt.Fprintf(os.Stderr, "🖼️ 已打开二维码图片\n")
					}
				}

				if !browserOpened {
					fmt.Fprintf(os.Stderr, "💡 请手动打开二维码图片: %s\n", qrPath)
				}
			}

			// Output paths for Hermes detection (stderr avoids pipe buffering)
			fmt.Fprintf(os.Stderr, "QRCODE_IMAGE:%s\n", qrPath)
			fmt.Fprintf(os.Stderr, "QRCODE_URL:%s\n", qr.URL)
			fmt.Fprintln(os.Stderr, "⏳ 等待扫码...（最长120秒）")

			// Also write auth_code to file for agent polling
			authFile := filepath.Join(credDir, "auth_code.txt")
			os.WriteFile(authFile, []byte(qr.AuthCode), 0644)

			cred, err := auth.PollQRCode(qr.AuthCode, 120*time.Second)
			if err != nil {
				return fmt.Errorf("登录失败: %w", err)
			}

			store.Save(cred)
			fmt.Println("✅ 扫码成功!")

			info, _ := auth.GetUserInfo(cred)
			if name, ok := info["name"].(string); ok {
				fmt.Printf("👤 用户: %s (UID: %.0f)\n", name, info["mid"])
			}
			return nil
		},
	}
}

// ─── Submit ─────────────────────────────────────────────────────────────────

func submitCommand(cfg *config.Config) *cli.Command {
	return &cli.Command{
		Name:      "submit",
		Usage:     "提交搬运任务",
		ArgsUsage: "<YouTube URL>",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "source-lang", Value: "en", Usage: "源语言"},
			&cli.StringFlag{Name: "target-lang", Value: "zh", Usage: "目标语言"},
			&cli.IntFlag{Name: "tid", Value: cfg.BiliTid, Usage: "B站分区ID"},
			&cli.BoolFlag{Name: "dry-run", Usage: "仅处理不上传"},
			&cli.BoolFlag{Name: "skip-translate", Usage: "跳过翻译"},
			&cli.StringFlag{Name: "chain", Usage: "自定义任务链，如 download,transcribe,translate（依赖会自动补全）"},
			&cli.BoolFlag{Name: "show-plan", Usage: "只显示规划后的任务链，不执行"},
			&cli.StringFlag{Name: "planner", Value: "adaptive", Usage: "规划器: adaptive 或 agent"},
			&cli.StringFlag{Name: "goal", Usage: "交给 Agent 的自然语言任务目标"},
			&cli.StringFlag{Name: "audio-dir", Usage: "按字幕序号命名的分段配音目录（audio-sync 步骤必需）"},
			&cli.BoolFlag{Name: "no-audio-speed-adjust", Usage: "音画同步时禁用智能调速，仅按时间轴填充"},
			&cli.StringFlag{Name: "audio-missing", Value: "error", Usage: "缺失配音处理: error 或 silence"},
		},
		Action: func(c *cli.Context) error {
			url := c.Args().First()
			if url == "" {
				return fmt.Errorf("请输入 YouTube URL")
			}

			opts := &submitOptions{
				SourceLang:    c.String("source-lang"),
				TargetLang:    c.String("target-lang"),
				Tid:           c.Int("tid"),
				DryRun:        c.Bool("dry-run"),
				SkipTranslate: c.Bool("skip-translate"),
				Source:        "manual",
				Chain:         workflow.ParseChain(c.String("chain")),
				ShowPlan:      c.Bool("show-plan"),
			}

			processor := &pipeline.Processor{Config: cfg, Reporter: func(event pipeline.Event) {
				if event.Status == "running" {
					fmt.Printf("\n[%d/%d] %s... ", event.Position, event.Total, event.Step)
				} else if event.Err != nil {
					fmt.Printf("❌ %v\n", event.Err)
				} else {
					fmt.Println("✅")
				}
			}}
			result, err := processor.Process(c.Context, pipeline.Request{
				URL: url, SourceLang: opts.SourceLang, TargetLang: opts.TargetLang,
				Tid: opts.Tid, DryRun: opts.DryRun, SkipTranslate: opts.SkipTranslate,
				Source: opts.Source, Chain: opts.Chain, PlanOnly: opts.ShowPlan,
				Planner: c.String("planner"), Goal: c.String("goal"),
				AudioDir: c.String("audio-dir"), DisableAudioSpeedAdjust: c.Bool("no-audio-speed-adjust"),
				AudioMissingMode: c.String("audio-missing"),
			})
			if result != nil {
				fmt.Printf("🔗 任务链: %s\n", strings.Join(result.Plan, " → "))
				if result.BVID != "" {
					fmt.Printf("📺 https://www.bilibili.com/video/%s\n", result.BVID)
				}
			}
			return err
		},
	}
}

// watchAndUploadSubtitle 异步监听B站视频审核状态，审核通过后上传字幕
// videoID: 任务/下载目录 ID; dlDir: 字幕文件所在目录
func watchAndUploadSubtitle(bvid, videoID, dlDir string, cred *auth.LoginInfo, cfg *config.Config) {
	subStore := storage.NewSubtitleStore(filepath.Join(cfg.DataDir, "subtitles"))

	// 同步最新状态
	subStore.SyncFromDownload(videoID, bvid, dlDir)
	pending := subStore.GetPending(videoID)
	if len(pending) == 0 {
		return
	}

	fmt.Printf("\n⏳ [字幕] 监听视频 %s 审核状态 (共 %d 个字幕待上传)...\n", bvid, len(pending))

	// 等待审核通过
	status, err := bili.WaitForReviewPassed(cred, bvid)
	if err != nil {
		fmt.Printf("❌ [字幕] 等待审核失败: %v\n", err)
		return
	}
	if status == nil {
		fmt.Printf("❌ [字幕] 获取审核状态失败\n")
		return
	}
	fmt.Printf("✅ [字幕] 视频审核通过 (state=%d)\n", status.State)

	// 重新同步（字幕文件可能已更新）
	subStore.SyncFromDownload(videoID, bvid, dlDir)
	pending = subStore.GetPending(videoID)
	if len(pending) == 0 {
		fmt.Printf("ℹ️ [字幕] 没有待上传的字幕文件\n")
		return
	}

	successCount := 0
	for _, track := range pending {
		fmt.Printf("  📤 上传字幕: %s (%s)... ", track.FileName, track.Language)

		// 使用 SubtitleUploader 上传字幕
		err := bili.UploadSubtitle(cred, bvid, track.FilePath, track.Language)
		if err != nil {
			fmt.Printf("❌ %v\n", err)
			subStore.MarkFailed(videoID, track.Language, err.Error())
			continue
		}

		subStore.MarkUploaded(videoID, track.Language)
		fmt.Println("✅")
		successCount++
	}

	if successCount > 0 {
		allDone := subStore.AllUploaded(videoID)
		if allDone {
			fmt.Printf("✅ [字幕] 全部字幕上传完成! https://www.bilibili.com/video/%s\n", bvid)
		} else {
			fmt.Printf("✅ [字幕] 已上传 %d 个字幕文件，部分仍待处理\n", successCount)
		}
	} else {
		fmt.Printf("❌ [字幕] 所有字幕上传均失败，请稍后重试: ytb2bili subtitle retry %s\n", bvid)
	}
}

// ─── Task Management ────────────────────────────────────────────────────────

func taskCommand(cfg *config.Config) *cli.Command {
	return &cli.Command{
		Name:  "task",
		Usage: "任务管理",
		Subcommands: []*cli.Command{
			{
				Name:  "list",
				Usage: "列出任务",
				Action: func(c *cli.Context) error {
					ts := storage.NewTaskStore(filepath.Join(cfg.DataDir, "tasks"))
					tasks := ts.List()
					if len(tasks) == 0 {
						fmt.Println("暂无任务")
						return nil
					}
					for _, t := range tasks {
						icon := map[string]string{
							"pending": "⏳", "running": "🔄",
							"completed": "✅", "failed": "❌",
						}[t.Status]
						if icon == "" {
							icon = "❓"
						}
						stepsDone := 0
						for _, s := range t.Steps {
							if s.Status == "completed" {
								stepsDone++
							}
						}
						url := t.SourceURL
						if len(url) > 60 {
							url = url[:60] + "..."
						}
						fmt.Printf("%s %s [%d/5] %s %s\n",
							icon, t.ID, stepsDone, url, t.UpdatedAt[:19])
					}
					return nil
				},
			},
			{
				Name:  "show",
				Usage: "查看任务详情",
				Action: func(c *cli.Context) error {
					id := c.Args().First()
					if id == "" {
						return fmt.Errorf("请输入任务ID")
					}
					ts := storage.NewTaskStore(filepath.Join(cfg.DataDir, "tasks"))
					t, err := ts.Get(id)
					if err != nil {
						return fmt.Errorf("任务 %s 不存在", id)
					}
					data, _ := json.MarshalIndent(t, "", "  ")
					fmt.Println(string(data))
					return nil
				},
			},
		},
	}
}

// ─── Channel Monitor ─────────────────────────────────────────────────────────

func channelCommand(cfg *config.Config) *cli.Command {
	monitor := channel.NewMonitor(cfg.DataDir)

	return &cli.Command{
		Name:  "channel",
		Usage: "YouTube 频道监控管理",
		Subcommands: []*cli.Command{
			{
				Name:      "add",
				Usage:     "添加一个 YouTube 频道到监控列表",
				ArgsUsage: "<channel_id>",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "title", Aliases: []string{"t"}, Usage: "频道名称（可选）"},
				},
				Action: func(c *cli.Context) error {
					channelID := c.Args().First()
					if channelID == "" {
						return fmt.Errorf("请输入 YouTube 频道 ID")
					}
					title := c.String("title")
					if title == "" {
						title = channelID
					}
					sub, err := monitor.AddSubscription(channelID, title)
					if err != nil {
						if sub != nil {
							fmt.Printf("ℹ️ %s\n", err)
							return nil
						}
						return err
					}
					fmt.Printf("✅ 已添加频道: %s (%s)\n", sub.ChannelTitle, sub.ChannelID)
					return nil
				},
			},
			{
				Name:  "list",
				Usage: "列出所有监控的频道",
				Action: func(c *cli.Context) error {
					subs := monitor.ListSubscriptions()
					if len(subs) == 0 {
						fmt.Println("📭 暂无频道订阅")
						return nil
					}
					fmt.Printf("📺 共 %d 个频道订阅:\n\n", len(subs))
					for _, s := range subs {
						icon := map[string]string{"active": "🟢", "inactive": "⭕"}[s.Status]
						if icon == "" {
							icon = "❓"
						}
						lastSync := "从未同步"
						if s.LastSyncAt != "" {
							lastSync = s.LastSyncAt[:19]
						}
						fmt.Printf("  %s %s\n", icon, s.ChannelTitle)
						fmt.Printf("     Channel ID: %s\n", s.ChannelID)
						fmt.Printf("     上次同步: %s    状态: %s\n", lastSync, s.Status)
						fmt.Println()
					}
					return nil
				},
			},
			{
				Name:      "remove",
				Usage:     "从监控列表移除一个频道",
				ArgsUsage: "<channel_id>",
				Action: func(c *cli.Context) error {
					channelID := c.Args().First()
					if channelID == "" {
						return fmt.Errorf("请输入 YouTube 频道 ID")
					}
					if err := monitor.RemoveSubscription(channelID); err != nil {
						return err
					}
					fmt.Printf("✅ 已移除频道: %s\n", channelID)
					return nil
				},
			},
			{
				Name:  "sync",
				Usage: "手动同步所有频道的 RSS feed",
				Flags: []cli.Flag{
					&cli.IntFlag{Name: "lookback", Value: 7, Usage: "回看天数（0=不限制）"},
				},
				Action: func(c *cli.Context) error {
					lookback := c.Int("lookback")
					fmt.Printf("🔄 开始同步 %d 个活跃频道 (回看 %d 天)...\n",
						len(monitor.GetActiveSubscriptions()), lookback)

					newCount, err := monitor.SyncAll(lookback, nil)
					if err != nil {
						fmt.Fprintf(os.Stderr, "⚠ %v\n", err)
					}
					fmt.Printf("\n✅ 同步完成! 发现 %d 个新视频\n", newCount)

					stats := monitor.GetStats()
					fmt.Printf("\n📊 监控统计:\n")
					fmt.Printf("   订阅频道: %d (活跃: %d)\n", stats["total_subscriptions"], stats["active_subscriptions"])
					fmt.Printf("   发现视频: %d (待处理: %d, 已提交: %d)\n",
						stats["total_videos"], stats["pending_videos"], stats["submitted_videos"])
					return nil
				},
			},
			{
				Name:  "videos",
				Usage: "列出已发现的视频",
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: "pending", Aliases: []string{"p"}, Usage: "仅显示待处理的视频"},
				},
				Action: func(c *cli.Context) error {
					var videos []channel.DiscoveredVideo
					if c.Bool("pending") {
						videos = monitor.PendingVideos()
					} else {
						videos = monitor.DiscoveredVideos()
					}

					if len(videos) == 0 {
						fmt.Println("📭 暂无视频记录")
						return nil
					}

					fmt.Printf("📺 共 %d 个视频:\n\n", len(videos))
					for _, v := range videos {
						icon := map[string]string{"new": "🆕", "submitted": "✅", "skipped": "⏭️"}[v.Status]
						if icon == "" {
							icon = "❓"
						}
						title := v.Title
						if len(title) > 60 {
							title = title[:60] + "..."
						}
						fmt.Printf("  %s %s\n", icon, title)
						fmt.Printf("     Video ID: %s | 状态: %s\n", v.VideoID, v.Status)
						fmt.Printf("     发布时间: %s\n", v.PublishedAt[:19])
						fmt.Printf("     %s\n\n", v.URL)
					}
					return nil
				},
			},
			{
				Name:  "stats",
				Usage: "显示监控统计信息",
				Action: func(c *cli.Context) error {
					stats := monitor.GetStats()
					fmt.Println("📊 频道监控统计:")
					fmt.Println()
					fmt.Printf("   📺 订阅频道:     %d\n", stats["total_subscriptions"])
					fmt.Printf("   🟢 活跃频道:     %d\n", stats["active_subscriptions"])
					fmt.Printf("   🎬 发现视频:     %d\n", stats["total_videos"])
					fmt.Printf("   🆕 待处理:       %d\n", stats["pending_videos"])
					fmt.Printf("   ✅ 已提交:       %d\n", stats["submitted_videos"])
					return nil
				},
			},
		},
	}
}

// ─── Server ────────────────────────────────────────────────────────────────

func serverCommand(cfg *config.Config) *cli.Command {
	return &cli.Command{
		Name:   "server",
		Usage:  "启动 HTTP API 服务器（前台运行）",
		Hidden: true,
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "addr", Value: "127.0.0.1:8096", Usage: "监听地址"},
			&cli.StringFlag{Name: "feishu-app-id", Usage: "飞书 App ID"},
			&cli.StringFlag{Name: "feishu-app-secret", Usage: "飞书 App Secret"},
			&cli.StringFlag{Name: "feishu-verify-token", Usage: "飞书 Verify Token"},
			&cli.StringFlag{Name: "feishu-encrypt-key", Usage: "飞书 Encrypt Key"},
		},
		Action: func(c *cli.Context) error {
			addr := c.String("addr")
			feishuAppID := c.String("feishu-app-id")
			feishuAppSecret := c.String("feishu-app-secret")

			fmt.Printf("🚀 启动 ytb2bili HTTP 服务器\n")
			fmt.Printf("   地址: %s\n", addr)

			if feishuAppID != "" {
				fmt.Printf("   飞书机器人: 已配置\n")
			}

			fmt.Printf("\n📡 API 端点:\n")
			fmt.Printf("   POST /api/v1/submit     - 提交视频\n")
			fmt.Printf("   GET  /api/v1/tasks      - 查看任务列表\n")
			fmt.Printf("   GET  /api/v1/tasks/:id  - 查看任务详情\n")
			fmt.Printf("   GET  /api/v1/history    - 查看历史记录\n")
			fmt.Printf("   GET  /health            - 健康检查\n")

			if feishuAppID != "" {
				fmt.Printf("   POST /feishu/webhook    - 飞书 Webhook\n")
			}

			fmt.Printf("\n💡 浏览器扩展配置:\n")
			fmt.Printf("   后端地址: http://YOUR_SERVER%s/api/v1/submit\n", addr)

			// 创建飞书机器人
			var feishuBot *server.FeishuBot
			if feishuAppID != "" {
				feishuBot = server.NewFeishuBot(feishuAppID, feishuAppSecret)
				fmt.Printf("   飞书机器人: 已启用\n")
			}

			// 创建服务器
			srv := server.New(cfg)
			srv.Feishu = feishuBot

			// 启动服务器
			return srv.Start(addr)
		},
	}
}

// ─── Debug ────────────────────────────────────────────────────────────────

func debugCommand(cfg *config.Config) *cli.Command {
	return &cli.Command{
		Name:  "debug",
		Usage: "调试模式 - 监听飞书消息并显示详细信息",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "feishu-app-id", Value: cfg.FeishuAppID, Usage: "飞书 App ID"},
			&cli.StringFlag{Name: "feishu-app-secret", Value: cfg.FeishuAppSecret, Usage: "飞书 App Secret"},
			&cli.BoolFlag{Name: "dry-run", Usage: "仅显示消息，不处理"},
		},
		Action: func(c *cli.Context) error {
			feishuAppID := c.String("feishu-app-id")
			feishuAppSecret := c.String("feishu-app-secret")
			dryRun := c.Bool("dry-run")

			fmt.Println("🔍 调试模式 - 飞书消息监听")
			fmt.Println(strings.Repeat("=", 50))
			fmt.Printf("   飞书 App ID: %s\n", feishuAppID)
			fmt.Printf("   模式: %s\n", map[bool]string{true: "仅监听", false: "监听并处理"}[dryRun])
			fmt.Println(strings.Repeat("=", 50))

			// 创建飞书机器人
			feishuBot := server.NewFeishuBot(feishuAppID, feishuAppSecret)

			// 创建调试处理器
			debugger := server.NewDebugger(cfg, feishuBot, dryRun)

			// 启动调试
			return debugger.Start()
		},
	}
}

// ─── Subtitle Management ─────────────────────────────────────────────────────

func subtitleCommand(cfg *config.Config) *cli.Command {
	return &cli.Command{
		Name:  "subtitle",
		Usage: "字幕管理（审核通过后上传）",
		Subcommands: []*cli.Command{
			{
				Name:      "retry",
				Usage:     "重试上传视频的字幕",
				ArgsUsage: "<video_id or bvid>",
				Action: func(c *cli.Context) error {
					ident := c.Args().First()
					if ident == "" {
						return fmt.Errorf("请输入视频ID或BVID")
					}

					// 尝试从字幕存储或任务存储查找
					subStore := storage.NewSubtitleStore(filepath.Join(cfg.DataDir, "subtitles"))
					credDir := filepath.Join(cfg.DataDir, "cookies")
					cs := storage.NewCredentialStore(credDir)

					var cred auth.LoginInfo
					if err := cs.Load(&cred); err != nil {
						return fmt.Errorf("请先登录: %w", err)
					}

					// 如果输入的是 BVID，扫描所有字幕记录找到匹配的
					if strings.HasPrefix(strings.ToUpper(ident), "BV") {
						videoID := ""

						// 先看字幕记录
						entries, _ := os.ReadDir(filepath.Join(cfg.DataDir, "subtitles"))
						for _, e := range entries {
							if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
								continue
							}
							data, _ := os.ReadFile(filepath.Join(cfg.DataDir, "subtitles", e.Name()))
							if data == nil {
								continue
							}
							var tracks []storage.SubtitleTrack
							if json.Unmarshal(data, &tracks) != nil {
								continue
							}
							for _, t := range tracks {
								if t.BVID == ident {
									videoID = t.VideoID
									break
								}
							}
							if videoID != "" {
								break
							}
						}

						// 再查历史记录
						if videoID == "" {
							history := storage.NewHistoryStore(filepath.Join(cfg.DataDir, "history"))
							videos, _ := history.List()
							for _, v := range videos {
								if v.BVID == ident {
									videoID = v.YouTubeID
									break
								}
							}
						}

						if videoID == "" {
							return fmt.Errorf("未找到 BVID %s 的字幕或历史记录", ident)
						}

						// 查找下载目录（可能是 taskID 或 videoID）
						dlDir := filepath.Join(cfg.DataDir, "downloads", videoID)
						if _, err := os.Stat(dlDir); err != nil {
							// 尝试从任务存储查找 task ID
							ts := storage.NewTaskStore(filepath.Join(cfg.DataDir, "tasks"))
							for _, t := range ts.List() {
								// 匹配 BVID 或 source_url 中包含 videoID
								if t.BVID == ident || strings.Contains(t.SourceURL, videoID) {
									dlDir = filepath.Join(cfg.DataDir, "downloads", t.ID)
									videoID = t.ID
									break
								}
							}
						}

						fmt.Printf("📝 准备重试视频 %s 的字幕上传 (videoID=%s)...\n", ident, videoID)
						watchAndUploadSubtitle(ident, videoID, dlDir, &cred, cfg)
						return nil
					}

					// 按 videoID 查找
					dlDir := filepath.Join(cfg.DataDir, "downloads", ident)
					tracks, allDone := subStore.GetStatus(ident)
					if tracks == nil || len(tracks) == 0 {
						// 尝试从下载目录重建
						fmt.Printf("📝 没有找到 %s 的字幕记录，尝试从下载目录重建...\n", ident)
						bvid := ""
						// 从 history 中查找 bvid
						history := storage.NewHistoryStore(filepath.Join(cfg.DataDir, "history"))
						videos, _ := history.List()
						for _, v := range videos {
							if v.YouTubeID == ident {
								bvid = v.BVID
								break
							}
						}
						if bvid == "" {
							return fmt.Errorf("未找到视频 %s 的BVID，请提供BVID参数", ident)
						}
						subStore.SyncFromDownload(ident, bvid, dlDir)
						tracks, _ = subStore.GetStatus(ident)
					}
					if allDone {
						fmt.Printf("✅ 视频 %s 的字幕已全部上传完成\n", ident)
						return nil
					}
					fmt.Printf("📤 开始上传视频 %s 的待处理字幕...\n", ident)
					watchAndUploadSubtitle(tracks[0].BVID, ident, dlDir, &cred, cfg)
					return nil
				},
			},
			{
				Name:      "status",
				Usage:     "查看字幕上传状态",
				ArgsUsage: "<video_id or bvid>",
				Action: func(c *cli.Context) error {
					ident := c.Args().First()
					if ident == "" {
						// 列出所有字幕记录
						entries, _ := os.ReadDir(filepath.Join(cfg.DataDir, "subtitles"))
						if len(entries) == 0 {
							fmt.Println("📭 暂无字幕上传记录")
							return nil
						}
						fmt.Printf("📋 共 %d 个字幕记录:\n\n", len(entries))
						for _, e := range entries {
							if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
								continue
							}
							data, _ := os.ReadFile(filepath.Join(cfg.DataDir, "subtitles", e.Name()))
							if data == nil {
								continue
							}
							var tracks []storage.SubtitleTrack
							if json.Unmarshal(data, &tracks) != nil {
								continue
							}
							for _, t := range tracks {
								icon := map[string]string{
									"pending": "⏳", "uploaded": "✅",
									"failed": "❌", "missing": "⭕",
								}[t.Status]
								if icon == "" {
									icon = "❓"
								}
								fmt.Printf("  %s %s | %s | %s\n", icon, t.FileName, t.BVID, t.Status)
							}
						}
						return nil
					}

					// 先按 videoID 查找
					subStore := storage.NewSubtitleStore(filepath.Join(cfg.DataDir, "subtitles"))
					tracks, allDone := subStore.GetStatus(ident)
					if tracks != nil {
						fmt.Printf("\n📝 视频 %s 字幕状态:\n", ident)
						for _, t := range tracks {
							icon := map[string]string{
								"pending": "⏳", "uploaded": "✅",
								"failed": "❌", "missing": "⭕",
							}[t.Status]
							fmt.Printf("  %s %s (%s): %s\n", icon, t.FileName, t.Language, t.Status)
							if t.Error != "" {
								fmt.Printf("    错误: %s\n", t.Error)
							}
						}
						if allDone {
							fmt.Printf("\n✅ 全部已上传完成\n")
						} else {
							fmt.Printf("\n💡 重试: ytb2bili subtitle retry %s\n", ident)
						}
						return nil
					}

					// 按 BVID 查找
					if strings.HasPrefix(strings.ToUpper(ident), "BV") {
						entries, _ := os.ReadDir(filepath.Join(cfg.DataDir, "subtitles"))
						for _, e := range entries {
							if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
								continue
							}
							data, _ := os.ReadFile(filepath.Join(cfg.DataDir, "subtitles", e.Name()))
							if data == nil {
								continue
							}
							var ts []storage.SubtitleTrack
							if json.Unmarshal(data, &ts) != nil {
								continue
							}
							for _, t := range ts {
								if t.BVID == ident {
									fmt.Printf("\n📝 视频 %s 字幕状态:\n", ident)
									for _, tt := range ts {
										icon := map[string]string{
											"pending": "⏳", "uploaded": "✅",
											"failed": "❌", "missing": "⭕",
										}[tt.Status]
										fmt.Printf("  %s %s (%s): %s\n", icon, tt.FileName, tt.Language, tt.Status)
									}
									return nil
								}
							}
						}
					}

					return fmt.Errorf("未找到视频 %s 的字幕记录", ident)
				},
			},
		},
	}
}

// extractYouTubeID 从 YouTube URL 中提取视频 ID
func extractYouTubeID(url string) string {
	// https://www.youtube.com/watch?v=VIDEO_ID
	// https://youtu.be/VIDEO_ID
	// https://www.youtube.com/embed/VIDEO_ID
	for _, prefix := range []string{"v=", "youtu.be/", "embed/"} {
		if idx := strings.Index(url, prefix); idx >= 0 {
			start := idx + len(prefix)
			end := strings.IndexAny(url[start:], "?&#")
			if end < 0 {
				end = len(url) - start
			}
			id := url[start : start+end]
			if len(id) == 11 {
				return id
			}
		}
	}
	// 尝试直接使用最后一段路径
	parts := strings.Split(url, "/")
	return parts[len(parts)-1]
}

// ─── Auto (Autonomous Mode) ──────────────────────────────────────────────────

func autoCommand(cfg *config.Config) *cli.Command {
	return &cli.Command{
		Name:      "auto",
		Usage:     "🤖 自主模式：自动搜索高价值视频并批量提交到B站",
		ArgsUsage: "<keyword1> [keyword2 ...]",
		Flags: []cli.Flag{
			&cli.IntFlag{Name: "max-videos", Aliases: []string{"n"}, Value: 3, Usage: "最多提交多少个视频"},
			&cli.IntFlag{Name: "min-views", Value: 1000, Usage: "最低观看次数过滤"},
			&cli.IntFlag{Name: "max-duration", Value: 1800, Usage: "最长时长（秒，默认30分钟=1800）"},
			&cli.StringFlag{Name: "date", Value: "this_week", Usage: "上传日期过滤: today, this_week, this_month, this_year"},
			&cli.BoolFlag{Name: "dry-run", Usage: "仅显示搜索结果，不上传"},
			&cli.BoolFlag{Name: "skip-translate", Usage: "跳过翻译"},
			&cli.IntFlag{Name: "tid", Value: cfg.BiliTid, Usage: "B站分区ID"},
		},
		Action: func(c *cli.Context) error {
			keywords := c.Args().Slice()
			if len(keywords) == 0 {
				return fmt.Errorf("请提供至少一个搜索关键词")
			}

			maxVideos := c.Int("max-videos")
			minViews := int64(c.Int("min-views"))
			maxDuration := c.Int("max-duration")
			dateFilter := c.String("date")
			dryRun := c.Bool("dry-run")

			// ── 阶段1: 搜索 ──
			fmt.Printf("🤖 自主模式启动\n")
			fmt.Printf("   关键词: %v\n", keywords)
			fmt.Printf("   条件: ≥%d views, ≤%ds, %s\n\n", minViews, maxDuration, dateFilter)

			history := storage.NewHistoryStore(filepath.Join(cfg.DataDir, "history"))
			searcher := search.New(20)

			// 收集所有候选视频
			type scoredVideo struct {
				Video   search.Video
				Keyword string
			}
			var candidates []scoredVideo
			seenIDs := make(map[string]bool)

			for _, kw := range keywords {
				// 展开短 ID + 追加负向屏蔽词
				safeQuery := search.BuildSearchQuery(kw)
				fmt.Printf("🔍 搜索: \"%s\"\n", safeQuery[:min(len(safeQuery), 100)]+"...")

				result, err := searcher.SearchWithOptions(kw,
					search.WithSortBy("view_count"),
					search.WithUploadDate(dateFilter),
				)
				if err != nil {
					fmt.Fprintf(os.Stderr, "  ⚠ 搜索 \"%s\" 失败: %v\n", kw, err)
					continue
				}

				// 内容安全过滤（黑名单 + 时长 + 观看数）
				filtered := search.ApplySafeSearch(result.Videos, minViews, maxDuration)
				fmt.Printf("   找到 %d 个结果，过滤后 %d 个\n", len(result.Videos), len(filtered))

				for _, v := range filtered {
					if seenIDs[v.ID] {
						continue
					}
					seenIDs[v.ID] = true

					// 排除已提交
					if history.IsSubmitted(v.ID) {
						continue
					}

					candidates = append(candidates, scoredVideo{Video: v, Keyword: kw})
				}
			}

			if len(candidates) == 0 {
				fmt.Println("\n📭 没有找到符合条件的视频")
				return nil
			}

			// 按观看数降序排列
			sort.Slice(candidates, func(i, j int) bool {
				return candidates[i].Video.ViewCount > candidates[j].Video.ViewCount
			})

			// 取前 N 个
			if len(candidates) > maxVideos {
				candidates = candidates[:maxVideos]
			}

			fmt.Printf("\n📊 候选视频 (%d 个，取前 %d 个):\n", len(seenIDs), len(candidates))
			for i, sv := range candidates {
				v := sv.Video
				dur := formatDuration(v.DurationSec)
				fmt.Printf("  %d. [%s] %s\n", i+1, sv.Keyword, v.Title)
				fmt.Printf("     通道: %s | 时长: %s | 👁 %.0f views | %s\n",
					v.Channel, dur, float64(v.ViewCount), v.PublishTime)
				fmt.Printf("     %s\n\n", v.URL)
			}

			if dryRun {
				fmt.Println("⏭️ 跳过上传 (--dry-run)")
				return nil
			}

			// ── 阶段2: 使用共享任务链批量提交 ──
			successCount, failCount := 0, 0
			for i, sv := range candidates {
				v := sv.Video
				fmt.Printf("\\n═══════════════════════════════════════\\n")
				fmt.Printf("📦 [%d/%d] 开始处理: %s\\n", i+1, len(candidates), v.Title)
				processor := &pipeline.Processor{Config: cfg, Reporter: func(event pipeline.Event) {
					if event.Status == "running" {
						fmt.Printf("  [%d/%d] %s... ", event.Position, event.Total, event.Step)
					} else if event.Err != nil {
						fmt.Printf("❌ %v\\n", event.Err)
					} else {
						fmt.Println("✅")
					}
				}}
				result, err := processor.Process(c.Context, pipeline.Request{
					URL: v.URL, SourceLang: "en", TargetLang: "zh", Tid: c.Int("tid"),
					SkipTranslate: c.Bool("skip-translate"), Source: v.Channel,
				})
				if err != nil {
					fmt.Printf("❌ %v\\n", err)
					failCount++
					continue
				}
				fmt.Printf("✅ https://www.bilibili.com/video/%s\\n", result.BVID)
				fmt.Printf("   ⏱ 耗时: %.0fs\\n", result.Duration.Seconds())
				successCount++
			}

			// 汇总
			fmt.Printf("\n═══════════════════════════════════════\n")
			fmt.Printf("📊 批量处理完成\n")
			fmt.Printf("   ✅ 成功: %d\n", successCount)
			fmt.Printf("   ❌ 失败: %d\n", failCount)
			fmt.Printf("   📺 B站主页: https://space.bilibili.com/\n")
			fmt.Printf("═══════════════════════════════════════\n")

			return nil
		},
	}
}

// formatDuration 将秒数格式化为可读时间
func formatDuration(seconds int) string {
	if seconds <= 0 {
		return "?"
	}
	h := seconds / 3600
	m := (seconds % 3600) / 60
	s := seconds % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// ─── submitPipeline ───────────────────────────────────────────────────────────────
// 可复用的提交流水线，被 submit 命令和 queue work 共享

type submitOptions struct {
	SourceLang    string
	TargetLang    string
	Tid           int
	DryRun        bool
	SkipTranslate bool
	Source        string // 来源标识（manual, channel-sync, ghibli）
	Chain         []string
	ShowPlan      bool
}

// submitPipeline 执行完整提交流水线：下载→转录→翻译→元数据→上传→记录
// 返回 bvid 和可能的错误
// ─── Queue ─────────────────────────────────────────────────────────────────────

func queueCommand(cfg *config.Config) *cli.Command {
	return &cli.Command{
		Name:  "queue",
		Usage: "作业队列管理（集中式状态机，防重复、防竞态）",
		Subcommands: []*cli.Command{
			{
				Name:      "add",
				Usage:     "添加视频到队列（幂等：已存在则忽略）",
				ArgsUsage: "<YouTube URL>",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "source", Value: "manual", Usage: "来源标识 (manual, channel-sync, ghibli)"},
				},
				Action: func(c *cli.Context) error {
					url := c.Args().First()
					if url == "" {
						return fmt.Errorf("请输入 YouTube URL")
					}
					videoID := queue.ExtractVideoID(url)
					if videoID == "" {
						return fmt.Errorf("无法从 URL 中提取视频 ID: %s", url)
					}
					q := queue.New(cfg.DataDir)
					added, err := q.Add(videoID, url, "", "", c.String("source"))
					if err != nil {
						return fmt.Errorf("添加失败: %w", err)
					}
					if added {
						fmt.Printf("✅ 已加入队列: %s\n", url)
					} else {
						fmt.Printf("⏭️ 已在队列中: %s\n", url)
					}
					return nil
				},
			},
			{
				Name:  "status",
				Usage: "查看队列状态",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "status", Usage: "按状态过滤 (queued, claimed, completed, failed, skipped)"},
				},
				Action: func(c *cli.Context) error {
					q := queue.New(cfg.DataDir)

					if statusFilter := c.String("status"); statusFilter != "" {
						if !queue.IsValidStatus(statusFilter) {
							return fmt.Errorf("无效状态: %s（有效值: queued, claimed, completed, failed, skipped）", statusFilter)
						}
						data, err := q.Status()
						if err != nil {
							return err
						}
						var filtered []queue.Video
						for _, v := range data.Videos {
							if v.Status == statusFilter {
								filtered = append(filtered, v)
							}
						}
						fmt.Printf("📊 队列状态 [%s]: %d 个\n", statusFilter, len(filtered))
						for _, v := range filtered {
							fmt.Printf("  %s | %s\n", v.Status, v.Title)
							if len(v.Title) > 50 {
								v.Title = v.Title[:50] + "..."
							}
							fmt.Printf("    URL: %s\n", v.URL)
							if v.BVID != "" {
								fmt.Printf("    B站: https://www.bilibili.com/video/%s\n", v.BVID)
							}
							if v.Error != "" {
								fmt.Printf("    错误: %s\n", v.Error)
							}
							fmt.Println()
						}
						return nil
					}

					stats := q.Stats()
					fmt.Printf("📊 队列统计:\n")
					fmt.Printf("   总任务:  %d\n", stats["total"])
					fmt.Printf("   ⏳ 排队中: %d\n", stats["queued"])
					fmt.Printf("   🔄 处理中: %d\n", stats["claimed"])
					fmt.Printf("   ✅ 已完成: %d\n", stats["completed"])
					fmt.Printf("   ❌ 已失败: %d\n", stats["failed"])
					fmt.Printf("   ⏭️ 已跳过: %d\n", stats["skipped"])
					return nil
				},
			},
			{
				Name:  "next",
				Usage: "取出下一个可用的视频（queued → claimed）",
				Action: func(c *cli.Context) error {
					q := queue.New(cfg.DataDir)
					workerID := queue.WorkerID()
					video, err := q.Next(workerID)
					if err != nil {
						return fmt.Errorf("取出失败: %w", err)
					}
					if video == nil {
						fmt.Println("📭 队列中没有待处理视频")
						return nil
					}
					// JSON 输出，方便脚本解析
					fmt.Println(video.URL)
					return nil
				},
			},
			{
				Name:      "complete",
				Usage:     "标记视频处理成功（claimed → completed）",
				ArgsUsage: "<video_id> <bvid>",
				Action: func(c *cli.Context) error {
					videoID := c.Args().Get(0)
					bvid := c.Args().Get(1)
					if videoID == "" || bvid == "" {
						return fmt.Errorf("用法: ytb queue complete <video_id> <bvid>")
					}
					q := queue.New(cfg.DataDir)
					if err := q.Complete(videoID, bvid); err != nil {
						return fmt.Errorf("标记完成失败: %w", err)
					}
					fmt.Printf("✅ %s 已完成 (BVID: %s)\n", videoID, bvid)
					return nil
				},
			},
			{
				Name:      "fail",
				Usage:     "标记视频处理失败（claimed → failed 或自动重试）",
				ArgsUsage: "<video_id> <error_message>",
				Action: func(c *cli.Context) error {
					videoID := c.Args().Get(0)
					errMsg := c.Args().Get(1)
					if videoID == "" {
						return fmt.Errorf("用法: ytb queue fail <video_id> <error_message>")
					}
					q := queue.New(cfg.DataDir)
					if err := q.Fail(videoID, errMsg); err != nil {
						return fmt.Errorf("标记失败时出错: %w", err)
					}
					// 检查是否已重试到上限
					v, _ := q.GetByID(videoID)
					if v != nil && v.Status == queue.StatusFailed {
						fmt.Printf("❌ %s 已失败（已达最大重试次数 %d）\n", videoID, v.MaxRetries)
					} else if v != nil {
						fmt.Printf("🔄 %s 已失败，将自动重试 (第 %d/%d 次)\n", videoID, v.RetryCount, v.MaxRetries)
					}
					return nil
				},
			},
			{
				Name:      "reset",
				Usage:     "重置视频状态为 queued（用于修复后重新处理）",
				ArgsUsage: "<video_id>",
				Action: func(c *cli.Context) error {
					videoID := c.Args().First()
					if videoID == "" {
						return fmt.Errorf("用法: ytb queue reset <video_id>")
					}
					q := queue.New(cfg.DataDir)
					if err := q.Reset(videoID); err != nil {
						return fmt.Errorf("重置失败: %w", err)
					}
					fmt.Printf("🔄 %s 已重置为 queued\n", videoID)
					return nil
				},
			},
			{
				Name:  "work",
				Usage: "启动工作进程：自动取队列 → 提交 → 标记完成/失败",
				Description: `持续从队列中取视频执行提交流水线。
每次取一个视频，完成后自动取下一个。
空闲时每 30 秒重试。支持 --once 单次模式。

死锁检测：claimed 超过 30 分钟自动回退到 queued 重新处理。`,
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: "once", Usage: "只处理一个视频后退出"},
					&cli.BoolFlag{Name: "skip-translate", Usage: "跳过翻译"},
					&cli.IntFlag{Name: "tid", Value: cfg.BiliTid, Usage: "B站分区ID"},
					&cli.StringFlag{Name: "source-lang", Value: "en", Usage: "源语言"},
					&cli.StringFlag{Name: "target-lang", Value: "zh", Usage: "目标语言"},
					&cli.IntFlag{Name: "poll-interval", Value: 30, Usage: "空闲时轮询间隔（秒）"},
				},
				Action: func(c *cli.Context) error {
					q := queue.New(cfg.DataDir)
					workerID := queue.WorkerID()
					pollInterval := c.Int("poll-interval")
					once := c.Bool("once")

					opts := &submitOptions{
						SourceLang:    c.String("source-lang"),
						TargetLang:    c.String("target-lang"),
						Tid:           c.Int("tid"),
						SkipTranslate: c.Bool("skip-translate"),
					}

					fmt.Printf("🚀 工作进程启动 (Worker: %s)\n", workerID)
					if once {
						fmt.Println("   模式: 单次运行")
					} else {
						fmt.Printf("   轮询间隔: %ds\n", pollInterval)
					}

					processed := 0
					for {
						video, err := q.Next(workerID)
						if err != nil {
							fmt.Fprintf(os.Stderr, "⚠ 取出视频失败: %v\n", err)
							if once {
								return err
							}
							time.Sleep(time.Duration(pollInterval) * time.Second)
							continue
						}
						if video == nil {
							if once && processed == 0 {
								fmt.Println("📭 队列为空")
								return nil
							}
							if once {
								break
							}
							time.Sleep(time.Duration(pollInterval) * time.Second)
							continue
						}

						processed++
						fmt.Printf("\n═══════════════════════════════════════\n")
						fmt.Printf("📦 [%s] 开始处理: %s\n", video.VideoID, video.Title)
						fmt.Printf("═══════════════════════════════════════\n")

						pipeResult, submitErr := (&pipeline.Processor{Config: cfg}).Process(c.Context, pipeline.Request{
							URL: video.URL, SourceLang: opts.SourceLang, TargetLang: opts.TargetLang,
							Tid: opts.Tid, SkipTranslate: opts.SkipTranslate, Source: video.Source,
						})
						bvid := ""
						if pipeResult != nil {
							bvid = pipeResult.BVID
						}

						if submitErr != nil {
							errMsg := submitErr.Error()
							fmt.Fprintf(os.Stderr, "❌ 处理失败: %s\n", errMsg)
							if fErr := q.Fail(video.VideoID, errMsg); fErr != nil {
								fmt.Fprintf(os.Stderr, "⚠ 标记失败时出错: %v\n", fErr)
							}
						} else {
							if cErr := q.Complete(video.VideoID, bvid); cErr != nil {
								fmt.Fprintf(os.Stderr, "⚠ 标记完成时出错: %v\n", cErr)
							}
							fmt.Printf("\n✅ 处理完成: https://www.bilibili.com/video/%s\n", bvid)
						}

						if once {
							break
						}
					}

					fmt.Printf("\n📊 本轮处理了 %d 个视频\n", processed)
					return nil
				},
			},
		},
	}
}
