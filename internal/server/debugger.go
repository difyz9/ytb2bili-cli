package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zolagz/ytb2bili-go/internal/config"
	"github.com/zolagz/ytb2bili-go/internal/download"
	"github.com/zolagz/ytb2bili-go/internal/storage"
	"github.com/zolagz/ytb2bili-go/internal/transcriber"
	"github.com/zolagz/ytb2bili-go/internal/translator"
)

// Debugger 调试器
type Debugger struct {
	cfg           *config.Config
	feishu        *FeishuBot
	dryRun        bool
	messageCount  int
	history       *storage.HistoryStore
}

// NewDebugger 创建调试器
func NewDebugger(cfg *config.Config, feishu *FeishuBot, dryRun bool) *Debugger {
	historyDir := cfg.DataDir + "/history"
	h := storage.NewHistoryStore(historyDir)

	return &Debugger{
		cfg:     cfg,
		feishu:  feishu,
		dryRun:  dryRun,
		history: h,
	}
}

// Start 启动调试器
func (d *Debugger) Start() error {
	fmt.Println("🚀 启动飞书机器人...")
	fmt.Println("💡 在飞书中发送消息测试")
	fmt.Println("   - 发送 YouTube 链接测试视频提交")
	fmt.Println("   - 发送 Chrome 插件提交的 JSON 数据")
	fmt.Println("   - 发送 'help' 查看帮助")
	fmt.Println("   - 发送 'status' 查看状态")
	fmt.Println("   - 按 Ctrl+C 退出")
	fmt.Println()

	// 启动飞书机器人
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 启动飞书机器人
	go func() {
		if err := d.feishu.Start(ctx); err != nil {
			log.Printf("❌ 飞书机器人启动失败: %v", err)
		}
	}()

	// 处理消息
	for msg := range d.feishu.GetMessageChan() {
		d.messageCount++
		d.handleMessage(msg)
	}

	return nil
}

// handleMessage 处理消息
func (d *Debugger) handleMessage(msg *FeishuMessage) {
	fmt.Println()
	fmt.Println(strings.Repeat("━", 60))
	fmt.Printf("📨 收到消息 #%d\n", d.messageCount)
	fmt.Println(strings.Repeat("━", 60))

	// 显示消息基本信息
	fmt.Printf("   📌 消息 ID: %s\n", msg.MessageID)
	fmt.Printf("   👤 用户 ID: %s\n", msg.UserID)
	fmt.Printf("   💬 聊天 ID: %s\n", msg.ChatID)
	fmt.Printf("   📝 聊天类型: %s\n", msg.ChatType)
	fmt.Printf("   ⏰ 时间: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println()

	// 显示消息内容
	fmt.Println("   📄 消息内容:")
	content := strings.TrimSpace(msg.Content)
	if len(content) > 500 {
		content = content[:500] + "..."
	}
	fmt.Printf("      %s\n", content)
	fmt.Println()

	// 解析消息类型
	msgType := d.parseMessageType(content)
	fmt.Printf("   🔍 消息类型: %s\n", msgType)

	// 处理不同类型的消息
	ctx := context.Background()
	switch msgType {
	case "video_submit_json":
		d.handleVideoSubmitJSON(ctx, msg, content)
	case "youtube_url":
		d.handleYouTubeMessage(ctx, msg, content)
	case "command":
		d.handleCommandMessage(ctx, msg, content)
	case "card_data":
		d.handleCardDataMessage(ctx, msg, content)
	default:
		d.handleUnknownMessage(ctx, msg, content)
	}

	fmt.Println(strings.Repeat("━", 60))
}

// parseMessageType 解析消息类型
func (d *Debugger) parseMessageType(content string) string {
	content = strings.TrimSpace(content)

	// 检查是否是 Chrome 插件提交的 JSON 数据
	if strings.HasPrefix(content, "{") && strings.HasSuffix(content, "}") {
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(content), &data); err == nil {
			if dataType, ok := data["type"].(string); ok && dataType == "video_submit" {
				return "video_submit_json"
			}
		}
		return "card_data"
	}

	// 检查是否是 YouTube URL
	if strings.Contains(content, "youtube.com/watch?v=") ||
		strings.Contains(content, "youtu.be/") ||
		strings.Contains(content, "youtube.com/shorts/") {
		return "youtube_url"
	}

	// 检查是否是命令
	commands := []string{"help", "status", "history", "list", "clear"}
	for _, cmd := range commands {
		if content == cmd {
			return "command"
		}
	}

	return "unknown"
}

