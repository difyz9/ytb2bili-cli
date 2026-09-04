package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/zolagz/ytb2bili-go/internal/auth"
	"github.com/zolagz/ytb2bili-go/internal/bili"
	"github.com/zolagz/ytb2bili-go/internal/config"
	"github.com/zolagz/ytb2bili-go/internal/download"
	"github.com/zolagz/ytb2bili-go/internal/pipeline"
	"github.com/zolagz/ytb2bili-go/internal/storage"
)

// subtitleWatchLogger implements bili.SubtitleWatchLogger for server-side log.Printf output
type subtitleWatchLogger struct{}

func (subtitleWatchLogger) Printf(format string, args ...interface{}) {
	log.Printf(format, args...)
}

// Server HTTP 服务器
type Server struct {
	cfg      *config.Config
	history  *storage.HistoryStore
	server   *http.Server
	Feishu   *FeishuBot
	taskChan chan *VideoTask
	jobs     *jobStore // 任务持久化（重启回放，Phase 4 M2）
	mu       sync.Mutex
}

// VideoTask 视频处理任务
type VideoTask struct {
	ID                      string          `json:"id"`
	URL                     string          `json:"url"`
	Title                   string          `json:"title"`
	Description             string          `json:"description,omitempty"`
	Subtitles               []SubtitleEntry `json:"subtitles,omitempty"`
	Cookies                 string          `json:"cookies,omitempty"`
	Status                  string          `json:"status"`
	CreatedAt               string          `json:"created_at"`
	UpdatedAt               string          `json:"updated_at"`
	BVID                    string          `json:"bvid,omitempty"`
	Error                   string          `json:"error,omitempty"`
	Chain                   []string        `json:"chain,omitempty"`
	DryRun                  bool            `json:"dry_run,omitempty"`
	SkipTranslate           bool            `json:"skip_translate,omitempty"`
	Planner                 string          `json:"planner,omitempty"`
	Goal                    string          `json:"goal,omitempty"`
	SourceLang              string          `json:"source_lang,omitempty"`
	TargetLang              string          `json:"target_lang,omitempty"`
	AudioDir                string          `json:"audio_dir,omitempty"`
	DisableAudioSpeedAdjust bool            `json:"disable_audio_speed_adjust,omitempty"`
	AudioMissingMode        string          `json:"audio_missing_mode,omitempty"`
}

// SubtitleEntry 字幕条目
type SubtitleEntry struct {
	Text     string  `json:"text"`
	Duration float64 `json:"duration"`
	Offset   float64 `json:"offset"`
	Lang     string  `json:"lang"`
}

// SubmitRequest 提交请求
type SubmitRequest struct {
	URL                     string          `json:"url"`
	Title                   string          `json:"title"`
	Description             string          `json:"description,omitempty"`
	Operation               string          `json:"operationType,omitempty"`
	Subtitles               []SubtitleEntry `json:"subtitles,omitempty"`
	PlaylistID              string          `json:"playlistId,omitempty"`
	Timestamp               string          `json:"timestamp,omitempty"`
	Meta                    string          `json:"meta,omitempty"`  // 加密的 cookies
	Chain                   []string        `json:"chain,omitempty"` // 可选任务链；依赖自动补全
	DryRun                  bool            `json:"dryRun,omitempty"`
	SkipTranslate           bool            `json:"skipTranslate,omitempty"`
	Planner                 string          `json:"planner,omitempty"`
	Goal                    string          `json:"goal,omitempty"`
	SourceLang              string          `json:"sourceLang,omitempty"`
	TargetLang              string          `json:"targetLang,omitempty"`
	AudioDir                string          `json:"audioDir,omitempty"`
	DisableAudioSpeedAdjust bool            `json:"disableAudioSpeedAdjust,omitempty"`
	AudioMissingMode        string          `json:"audioMissingMode,omitempty"`
}

// SubmitResponse 提交响应
type SubmitResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	TaskID  string `json:"task_id,omitempty"`
}

