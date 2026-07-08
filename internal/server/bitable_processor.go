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

	"github.com/difyz9/ytb2bili-cli/internal/config"
	"github.com/difyz9/ytb2bili-cli/internal/download"
	"github.com/difyz9/ytb2bili-cli/internal/feishu"
	"github.com/difyz9/ytb2bili-cli/internal/storage"
	"github.com/difyz9/ytb2bili-cli/internal/transcriber"
	"github.com/difyz9/ytb2bili-cli/internal/translator"
)

// BitableProcessor 多维表格任务处理器
type BitableProcessor struct {
	cfg      *config.Config
	client   *feishu.MultiTableClient
	config   feishu.BitableConfig
	interval time.Duration
	history  *storage.HistoryStore
}

// NewBitableProcessor 创建多维表格任务处理器
func NewBitableProcessor(cfg *config.Config, client *feishu.MultiTableClient, config feishu.BitableConfig, interval time.Duration) *BitableProcessor {
	historyDir := filepath.Join(cfg.DataDir, "history")
	return &BitableProcessor{
		cfg:      cfg,
		client:   client,
		config:   config,
		interval: interval,
		history:  storage.NewHistoryStore(historyDir),
	}
}

// Start 启动处理器
func (p *BitableProcessor) Start(ctx context.Context) {
	log.Println("🚀 多维表格任务处理器已启动")
	log.Printf("   轮询间隔: %v", p.interval)

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	// 立即执行一次
	p.processOnce(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Println("🛑 多维表格任务处理器已停止")
			return
		case <-ticker.C:
			p.processOnce(ctx)
		}
	}
}

// processOnce 处理一轮任务
func (p *BitableProcessor) processOnce(ctx context.Context) {
	tasks, err := p.client.GetPendingTasks(p.config, 10)
	if err != nil {
		log.Printf("❌ 获取任务失败: %v", err)
		return
	}

	if len(tasks) == 0 {
		return
	}

	log.Printf("📋 发现 %d 个待处理任务", len(tasks))

	for _, task := range tasks {
		p.processTask(ctx, task)
	}
}

// processTask 处理单个任务
func (p *BitableProcessor) processTask(ctx context.Context, task *feishu.VideoTaskRecord) {
	log.Printf("\n🔄 处理任务: %s (%s)", task.Title, task.VideoID)

	// 检查是否已提交
	if p.history.IsSubmitted(task.VideoID) {
		submitted := p.history.GetSubmitted(task.VideoID)
		errMsg := fmt.Sprintf("该视频已提交过，BVID: %s", submitted.BVID)
		log.Printf("⚠️ %s", errMsg)
		p.client.UpdateTaskStatus(p.config, task.RecordID, "skipped", errMsg, "")
		return
	}

	// 保存 cookies 到文件
	outputDir := p.cfg.DataDir + "/downloads/" + task.VideoID
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Printf("❌ 创建目录失败: %v", err)
		p.client.UpdateTaskStatus(p.config, task.RecordID, "failed", err.Error(), "")
		return
	}

	cookiesPath := ""
	if task.Cookies != "" {
		cookiesPath = filepath.Join(outputDir, "cookies.txt")
		if err := p.saveCookiesToFile(task.Cookies, cookiesPath); err != nil {
			log.Printf("❌ 保存 cookies 失败: %v", err)
		} else {
			log.Printf("✅ Cookies 已保存到: %s", cookiesPath)
		}
	}

	// Step 1: 下载视频
	log.Println("\n📥 Step 1: 下载视频...")
	result, err := download.Video(task.URL, outputDir, "en", cookiesPath)
	if err != nil {
		log.Printf("❌ 下载失败: %v", err)
		p.client.UpdateTaskStatus(p.config, task.RecordID, "failed", "下载失败: "+err.Error(), "")
		return
	}
	log.Printf("✅ 下载完成: %s", result.VideoPath)

	// 更新状态为 transcribing
	p.client.UpdateTaskStatus(p.config, task.RecordID, "transcribing", "", "")

	// Step 2: 语音转录
	log.Println("\n🎙️ Step 2: 语音转录...")
	srtPath, err := transcriber.BcutASR(result.VideoPath, outputDir)
	if err != nil {
		log.Printf("❌ 转录失败: %v", err)
		p.client.UpdateTaskStatus(p.config, task.RecordID, "failed", "转录失败: "+err.Error(), "")
		return
	}
	log.Printf("✅ 转录完成: %s", srtPath)

	// 更新状态为 translating
	p.client.UpdateTaskStatus(p.config, task.RecordID, "translating", "", "")

	// Step 3: 翻译字幕
	log.Println("\n🔤 Step 3: 翻译字幕...")
	trans := translator.New(translator.Config{
		APIKey:     p.cfg.LLMAPIKey,
		BaseURL:    p.cfg.LLMBaseURL,
		Model:      p.cfg.LLMModel,
		SourceLang: "en",
		TargetLang: "zh",
		BatchSize:  25,
		MaxWorkers: 3,
	})

	zhSrtPath := outputDir + "/subtitle.zh.srt"
	err = trans.TranslateSRTFile(context.Background(), srtPath, zhSrtPath)
	if err != nil {
		log.Printf("❌ 翻译失败: %v", err)
		p.client.UpdateTaskStatus(p.config, task.RecordID, "failed", "翻译失败: "+err.Error(), "")
		return
	}
	log.Printf("✅ 翻译完成: %s", zhSrtPath)

	// 完成
	log.Println()
	log.Println(strings.Repeat("━", 60))
	log.Println("✅ 视频处理完成！")
	log.Printf("   📁 输出目录: %s", outputDir)
	log.Printf("   🎬 视频: %s", result.VideoPath)
	log.Printf("   📝 英文字幕: %s", srtPath)
	log.Printf("   📝 中文字幕: %s", zhSrtPath)
	log.Println(strings.Repeat("━", 60))

	// 更新状态为 completed
	if err := p.client.UpdateTaskStatus(p.config, task.RecordID, "completed", "", ""); err != nil {
		log.Printf("❌ 更新状态失败: %v", err)
	}
}