// handleVideoSubmitJSON 处理 Chrome 插件提交的 JSON 数据
func (d *Debugger) handleVideoSubmitJSON(ctx context.Context, msg *FeishuMessage, content string) {
	fmt.Println("   🎬 检测到 Chrome 插件视频提交")

	// 解析 JSON 数据
	var submitData struct {
		Type string `json:"type"`
		Data struct {
			URL       string `json:"url"`
			Title     string `json:"title"`
			Channel   string `json:"channel"`
			VideoID   string `json:"video_id"`
			Cookies   string `json:"cookies"`
			Subtitles []struct {
				Text     string  `json:"text"`
				Duration float64 `json:"duration"`
				Offset   float64 `json:"offset"`
			} `json:"subtitles"`
		} `json:"data"`
	}

	if err := json.Unmarshal([]byte(content), &submitData); err != nil {
		fmt.Printf("   ❌ JSON 解析失败: %v\n", err)
		d.feishu.ReplyMessage(ctx, msg, "❌ 消息格式错误，请重新提交")
		return
	}

	// 显示提取的信息
	fmt.Printf("   📺 标题: %s\n", submitData.Data.Title)
	fmt.Printf("   👤 频道: %s\n", submitData.Data.Channel)
	fmt.Printf("   🔗 链接: %s\n", submitData.Data.URL)
	fmt.Printf("   🎬 视频 ID: %s\n", submitData.Data.VideoID)

	// 显示 cookies 信息
	if submitData.Data.Cookies != "" {
		// 解析 cookies JSON
		var cookies []struct {
			Name     string `json:"name"`
			Value    string `json:"value"`
			Domain   string `json:"domain"`
			Path     string `json:"path"`
			Expire   int64  `json:"expire"`
			HttpOnly bool   `json:"http_only"`
			Secure   bool   `json:"secure"`
		}

		if err := json.Unmarshal([]byte(submitData.Data.Cookies), &cookies); err == nil {
			fmt.Printf("   🍪 Cookies: %d 条\n", len(cookies))
			// 显示关键 cookies（不显示完整值）
			for _, c := range cookies {
				if c.Name == "LOGIN_INFO" || c.Name == "SID" || c.Name == "SSID" {
					valuePreview := c.Value
					if len(valuePreview) > 20 {
						valuePreview = valuePreview[:20] + "..."
					}
					fmt.Printf("      - %s: %s\n", c.Name, valuePreview)
				}
			}
		} else {
			fmt.Printf("   🍪 Cookies: 解析失败\n")
		}
	} else {
		fmt.Printf("   🍪 Cookies: 未提供\n")
	}

	// 显示字幕信息
	if len(submitData.Data.Subtitles) > 0 {
		fmt.Printf("   📝 字幕: %d 条\n", len(submitData.Data.Subtitles))
	}

	fmt.Println()

	// 解析 cookies 数组用于统计
	var cookiesCount int
	if submitData.Data.Cookies != "" {
		var cookies []struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal([]byte(submitData.Data.Cookies), &cookies); err == nil {
			cookiesCount = len(cookies)
		}
	}

	if d.dryRun {
		fmt.Println("   ⚠️  干运行模式，跳过处理")
		d.feishu.ReplyMarkdown(ctx, msg, fmt.Sprintf("✅ 收到视频提交（干运行模式）\n\n**标题:** %s\n**链接:** %s\n**Cookies:** %d 条\n**字幕:** %d 条",
			submitData.Data.Title, submitData.Data.URL, cookiesCount, len(submitData.Data.Subtitles)))
		return
	}

	// 检查是否已提交
	if d.history.IsSubmitted(submitData.Data.VideoID) {
		submitted := d.history.GetSubmitted(submitData.Data.VideoID)
		d.feishu.ReplyMarkdown(ctx, msg, fmt.Sprintf("⚠️ 该视频已提交过\n\nB站链接: https://www.bilibili.com/video/%s\n提交时间: %s", submitted.BVID, submitted.SubmittedAt[:19]))
		return
	}

	// 发送处理中消息
	d.feishu.ReplyMarkdown(ctx, msg, fmt.Sprintf("📥 收到视频提交，开始处理...\n\n**标题:** %s\n**链接:** %s", submitData.Data.Title, submitData.Data.URL))

	// 保存 cookies 到文件
	outputDir := d.cfg.DataDir + "/downloads/" + submitData.Data.VideoID
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Printf("   ❌ 创建目录失败: %v\n", err)
		return
	}

	cookiesPath := ""
	if submitData.Data.Cookies != "" {
		cookiesPath = filepath.Join(outputDir, "cookies.txt")
		if err := d.saveCookiesToFile(submitData.Data.Cookies, cookiesPath); err != nil {
			fmt.Printf("   ❌ 保存 cookies 失败: %v\n", err)
		} else {
			fmt.Printf("   ✅ Cookies 已保存到: %s\n", cookiesPath)
		}
	}

	// 开始处理视频
	go d.processVideo(submitData.Data.URL, submitData.Data.VideoID, submitData.Data.Title, cookiesPath, msg)
}