// VideoSubmitData Chrome 插件提交的视频数据
type VideoSubmitData struct {
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

// New 创建服务器
func New(cfg *config.Config) *Server {
	historyDir := cfg.DataDir + "/history"
	h := storage.NewHistoryStore(historyDir)

	s := &Server{
		cfg:      cfg,
		history:  h,
		taskChan: make(chan *VideoTask, 100),
		jobs:     newJobStore(cfg.DataDir),
	}

	// 启动任务处理器
	go s.processTasks()
	// 重启回放：恢复上次服务中断时未完成的任务
	go s.replayPendingJobs()

	return s
}

// persistTask 把任务状态落盘（best-effort，失败仅告警，不中断处理）。
func (s *Server) persistTask(t *VideoTask) {
	if s == nil || s.jobs == nil || t == nil {
		return
	}
	if err := s.jobs.save(t); err != nil {
		log.Printf("⚠ 任务持久化失败 %s: %v", t.ID, err)
	}
}

// replayPendingJobs 启动时把上次未完成（非 completed/failed）的任务重新入队。
// 重复投稿已由 pipeline 顶部 history 守卫 + pending.jsonl 兜底，安全重跑。
func (s *Server) replayPendingJobs() {
	if s == nil || s.jobs == nil {
		return
	}
	pending, err := s.jobs.replayPending()
	if err != nil {
		log.Printf("⚠ 恢复未完成任务失败: %v", err)
		return
	}
	if len(pending) == 0 {
		return
	}
	log.Printf("♻️ 恢复 %d 个未完成任务（上次服务中断）", len(pending))
	for _, t := range pending {
		log.Printf("♻️ 重新入队: %s - %s", t.ID, t.URL)
		s.taskChan <- t
	}
}

// Start 启动服务器
func (s *Server) Start(addr string) error {
	if !isLoopbackAddr(addr) && s.cfg.ServerToken == "" {
		return fmt.Errorf("拒绝在非本机地址 %q 上启动：请配置 YTB2BILI_SERVER_TOKEN", addr)
	}
	mux := http.NewServeMux()

	// API 路由
	mux.HandleFunc("/api/v1/submit", s.handleSubmit)
	mux.HandleFunc("/api/v1/tasks", s.handleListTasks)
	mux.HandleFunc("/api/v1/tasks/", s.handleGetTask)
	mux.HandleFunc("/api/v1/history", s.handleHistory)
	mux.HandleFunc("/health", s.handleHealth)

	// 飞书机器人路由
	mux.HandleFunc("/feishu/webhook", s.feishuWebhook)

	// Apply middleware: cors → auth → timeout
	handler := requestTimeoutMiddleware(
		corsMiddleware(s.cfg.AllowedOrigins,
			apiAuthMiddleware(s.cfg.ServerToken, mux),
		),
	)

	s.server = &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Printf("🚀 服务器启动在 %s", addr)
	log.Printf("   API: http://%s/api/v1/submit", addr)
	log.Printf("   健康检查: http://%s/health", addr)

	// 启动飞书机器人
	if s.Feishu != nil {
		go func() {
			ctx := context.Background()
			if err := s.Feishu.Start(ctx); err != nil {
				log.Printf("❌ 飞书机器人启动失败: %v", err)
			}
		}()

		// 处理飞书消息
		go s.handleFeishuMessages()
	}

	// 恢复上次未完成的字幕监听（重启后继续等待审核）
	go s.resumePendingSubtitleWatches()

	// Graceful shutdown: listen for signals in a goroutine
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		log.Printf("🛑 接收到信号 %v，开始优雅关闭...", sig)

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		if err := s.Stop(ctx); err != nil {
			log.Printf("❌ 服务器关闭失败: %v", err)
		}
	}()

	return s.server.ListenAndServe()
}

// handleFeishuMessages 处理飞书消息
func (s *Server) handleFeishuMessages() {
	for msg := range s.Feishu.GetMessageChan() {
		go s.processFeishuMessage(msg)
	}
}