// saveCookiesToFile 保存 cookies 到文件
func (p *BitableProcessor) saveCookiesToFile(cookiesJSON string, filePath string) error {
	// 支持两种 cookies 格式：
	// 1. Chrome 扩展格式: expirationDate (camelCase)
	// 2. 旧格式: expire (lowercase)
	var cookies []struct {
		Name            string  `json:"name"`
		Value           string  `json:"value"`
		Domain          string  `json:"domain"`
		Path            string  `json:"path"`
		Expire          int64   `json:"expire"`
		ExpirationDate  float64 `json:"expirationDate"`
		HttpOnly        bool    `json:"httpOnly"`
		Secure          bool    `json:"secure"`
		SameSite        string  `json:"sameSite"`
		Session         bool    `json:"session"`
		StoreID         string  `json:"storeId"`
		HostOnly        bool    `json:"hostOnly"`
	}

	if err := json.Unmarshal([]byte(cookiesJSON), &cookies); err != nil {
		return err
	}

	// 生成 Netscape 格式的 cookies 文件
	var sb strings.Builder
	sb.WriteString("# Netscape HTTP Cookie File\n")
	sb.WriteString("# https://curl.haxx.se/rfc/cookie_spec.html\n")
	sb.WriteString("# This is a generated file! Do not edit.\n\n")

	for _, c := range cookies {
		// 确定过期时间：优先使用 expirationDate，否则使用 expire
		var expires int64
		if c.ExpirationDate > 0 {
			expires = int64(c.ExpirationDate)
		} else {
			expires = c.Expire
		}

		// 跳过 session cookies (expires=0)
		if expires == 0 {
			continue
		}

		secure := "FALSE"
		if c.Secure {
			secure = "TRUE"
		}

		// 格式: domain flag path secure expires name value
		sb.WriteString(fmt.Sprintf("%s\tTRUE\t%s\t%s\t%d\t%s\t%s\n",
			c.Domain,
			c.Path,
			secure,
			expires,
			c.Name,
			c.Value,
		))
	}

	return os.WriteFile(filePath, []byte(sb.String()), 0644)
}
