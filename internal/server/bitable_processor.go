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
	"github.com/zolagz/ytb2bili-go/internal/feishu"
	"github.com/zolagz/ytb2bili-go/internal/pipeline"
	"github.com/zolagz/ytb2bili-go/internal/storage"
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
	cookiesPath := ""
	if task.Cookies != "" {
		outputDir := filepath.Join(p.cfg.DataDir, "downloads", task.VideoID)
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			_ = p.client.UpdateTaskStatus(p.config, task.RecordID, "failed", err.Error(), "")
			return
		}
		cookiesPath = filepath.Join(outputDir, "cookies.txt")
		if err := p.saveCookiesToFile(task.Cookies, cookiesPath); err != nil {
			log.Printf("⚠️ 保存任务 cookies 失败，将使用全局 cookies: %v", err)
			cookiesPath = ""
		}
	}
	processor := &pipeline.Processor{Config: p.cfg, Reporter: func(event pipeline.Event) {
		status := event.Step
		errMsg := ""
		if event.Err != nil {
			status, errMsg = "failed", event.Err.Error()
		}
		if err := p.client.UpdateTaskStatus(p.config, task.RecordID, status, errMsg, ""); err != nil {
			log.Printf("⚠️ 更新任务状态失败: %v", err)
		}
	}}
	result, err := processor.Process(ctx, pipeline.Request{
		URL: task.URL, SourceLang: "en", TargetLang: p.cfg.EffectiveTranslationTargetLang(), Source: "bitable",
		CookiesPath: cookiesPath, Chain: []string{"translate"}, DryRun: true,
	})
	if err != nil {
		_ = p.client.UpdateTaskStatus(p.config, task.RecordID, "failed", err.Error(), "")
		return
	}
	if err := p.client.UpdateTaskStatus(p.config, task.RecordID, "completed", "", result.BVID); err != nil {
		log.Printf("❌ 更新状态失败: %v", err)
	}
}

// saveCookiesToFile 保存 cookies 到文件
func (p *BitableProcessor) saveCookiesToFile(cookiesJSON string, filePath string) error {
	// 支持两种 cookies 格式：
	// 1. Chrome 扩展格式: expirationDate (camelCase)
	// 2. 旧格式: expire (lowercase)
	var cookies []struct {
		Name           string  `json:"name"`
		Value          string  `json:"value"`
		Domain         string  `json:"domain"`
		Path           string  `json:"path"`
		Expire         int64   `json:"expire"`
		ExpirationDate float64 `json:"expirationDate"`
		HttpOnly       bool    `json:"httpOnly"`
		Secure         bool    `json:"secure"`
		SameSite       string  `json:"sameSite"`
		Session        bool    `json:"session"`
		StoreID        string  `json:"storeId"`
		HostOnly       bool    `json:"hostOnly"`
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

	return os.WriteFile(filePath, []byte(sb.String()), 0600)
}