// saveCookiesToFile 保存 cookies 到文件
func (d *Debugger) saveCookiesToFile(cookiesJSON string, filePath string) error {
	var cookies []struct {
		Name     string `json:"name"`
		Value    string `json:"value"`
		Domain   string `json:"domain"`
		Path     string `json:"path"`
		Expire   int64  `json:"expire"`
		HttpOnly bool   `json:"http_only"`
		Secure   bool   `json:"secure"`
	}

	if err := json.Unmarshal([]byte(cookiesJSON), &cookies); err != nil {
		return err
	}

	// 生成 Netscape 格式的 cookies 文件
	var sb strings.Builder
	sb.WriteString("# Netscape HTTP Cookie File\n")
	sb.WriteString("# This file was generated by ytb2bili-go\n\n")

	for _, c := range cookies {
		secure := "FALSE"
		if c.Secure {
			secure = "TRUE"
		}

		// 格式: domain flag path secure expires name value
		sb.WriteString(fmt.Sprintf("%s	%s	%s	%s	%d	%s	%s\n",
			c.Domain,
			"TRUE", // includeSubdomains
			c.Path,
			secure,
			c.Expire,
			c.Name,
			c.Value,
		))
	}

	return os.WriteFile(filePath, []byte(sb.String()), 0644)
}