// processFeishuMessage 处理单条飞书消息
func (s *Server) processFeishuMessage(msg *FeishuMessage) {
	content := strings.TrimSpace(msg.Content)

	// 尝试解析 JSON 数据（来自 Chrome 插件）
	if strings.HasPrefix(content, "{") && strings.HasSuffix(content, "}") {
		var submitData VideoSubmitData
		if err := json.Unmarshal([]byte(content), &submitData); err == nil && submitData.Type == "video_submit" {
			s.handleVideoSubmitFromExtension(msg, &submitData)
			return
		}
	}

	// 解析命令
	switch {
	case content == "help" || content == "帮助":
		s.Feishu.ReplyCard(context.Background(), msg, CreateHelpCard())

	case content == "history" || content == "历史":
		videos, _ := s.history.List()
		var videoList []map[string]string
		for _, v := range videos {
			videoList = append(videoList, map[string]string{
				"title":        v.Title,
				"youtube_url":  fmt.Sprintf("https://www.youtube.com/watch?v=%s", v.YouTubeID),
				"bilibili_url": fmt.Sprintf("https://www.bilibili.com/video/%s", v.BVID),
			})
		}
		s.Feishu.ReplyCard(context.Background(), msg, CreateHistoryCard(videoList))

	case content == "status" || content == "状态":
		videos, _ := s.history.List()
		s.Feishu.ReplyCard(context.Background(), msg, CreateStatusCard(0, len(videos)))

	default:
		// 尝试解析 YouTube URL
		if youtubeURL := ParseYouTubeURL(content); youtubeURL != "" {
			s.handleYouTubeSubmission(msg, youtubeURL, "")
		} else {
			s.Feishu.ReplyMarkdown(context.Background(), msg, "发送 YouTube 视频链接即可自动处理\n\n输入 `help` 查看帮助")
		}
	}
}

// handleVideoSubmitFromExtension 处理来自 Chrome 插件的视频提交
func (s *Server) handleVideoSubmitFromExtension(msg *FeishuMessage, data *VideoSubmitData) {
	youtubeURL := data.Data.URL
	videoID := data.Data.VideoID
	title := data.Data.Title
	cookies := data.Data.Cookies

	log.Printf("📥 收到 Chrome 插件提交: %s - %s", title, youtubeURL)

	// 检查是否已提交
	if s.history.IsSubmitted(videoID) {
		submitted := s.history.GetSubmitted(videoID)
		s.Feishu.ReplyMarkdown(context.Background(), msg, fmt.Sprintf("⚠️ 该视频已提交过\n\nB站链接: https://www.bilibili.com/video/%s\n提交时间: %s", submitted.BVID, submitted.SubmittedAt[:19]))
		return
	}

	// 发送处理中卡片
	card := CreateTaskCard("pending", "正在处理...", "processing", "")
	s.Feishu.ReplyCard(context.Background(), msg, card)

	// 创建任务
	task := &VideoTask{
		ID:        generateTaskID(),
		URL:       youtubeURL,
		Title:     title,
		Cookies:   cookies,
		Status:    "pending",
		CreatedAt: time.Now().Format(time.RFC3339),
		UpdatedAt: time.Now().Format(time.RFC3339),
	}

	// 先持久化再入队：服务中途重启不丢任务
	if err := s.jobs.save(task); err != nil {
		s.Feishu.ReplyMessage(context.Background(), msg, "❌ 任务持久化失败，请稍后重试")
		log.Printf("❌ 任务持久化失败: %v", err)
		return
	}

	// 发送到任务队列
	select {
	case s.taskChan <- task:
		log.Printf("📥 新任务: %s - %s", task.ID, task.URL)
	default:
		s.Feishu.ReplyMessage(context.Background(), msg, "❌ 任务队列已满，请稍后重试")
	}
}

