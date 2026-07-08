package command

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/difyz9/bilibili-go-sdk/bilibili"
	"github.com/urfave/cli/v2"

	"github.com/difyz9/ytb2bili-cli/internal/auth"
	"github.com/difyz9/ytb2bili-cli/internal/bili"
	"github.com/difyz9/ytb2bili-cli/internal/channel"
	"github.com/difyz9/ytb2bili-cli/internal/config"
	"github.com/difyz9/ytb2bili-cli/internal/download"
	"github.com/difyz9/ytb2bili-cli/internal/metadata"
	"github.com/difyz9/ytb2bili-cli/internal/search"
	"github.com/difyz9/ytb2bili-cli/internal/server"
	"github.com/difyz9/ytb2bili-cli/internal/storage"
	"github.com/difyz9/ytb2bili-cli/internal/transcriber"
	"github.com/difyz9/ytb2bili-cli/internal/translator"
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
		loginCommand(cfg),
		searchCommand(cfg),
		submitCommand(cfg),
		taskCommand(cfg),
		channelCommand(cfg),
		serverCommand(cfg),
		debugCommand(cfg),
		bitableCommand(cfg),
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
					srtPath, err = transcriber.BcutASR(dlResult.VideoPath, dlDir)
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
				translatedSrt := srtPath
				if !c.Bool("skip-translate") {
					ts.UpdateStep(id, "translate", "running")
					fmt.Printf("🌐 [3/5] AI 翻译 (en→zh)... ")
					translatedSrt, err = translator.SRT(srtPath, "en", "zh", cfg)
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
				fmt.Printf("✅ https://www.bilibili.com/video/%s\n", bvid)

				// 记录到历史
				history.Add(&storage.SubmittedVideo{
					YouTubeID: video.ID,
					BVID:      bvid,
					Title:     video.Title,
					Channel:   video.Channel,
				})

				// 异步监听审核状态
				if translatedSrt != "" {
					go watchAndUploadSubtitle(bvid, translatedSrt, &cred, cfg)
				}

				elapsed := time.Since(totalStart).Seconds()
				fmt.Printf("\n✨ 总耗时: %.0fs\n", elapsed)
			}

			return nil
		},
	}
}

// ─── Login ──────────────────────────────────────────────────────────────────