// processVideo 处理视频
func (d *Debugger) processVideo(url, videoID, title, cookiesPath string, msg *FeishuMessage) {
	fmt.Println()
	fmt.Println(strings.Repeat("━", 60))
	fmt.Printf("🎬 开始处理视频: %s\n", title)
	fmt.Println(strings.Repeat("━", 60))

	outputDir := d.cfg.DataDir + "/downloads/" + videoID

	// Step 1: 下载视频
	fmt.Println("\n📥 Step 1: 下载视频...")
	d.feishu.ReplyMarkdown(context.Background(), msg, "📥 正在下载视频...")

	result, err := download.Video(url, outputDir, "en", cookiesPath)
	if err != nil {
		fmt.Printf("   ❌ 下载失败: %v\n", err)
		d.feishu.ReplyMessage(context.Background(), msg, fmt.Sprintf("❌ 下载失败: %v", err))
		return
	}
	fmt.Printf("   ✅ 下载完成: %s\n", result.VideoPath)

	// Step 2: 语音转录
	fmt.Println("\n🎙️ Step 2: 语音转录...")
	d.feishu.ReplyMarkdown(context.Background(), msg, "🎙️ 正在进行语音转录...")

	srtPath, err := transcriber.BcutASR(result.VideoPath, outputDir)
	if err != nil {
		fmt.Printf("   ❌ 转录失败: %v\n", err)
		d.feishu.ReplyMessage(context.Background(), msg, fmt.Sprintf("❌ 转录失败: %v", err))
		return
	}
	fmt.Printf("   ✅ 转录完成: %s\n", srtPath)

	// Step 3: 翻译字幕
	fmt.Println("\n🔤 Step 3: 翻译字幕...")
	d.feishu.ReplyMarkdown(context.Background(), msg, "🔤 正在翻译字幕...")

	trans := translator.New(translator.Config{
		APIKey:     d.cfg.LLMAPIKey,
		BaseURL:    d.cfg.LLMBaseURL,
		Model:      d.cfg.LLMModel,
		SourceLang: "en",
		TargetLang: "zh",
		BatchSize:  25,
		MaxWorkers: 3,
	})

	zhSrtPath := outputDir + "/subtitle.zh.srt"
	err = trans.TranslateSRTFile(context.Background(), srtPath, zhSrtPath)
	if err != nil {
		fmt.Printf("   ❌ 翻译失败: %v\n", err)
		d.feishu.ReplyMessage(context.Background(), msg, fmt.Sprintf("❌ 翻译失败: %v", err))
		return
	}
	fmt.Printf("   ✅ 翻译完成: %s\n", zhSrtPath)

	// 完成
	fmt.Println()
	fmt.Println(strings.Repeat("━", 60))
	fmt.Println("✅ 视频处理完成！")
	fmt.Printf("   📁 输出目录: %s\n", outputDir)
	fmt.Printf("   🎬 视频: %s\n", result.VideoPath)
	fmt.Printf("   📝 英文字幕: %s\n", srtPath)
	fmt.Printf("   📝 中文字幕: %s\n", zhSrtPath)
	fmt.Println(strings.Repeat("━", 60))

	// 发送完成消息
	d.feishu.ReplyMarkdown(context.Background(), msg, fmt.Sprintf("✅ 视频处理完成！\n\n**标题:** %s\n**视频:** %s\n**英文字幕:** %s\n**中文字幕:** %s",
		title, result.VideoPath, srtPath, zhSrtPath))
}

// handleYouTubeMessage 处理 YouTube URL 消息
func (d *Debugger) handleYouTubeMessage(ctx context.Context, msg *FeishuMessage, content string) {
	fmt.Println("   🎬 检测到 YouTube 视频链接")

	// 提取视频 URL
	youtubeURL := ParseYouTubeURL(content)
	if youtubeURL == "" {
		fmt.Println("   ❌ 无法解析 YouTube URL")
		return
	}

	videoID := extractVideoID(youtubeURL)
	fmt.Printf("   📺 视频 ID: %s\n", videoID)
	fmt.Printf("   🔗 视频链接: %s\n", youtubeURL)

	if d.dryRun {
		fmt.Println("   ⚠️  干运行模式，跳过处理")
		d.feishu.ReplyMarkdown(ctx, msg, fmt.Sprintf("✅ 收到视频链接（干运行模式）\n\n**视频 ID:** %s\n**链接:** %s", videoID, youtubeURL))
		return
	}

	// 检查是否已提交
	if d.history.IsSubmitted(videoID) {
		submitted := d.history.GetSubmitted(videoID)
		d.feishu.ReplyMarkdown(ctx, msg, fmt.Sprintf("⚠️ 该视频已提交过\n\nB站链接: https://www.bilibili.com/video/%s\n提交时间: %s", submitted.BVID, submitted.SubmittedAt[:19]))
		return
	}

	// 发送处理中消息
	d.feishu.ReplyMarkdown(ctx, msg, fmt.Sprintf("📥 收到视频链接，开始处理...\n\n**链接:** %s", youtubeURL))

	// 开始处理视频
	go d.processVideo(youtubeURL, videoID, "待处理", "", msg)
}