// handleYouTubeSubmission 处理 YouTube 提交
func (s *Server) handleYouTubeSubmission(msg *FeishuMessage, youtubeURL string, cookies string) {
	// 提取视频 ID
	videoID := extractVideoID(youtubeURL)
	if videoID == "" {
		s.Feishu.ReplyMessage(context.Background(), msg, "❌ 无效的 YouTube URL")
		return
	}

	// 检查是否已提交
	if s.history.IsSubmitted(videoID) {
		submitted := s.history.GetSubmitted(videoID)
		s.Feishu.ReplyMarkdown(context.Background(), msg, fmt.Sprintf("⚠️ 该视频已提交过\n\nB站链接: https://www.bilibili.com/video/%s\n提交时间: %s", submitted.BVID, submitted.SubmittedAt[:19]))
		return
	}

	// 发送处理中卡片
	card := CreateTaskCard("pending", "正在处理...", "processing", "")
	s.Feishu.ReplyCard(context.Background(), msg, card)

	// 创建任务
	task := &VideoTask{
		ID:        generateTaskID(),
		URL:       youtubeURL,
		Title:     "待处理",
		Cookies:   cookies,
		Status:    "pending",
		CreatedAt: time.Now().Format(time.RFC3339),
		UpdatedAt: time.Now().Format(time.RFC3339),
	}

	// 先持久化再入队：服务中途重启不丢任务
	if err := s.jobs.save(task); err != nil {
		s.Feishu.ReplyMessage(context.Background(), msg, "❌ 任务持久化失败，请稍后重试")
		log.Printf("❌ 任务持久化失败: %v", err)
		return
	}

	// 发送到任务队列
	select {
	case s.taskChan <- task:
		log.Printf("📥 新任务: %s - %s", task.ID, task.URL)
	default:
		s.Feishu.ReplyMessage(context.Background(), msg, "❌ 任务队列已满，请稍后重试")
	}
}

// processTasks 处理任务队列
func (s *Server) processTasks() {
	for task := range s.taskChan {
		s.processVideoTask(task)
	}
}

// processVideoTask runs every HTTP/Feishu task through the shared application pipeline.
func (s *Server) processVideoTask(task *VideoTask) {
	log.Printf("🎬 开始处理任务: %s - %s", task.ID, task.URL)
	s.persistTask(task)
	cookiesPath := ""
	if task.Cookies != "" {
		if path, err := download.SaveCookiesFromMeta(task.Cookies, s.cfg.DataDir); err == nil {
			cookiesPath = path
		} else {
			log.Printf("⚠️ 解析任务 cookies 失败，将使用全局 cookies: %v", err)
		}
	}
	processor := &pipeline.Processor{Config: s.cfg, Reporter: func(event pipeline.Event) {
		task.Status = event.Step
		task.UpdatedAt = time.Now().Format(time.RFC3339)
		if event.Status == "running" {
			log.Printf("[%d/%d] %s...", event.Position, event.Total, event.Step)
		} else if event.Err != nil {
			log.Printf("❌ %s: %v", event.Step, event.Err)
			task.Error = event.Err.Error()
		} else {
			log.Printf("✅ %s", event.Step)
		}
		s.persistTask(task)
	}}
	result, err := processor.Process(context.Background(), pipeline.Request{
		URL: task.URL, SourceLang: task.SourceLang, TargetLang: task.TargetLang, Tid: s.cfg.BiliTid,
		Source: "server", CookiesPath: cookiesPath, Chain: task.Chain,
		DryRun: task.DryRun, SkipTranslate: task.SkipTranslate,
		Planner: task.Planner, Goal: task.Goal,
		TaskID:   task.ID,
		AudioDir: task.AudioDir, DisableAudioSpeedAdjust: task.DisableAudioSpeedAdjust,
		AudioMissingMode: task.AudioMissingMode,
	})
	if err != nil {
		task.Status, task.Error = "failed", err.Error()
		// 补偿：回放时若该视频其实已投稿成功（history 守卫拦下的重跑），按已完成为准，避免误标失败
		if strings.Contains(err.Error(), "已提交过") {
			if videoID := extractVideoID(task.URL); videoID != "" {
				if sub := s.history.GetSubmitted(videoID); sub != nil {
					task.Status, task.BVID, task.Error = "completed", sub.BVID, ""
				}
			}
		}
		s.persistTask(task)
		store := storage.NewTaskStore(filepath.Join(s.cfg.DataDir, "tasks"))
		if persisted, getErr := store.Get(task.ID); getErr == nil && persisted.Status != "failed" {
			if uerr := store.UpdateStep(task.ID, "planning", "failed", err.Error()); uerr != nil {
				log.Printf("⚠ UpdateStep(%s, planning, failed): %v", task.ID, uerr)
			}
		}
		log.Printf("❌ 任务失败: %s: %v", task.ID, err)
		return
	}
	task.Status, task.BVID = "completed", result.BVID
	s.persistTask(task)
	if result.BVID == "" {
		return
	}
	var cred auth.LoginInfo
	if err := storage.NewCredentialStore(filepath.Join(s.cfg.DataDir, "cookies")).Load(&cred); err == nil {
					go bili.WatchAndUploadSubtitle(result.BVID, result.ArtifactID(), result.DownloadDir, &cred, s.cfg.DataDir, subtitleWatchLogger{})
	}
}