func loginCommand(cfg *config.Config) *cli.Command {
	return &cli.Command{
		Name:  "login",
		Usage: "B站扫码登录",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "password", Usage: "使用密码登录"},
			&cli.StringFlag{Name: "username", Usage: "B站账号"},
			&cli.StringFlag{Name: "password-val", Usage: "B站密码"},
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
				// TODO: password login via bilibili-go-sdk
				return fmt.Errorf("密码登录待实现")
			}

			// QR code login
			fmt.Println("📱 获取二维码...")
			qr, err := auth.GetQRCode()
			if err != nil {
				return err
			}

			// Generate QR code image
			qrPath := filepath.Join(credDir, "bilibili_qrcode.png")
			if err := SaveQRCode(qr.URL, qrPath); err != nil {
				fmt.Fprintf(os.Stderr, "⚠ 无法生成二维码图片: %v\n", err)
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
		},
		Action: func(c *cli.Context) error {
			url := c.Args().First()
			if url == "" {
				return fmt.Errorf("请输入 YouTube URL")
			}

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
			result, err := download.Video(url, dlDir, c.String("source-lang"), cookiesPath)
			if err != nil {
				ts.UpdateStep(id, "download", "failed", err.Error())
				return fmt.Errorf("下载失败: %w", err)
			}
			ts.UpdateStep(id, "download", "completed")
			fmt.Printf("✅ %s\n", filepath.Base(result.VideoPath))

			// Step 2: Transcribe
			ts.UpdateStep(id, "transcribe", "running")
			srtPath := result.SubtitlePath
			if srtPath == "" {
				fmt.Print("🎙️ [2/5] Bcut 语音转字幕... ")
				srtPath, err = transcriber.BcutASR(result.VideoPath, dlDir)
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
			translatedSrt := srtPath
			if !c.Bool("skip-translate") {
				ts.UpdateStep(id, "translate", "running")
				fmt.Printf("🌐 [3/5] AI 翻译 (%s→%s)... ", c.String("source-lang"), c.String("target-lang"))
				translatedSrt, err = translator.SRT(srtPath, c.String("source-lang"), c.String("target-lang"), cfg)
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
			meta, err := metadata.Generate(result.Info, cfg)
			if err != nil {
				// Non-fatal
				meta = &metadata.VideoMeta{Title: result.Info.Title}
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

			// Load credential
			var cred auth.LoginInfo
			if err := cs.Load(&cred); err != nil {
				ts.UpdateStep(id, "upload", "failed", "未登录")
				return fmt.Errorf("请先登录: ytb2bili login")
			}

			bvid, err := bili.Upload(&cred, &bili.UploadParams{
				VideoPath:    result.VideoPath,
				Title:        meta.Title,
				Desc:         meta.Description,
				Tags:         meta.Tags,
				Source:       url,
				Tid:          c.Int("tid"),
				CoverPath:    result.CoverPath,
				// SubtitlePath 暂不上传，等审核通过后再上传
			})
			if err != nil {
				ts.UpdateStep(id, "upload", "failed", err.Error())
				return fmt.Errorf("上传失败: %w", err)
			}

			task.BVID = bvid
			task.Status = "completed"
			fmt.Printf("✅ https://www.bilibili.com/video/%s\n", bvid)

			// 异步监听审核状态，审核通过后上传字幕
			if translatedSrt != "" {
				go watchAndUploadSubtitle(bvid, translatedSrt, &cred, cfg)
			}

			elapsed := time.Since(totalStart).Seconds()
			fmt.Printf("\n✨ 总耗时: %.0fs\n", elapsed)
			return nil
		},
	}
}

// watchAndUploadSubtitle 异步监听视频审核状态，审核通过后上传字幕
func watchAndUploadSubtitle(bvid, subtitlePath string, cred *auth.LoginInfo, cfg *config.Config) {
	fmt.Printf("\n⏳ [字幕] 监听视频 %s 审核状态...\n", bvid)

	client := bilibili.NewClient()
	sdkLogin := bili.CredToSDKLogin(cred)
	cookies := buildCookiesString(cred)

	// 等待审核通过（最多等待 24 小时）
	interval := 30 * time.Second
	timeout := 24 * time.Hour

	status, err := client.WaitForVideoReviewPassed(bvid, cookies, interval, timeout)
	if err != nil {
		fmt.Printf("❌ [字幕] 等待审核失败: %v\n", err)
		return
	}

	if status == nil {
		fmt.Printf("❌ [字幕] 获取审核状态失败\n")
		return
	}

	fmt.Printf("✅ [字幕] 视频审核通过 (state=%d)\n", status.State)

	// 上传字幕
	lang := bilibili.NormalizeSubtitleLanguage("zh")
	if err := client.UploadSubtitle(sdkLogin, bvid, subtitlePath, lang); err != nil {
		fmt.Printf("❌ [字幕] 字幕上传失败: %v\n", err)
		return
	}

	fmt.Printf("✅ [字幕] 字幕上传成功: %s\n", subtitlePath)
}

// buildCookiesString 构建 cookies 字符串
func buildCookiesString(cred *auth.LoginInfo) string {
	var parts []string
	for name, val := range cred.Cookies {
		parts = append(parts, name+"="+val)
	}
	return strings.Join(parts, "; ")
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
					fmt.Println("📊 频道监控统计:\n")
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
		Name:  "server",
		Usage: "启动 HTTP API 服务器",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "addr", Value: ":8096", Usage: "监听地址"},
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
			&cli.StringFlag{Name: "feishu-app-id", Value: "cli_aaa921692978dce6", Usage: "飞书 App ID"},
			&cli.StringFlag{Name: "feishu-app-secret", Value: "x6wwyDmSVlbubdcGpO5nKhdwxniTB7ka", Usage: "飞书 App Secret"},
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