// handleCommandMessage 处理命令消息
func (d *Debugger) handleCommandMessage(ctx context.Context, msg *FeishuMessage, content string) {
	fmt.Printf("   📋 执行命令: %s\n", content)

	switch content {
	case "help":
		d.feishu.ReplyCard(ctx, msg, CreateHelpCard())
	case "status":
		videos, _ := d.history.List()
		d.feishu.ReplyCard(ctx, msg, CreateStatusCard(0, len(videos)))
	case "history":
		videos, _ := d.history.List()
		var videoList []map[string]string
		for _, v := range videos {
			videoList = append(videoList, map[string]string{
				"title":         v.Title,
				"youtube_url":   fmt.Sprintf("https://www.youtube.com/watch?v=%s", v.YouTubeID),
				"bilibili_url":  fmt.Sprintf("https://www.bilibili.com/video/%s", v.BVID),
			})
		}
		d.feishu.ReplyCard(ctx, msg, CreateHistoryCard(videoList))
	default:
		d.feishu.ReplyMarkdown(ctx, msg, fmt.Sprintf("未知命令: %s\n\n输入 `help` 查看帮助", content))
	}
}

// handleCardDataMessage 处理卡片数据消息
func (d *Debugger) handleCardDataMessage(ctx context.Context, msg *FeishuMessage, content string) {
	fmt.Println("   📦 检测到卡片数据")

	// 尝试解析 JSON
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(content), &data); err != nil {
		fmt.Printf("   ❌ JSON 解析失败: %v\n", err)
		return
	}

	// 显示数据结构
	fmt.Println("   📊 数据结构:")
	printJSON(data, "      ")

	if d.dryRun {
		fmt.Println("   ⚠️  干运行模式，跳过处理")
		return
	}

	// 处理提交数据
	if data["type"] == "video_submit" {
		d.handleVideoSubmission(ctx, msg, data)
	}
}

// handleVideoSubmission 处理视频提交
func (d *Debugger) handleVideoSubmission(ctx context.Context, msg *FeishuMessage, data map[string]interface{}) {
	fmt.Println("   🎬 处理视频提交...")

	videoData, ok := data["data"].(map[string]interface{})
	if !ok {
		fmt.Println("   ❌ 无效的视频数据")
		return
	}

	url, _ := videoData["url"].(string)
	title, _ := videoData["title"].(string)

	fmt.Printf("   📺 视频: %s\n", title)
	fmt.Printf("   🔗 链接: %s\n", url)

	// 发送确认
	d.feishu.ReplyMarkdown(ctx, msg, fmt.Sprintf("✅ 收到视频提交\n\n**标题:** %s\n**链接:** %s\n\n正在处理中...", title, url))
}

// handleUnknownMessage 处理未知消息
func (d *Debugger) handleUnknownMessage(ctx context.Context, msg *FeishuMessage, content string) {
	fmt.Println("   ❓ 未知消息类型")

	// 回复消息
	d.feishu.ReplyMarkdown(ctx, msg, "收到消息！\n\n发送 YouTube 链接可自动处理\n输入 `help` 查看帮助")
}

// printJSON 打印 JSON 数据
func printJSON(data interface{}, prefix string) {
	bytes, err := json.MarshalIndent(data, prefix, "  ")
	if err != nil {
		fmt.Printf("%s%v\n", prefix, data)
		return
	}
	fmt.Println(string(bytes))
}