// Stop 停止服务器
func (s *Server) Stop(ctx context.Context) error {
	if s.Feishu != nil {
		s.Feishu.Stop(ctx)
	}
	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}

// resumePendingSubtitleWatches 启动时恢复所有待审核的字幕监听
func (s *Server) resumePendingSubtitleWatches() {
	subStore := storage.NewSubtitleStore(filepath.Join(s.cfg.DataDir, "subtitles"))
	pending := subStore.ListPendingVideos()
	if len(pending) == 0 {
		return
	}

	log.Printf("🔍 检测到 %d 个视频有待上传字幕，恢复监听审核状态...", len(pending))

	// 加载 B站凭证
	credDir := filepath.Join(s.cfg.DataDir, "cookies")
	cs := storage.NewCredentialStore(credDir)
	var cred auth.LoginInfo
	if err := cs.Load(&cred); err != nil {
		log.Printf("⚠️ 恢复字幕监听失败：无法加载 B站凭证 (%v)", err)
		return
	}

	for _, videoID := range pending {
		tracks, _ := subStore.GetStatus(videoID)
		if len(tracks) == 0 {
			continue
		}
		bvid := tracks[0].BVID
		if bvid == "" {
			continue
		}
		// 提取下载目录（从第一条字幕文件的路径推断）
		dlDir := filepath.Dir(tracks[0].FilePath)
		log.Printf("🔄 恢复字幕监听: %s (BVID=%s, %d 个字幕待上传)", videoID, bvid, len(tracks))
			go bili.WatchAndUploadSubtitle(bvid, videoID, dlDir, &cred, s.cfg.DataDir, subtitleWatchLogger{})
	}
}

// handleSubmit 处理视频提交
func (s *Server) handleSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			s.jsonError(w, "请求体过大", http.StatusRequestEntityTooLarge)
			return
		}
		s.jsonError(w, "读取请求失败", http.StatusBadRequest)
		return
	}

	var req SubmitRequest
	if err := json.Unmarshal(body, &req); err != nil {
		s.jsonError(w, "解析请求失败", http.StatusBadRequest)
		return
	}

	// 验证 URL
	if req.URL == "" {
		s.jsonError(w, "URL 不能为空", http.StatusBadRequest)
		return
	}

	// 提取视频 ID
	videoID := extractVideoID(req.URL)
	if videoID == "" {
		s.jsonError(w, "无效的 YouTube URL", http.StatusBadRequest)
		return
	}

	// 检查是否已提交
	if s.history.IsSubmitted(videoID) {
		s.jsonError(w, "该视频已提交过", http.StatusConflict)
		return
	}

	// 创建任务
	task := &VideoTask{
		ID:                      generateTaskID(),
		URL:                     req.URL,
		Title:                   req.Title,
		Description:             req.Description,
		Subtitles:               req.Subtitles,
		Cookies:                 req.Meta,
		Chain:                   req.Chain,
		DryRun:                  req.DryRun,
		SkipTranslate:           req.SkipTranslate,
		Planner:                 req.Planner,
		Goal:                    req.Goal,
		SourceLang:              req.SourceLang,
		TargetLang:              req.TargetLang,
		AudioDir:                req.AudioDir,
		DisableAudioSpeedAdjust: req.DisableAudioSpeedAdjust,
		AudioMissingMode:        req.AudioMissingMode,
		Status:                  "pending",
		CreatedAt:               time.Now().Format(time.RFC3339),
		UpdatedAt:               time.Now().Format(time.RFC3339),
	}

	// 先持久化再入队：服务中途重启不丢任务（与下方 tasks 台账分开）
	if err := s.jobs.save(task); err != nil {
		s.jsonError(w, "任务持久化失败", http.StatusInternalServerError)
		return
	}

	// 发送到任务队列
	taskStore := storage.NewTaskStore(filepath.Join(s.cfg.DataDir, "tasks"))
	if err := taskStore.Persist(taskStore.Prepare(task.ID, task.URL), nil); err != nil {
		s.jsonError(w, "创建任务失败", http.StatusInternalServerError)
		return
	}
	select {
	case s.taskChan <- task:
		s.jsonResponse(w, SubmitResponse{
			Success: true,
			Message: "任务已提交",
			TaskID:  task.ID,
		})
	default:
		if err := taskStore.Delete(task.ID); err != nil {
			log.Printf("warning: failed to clean up task %s: %v", task.ID, err)
		}
		if err := s.jobs.remove(task.ID); err != nil {
			log.Printf("warning: failed to clean up job %s: %v", task.ID, err)
		}
		s.jsonError(w, "任务队列已满", http.StatusServiceUnavailable)
	}
}

// handleListTasks 处理任务列表
func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tasks := storage.NewTaskStore(filepath.Join(s.cfg.DataDir, "tasks")).List()
	s.jsonResponse(w, tasks)
}

// handleGetTask 处理获取任务
func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/tasks/")
	if id == "" || strings.ContainsAny(id, `/\\`) {
		s.jsonError(w, "无效任务 ID", http.StatusBadRequest)
		return
	}
	task, err := storage.NewTaskStore(filepath.Join(s.cfg.DataDir, "tasks")).Get(id)
	if err != nil {
		if os.IsNotExist(err) {
			s.jsonError(w, "任务不存在", http.StatusNotFound)
			return
		}
		s.jsonError(w, "读取任务失败", http.StatusInternalServerError)
		return
	}
	s.jsonResponse(w, task)
}

// handleHistory 处理历史记录
func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	videos, err := s.history.List()
	if err != nil {
		s.jsonError(w, "获取历史记录失败", http.StatusInternalServerError)
		return
	}
	s.jsonResponse(w, videos)
}

// handleHealth 处理健康检查
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.jsonResponse(w, map[string]string{"status": "ok"})
}

// feishuWebhook 处理飞书 Webhook
func (s *Server) feishuWebhook(w http.ResponseWriter, r *http.Request) {
	// TODO: 实现飞书 Webhook
	s.jsonResponse(w, map[string]string{"status": "ok"})
}

// jsonResponse 返回 JSON 响应
func (s *Server) jsonResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// jsonError 返回 JSON 错误响应
func (s *Server) jsonError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// generateTaskID 生成任务 ID
func generateTaskID() string {
	return fmt.Sprintf("task_%d", time.Now().UnixNano())
}

// extractVideoID 提取视频 ID
func extractVideoID(url string) string {
	// youtube.com/watch?v=xxx
	if strings.Contains(url, "youtube.com/watch?v=") {
		parts := strings.Split(url, "v=")
		if len(parts) > 1 {
			id := strings.Split(parts[1], "&")[0]
			if len(id) == 11 {
				return id
			}
		}
	}

	// youtu.be/xxx
	if strings.Contains(url, "youtu.be/") {
		parts := strings.Split(url, "youtu.be/")
		if len(parts) > 1 {
			id := strings.Split(parts[1], "?")[0]
			if len(id) == 11 {
				return id
			}
		}
	}

	// youtube.com/shorts/xxx
	if strings.Contains(url, "youtube.com/shorts/") {
		parts := strings.Split(url, "shorts/")
		if len(parts) > 1 {
			id := strings.Split(parts[1], "?")[0]
			if len(id) == 11 {
				return id
			}
		}
	}

	return ""
}

// corsMiddleware 添加 CORS 头，允许 Chrome 扩展内容脚本跨域请求
//
// 当 allowedOrigins 为空时（默认配置），允许任意 Origin——本地服务仅监听
// 127.0.0.1，外部无法访问，放宽 CORS 是安全的。当配置了明确的允许来源列表
// 时，仅放行列表内的 Origin。
func corsMiddleware(allowedOrigins []string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		if origin != "" && len(allowedOrigins) > 0 {
			allowed := false
			for _, candidate := range allowedOrigins {
				if candidate == origin {
					allowed = true
					break
				}
			}
			if !allowed {
				http.Error(w, "origin not allowed", http.StatusForbidden)
				return
			}
		}

		// 无白名单（默认）= 允许所有来源；有白名单 = 仅放行匹配的
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// 处理 OPTIONS 预检请求
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func apiAuthMiddleware(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token == "" || r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}
		provided := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if len(provided) != len(token) || subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// requestTimeoutMiddleware bounds slow handlers with a per-request timeout.
// handler 在独立 goroutine 执行；超时后向响应写入 503。
// 通过 singleWriter 保证"业务响应"与"超时 503"之间只有一个写入者生效：
// 一旦任一方已开始写（或已发 503），另一方迟到的写入被丢弃，避免双写竞态。
// 注意：context 取消无法强制终止不配合的 handler，长业务应自行检查 r.Context()。
func requestTimeoutMiddleware(next http.Handler) http.Handler {
	return requestTimeoutMiddlewareWith(next, 5*time.Minute)
}

func requestTimeoutMiddlewareWith(next http.Handler, timeout time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip timeout for health checks and long-lived endpoints
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		r = r.WithContext(ctx)

		tw := &singleWriter{ResponseWriter: w}
		done := make(chan struct{})
		go func() {
			defer close(done)
			next.ServeHTTP(tw, r)
		}()

		select {
		case <-done:
			return
		case <-ctx.Done():
			if ctx.Err() == context.DeadlineExceeded {
				log.Printf("⏰ 请求超时: %s %s", r.Method, r.URL.Path)
				tw.timeout503()
			}
		}
	})
}

// singleWriter 包装 ResponseWriter：业务响应与超时 503 之间只有一次生效。
// stopped=true 表示超时路径已写 503，随后 handler 的迟到写入被丢弃；
// started=true 表示 handler 已开始提交响应，超时路径不得再写 503。
type singleWriter struct {
	http.ResponseWriter
	mu      sync.Mutex
	started bool // handler 已开始响应
	stopped bool // 超时 503 已占用
}

func (w *singleWriter) WriteHeader(code int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopped {
		return // 503 已发出：丢弃迟到的业务响应头
	}
	w.started = true
	w.ResponseWriter.WriteHeader(code)
}

func (w *singleWriter) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopped {
		return len(b), nil // 503 已发出：丢弃迟到业务正文，假装成功避免 handler 误判
	}
	w.started = true
	return w.ResponseWriter.Write(b)
}

func (w *singleWriter) timeout503() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.started {
		return // handler 已在写响应，无法再改状态码
	}
	w.stopped = true
	w.ResponseWriter.WriteHeader(http.StatusServiceUnavailable)
	_, _ = w.ResponseWriter.Write([]byte(`{"error":"请求超时"}`))
}
