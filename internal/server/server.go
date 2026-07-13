package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/zolagz/ytb2bili-go/internal/auth"
	"github.com/zolagz/ytb2bili-go/internal/bili"
	"github.com/zolagz/ytb2bili-go/internal/config"
	"github.com/zolagz/ytb2bili-go/internal/download"
	"github.com/zolagz/ytb2bili-go/internal/metadata"
	"github.com/zolagz/ytb2bili-go/internal/storage"
	"github.com/zolagz/ytb2bili-go/internal/transcriber"
	"github.com/zolagz/ytb2bili-go/internal/translator"
)

// Server HTTP 服务器
type Server struct {
	cfg      *config.Config
	history  *storage.HistoryStore
	server   *http.Server
	Feishu   *FeishuBot
	taskChan chan *VideoTask
	mu       sync.Mutex
}

// VideoTask 视频处理任务
type VideoTask struct {
	ID          string          `json:"id"`
	URL         string          `json:"url"`
	Title       string          `json:"title"`
	Description string          `json:"description,omitempty"`
	Subtitles   []SubtitleEntry `json:"subtitles,omitempty"`
	Cookies     string          `json:"cookies,omitempty"`
	Status      string          `json:"status"`
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
	BVID        string          `json:"bvid,omitempty"`
	Error       string          `json:"error,omitempty"`
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
	URL         string          `json:"url"`
	Title       string          `json:"title"`
	Description string          `json:"description,omitempty"`
	Operation   string          `json:"operationType,omitempty"`
	Subtitles   []SubtitleEntry `json:"subtitles,omitempty"`
	PlaylistID  string          `json:"playlistId,omitempty"`
	Timestamp   string          `json:"timestamp,omitempty"`
	Meta        string          `json:"meta,omitempty"` // 加密的 cookies
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
	}

	// 启动任务处理器
	go s.processTasks()

	return s
}

// Start 启动服务器
func (s *Server) Start(addr string) error {
	mux := http.NewServeMux()

	// API 路由
	mux.HandleFunc("/api/v1/submit", s.handleSubmit)
	mux.HandleFunc("/api/v1/tasks", s.handleListTasks)
	mux.HandleFunc("/api/v1/tasks/", s.handleGetTask)
	mux.HandleFunc("/api/v1/history", s.handleHistory)
	mux.HandleFunc("/health", s.handleHealth)

	// 飞书机器人路由
	mux.HandleFunc("/feishu/webhook", s.feishuWebhook)

	s.server = &http.Server{
		Addr:    addr,
		Handler: corsMiddleware(mux),
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
				"title":         v.Title,
				"youtube_url":   fmt.Sprintf("https://www.youtube.com/watch?v=%s", v.YouTubeID),
				"bilibili_url":  fmt.Sprintf("https://www.bilibili.com/video/%s", v.BVID),
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

// processVideoTask 处理单个视频任务
func (s *Server) processVideoTask(task *VideoTask) {
	log.Printf("🎬 开始处理任务: %s - %s", task.ID, task.URL)

	// 提取视频 ID 作为文件夹名
	videoID := extractVideoID(task.URL)
	if videoID == "" {
		log.Printf("❌ 无法提取视频 ID")
		task.Status = "failed"
		task.Error = "无法提取视频 ID"
		return
	}

	// 更新任务状态
	task.Status = "downloading"
	task.UpdatedAt = time.Now().Format(time.RFC3339)

	// 下载视频（使用视频 ID 作为保存文件夹）
	outputDir := s.cfg.DataDir + "/downloads/" + videoID
	cookiesPath := ""
	if task.Cookies != "" {
		// 解密 meta 加密的 cookies → 转为 Netscape 格式 → 保存到本地文件
		if path, err := download.SaveCookiesFromMeta(task.Cookies, s.cfg.DataDir ); err == nil {
			cookiesPath = path
			log.Printf("🍪 已从 meta 解密并保存 cookies: %s", path)
		} else {
			log.Printf("⚠️ 解析 meta cookies 失败: %v，将使用全局 cookies", err)
		}
	}

	result, err := download.Video(task.URL, outputDir, "en", cookiesPath)
	if err != nil {
		log.Printf("❌ 下载失败: %v", err)
		task.Status = "failed"
		task.Error = err.Error()
		return
	}

	log.Printf("✅ 下载完成: %s", result.VideoPath)

	// 更新任务状态
	task.Status = "transcribing"
	task.UpdatedAt = time.Now().Format(time.RFC3339)

	// 转录 — 使用 videoID 命名 SRT 文件
	srtPath, err := transcriber.BcutASR(result.VideoPath, outputDir, videoID)
	if err != nil {
		log.Printf("❌ 转录失败: %v", err)
		task.Status = "failed"
		task.Error = err.Error()
		return
	}

	log.Printf("✅ 转录完成: %s", srtPath)

	// 更新任务状态
	task.Status = "translating"
	task.UpdatedAt = time.Now().Format(time.RFC3339)

	// 翻译
	translator := translator.New(translator.Config{
		APIKey:     s.cfg.LLMAPIKey,
		BaseURL:    s.cfg.LLMBaseURL,
		Model:      s.cfg.LLMModel,
		SourceLang: "en",
		TargetLang: "zh",
		BatchSize:  25,
		MaxWorkers: 3,
	})

	zhSrtPath := outputDir + "/" + videoID + ".zh.srt"
	err = translator.TranslateSRTFile(context.Background(), srtPath, zhSrtPath)
	if err != nil {
		log.Printf("❌ 翻译失败: %v", err)
		task.Status = "failed"
		task.Error = err.Error()
		return
	}

	log.Printf("✅ 翻译完成: %s", zhSrtPath)

	// 生成元数据（AI 标题/简介/标签）
	task.Status = "generating_metadata"
	task.UpdatedAt = time.Now().Format(time.RFC3339)

	meta, err := metadata.Generate(result.Info, s.cfg)
	if err != nil {
		log.Printf("⚠️ 元数据生成失败: %v，回退到原始标题", err)
		meta = &metadata.VideoMeta{
			Title:       result.Info.Title,
			Description: result.Info.Description,
			Tags:        []string{},
		}
	}
	log.Printf("✅ 元数据生成完成: %s", meta.Title)

	// 加载 B站凭证
	credDir := filepath.Join(s.cfg.DataDir, "cookies")
	cs := storage.NewCredentialStore(credDir)
	var cred auth.LoginInfo
	if err := cs.Load(&cred); err != nil {
		log.Printf("❌ 加载 B站凭证失败: %v", err)
		task.Status = "failed"
		task.Error = "未登录，请先通过 CLI 执行 ytb2bili login"
		return
	}

	// 投稿到 B站
	task.Status = "uploading"
	task.UpdatedAt = time.Now().Format(time.RFC3339)

	bvid, err := bili.Upload(&cred, &bili.UploadParams{
		VideoPath: result.VideoPath,
		Title:     meta.Title,
		Desc:      meta.Description,
		Tags:      meta.Tags,
		Source:    task.URL,
		Tid:       s.cfg.BiliTid,
		CoverPath: result.CoverPath,
	})
	if err != nil {
		log.Printf("❌ 投稿到 B站失败: %v", err)
		task.Status = "failed"
		task.Error = err.Error()
		return
	}

	task.BVID = bvid
	log.Printf("✅ 投稿成功: https://www.bilibili.com/video/%s", bvid)

	// 记录到历史
	s.history.Add(&storage.SubmittedVideo{
		YouTubeID: videoID,
		BVID:      bvid,
		Title:     meta.Title,
		Channel:   "",
	})

	// 更新任务状态
	task.Status = "completed"
	task.UpdatedAt = time.Now().Format(time.RFC3339)

	log.Printf("✅ 任务完成: %s - BVID=%s", task.ID, bvid)

	// 异步监听审核并上传字幕
	dlDir := filepath.Join(s.cfg.DataDir, "downloads", videoID)
	go s.watchAndUploadSubtitle(bvid, videoID, dlDir, &cred)
}

// watchAndUploadSubtitle 异步监听B站视频审核状态，审核通过后上传字幕
func (s *Server) watchAndUploadSubtitle(bvid, videoID, dlDir string, cred *auth.LoginInfo) {
	subStore := storage.NewSubtitleStore(filepath.Join(s.cfg.DataDir, "subtitles"))

	// 同步最新字幕追踪状态
	tracks, err := subStore.SyncFromDownload(videoID, bvid, dlDir)
	if err != nil {
		log.Printf("❌ [字幕] 同步字幕追踪失败: %v", err)
		return
	}

	pending := 0
	for _, t := range tracks {
		if t.Status == storage.SubtitleStatusPending {
			pending++
		}
	}
	if pending == 0 {
		log.Printf("ℹ️ [字幕] 没有待上传的字幕文件")
		return
	}

	log.Printf("⏳ [字幕] 监听视频 %s 审核状态 (共 %d 个字幕待上传)...", bvid, pending)

	// 等待审核通过
	status, err := bili.WaitForReviewPassed(cred, bvid)
	if err != nil {
		log.Printf("❌ [字幕] 等待审核失败: %v", err)
		return
	}
	if status == nil {
		log.Printf("❌ [字幕] 获取审核状态失败")
		return
	}
	log.Printf("✅ [字幕] 视频审核通过 (state=%d)", status.State)

	// 重新同步（字幕文件可能已更新）
	subStore.SyncFromDownload(videoID, bvid, dlDir)
	pendingTracks := subStore.GetPending(videoID)
	if len(pendingTracks) == 0 {
		log.Printf("ℹ️ [字幕] 没有待上传的字幕文件")
		return
	}

	successCount := 0
	for _, track := range pendingTracks {
		log.Printf("  📤 上传字幕: %s (%s)...", track.FileName, track.Language)
		err := bili.UploadSubtitle(cred, bvid, track.FilePath, track.Language)
		if err != nil {
			log.Printf("  ❌ 字幕上传失败: %v", err)
			subStore.MarkFailed(videoID, track.Language, err.Error())
			continue
		}
		subStore.MarkUploaded(videoID, track.Language)
		log.Printf("  ✅ 字幕上传完成: %s", track.FileName)
		successCount++
	}

	if successCount > 0 {
		allDone := subStore.AllUploaded(videoID)
		if allDone {
			log.Printf("✅ [字幕] 全部字幕上传完成! https://www.bilibili.com/video/%s", bvid)
		} else {
			log.Printf("✅ [字幕] 已上传 %d 个字幕文件，部分仍待处理", successCount)
		}
	} else {
		log.Printf("❌ [字幕] 所有字幕上传均失败，请稍后重试")
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
		go s.watchAndUploadSubtitle(bvid, videoID, dlDir, &cred)
	}
}

// handleSubmit 处理视频提交
func (s *Server) handleSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
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
		ID:          generateTaskID(),
		URL:         req.URL,
		Title:       req.Title,
		Description: req.Description,
		Subtitles:   req.Subtitles,
		Cookies:     req.Meta,
		Status:      "pending",
		CreatedAt:   time.Now().Format(time.RFC3339),
		UpdatedAt:   time.Now().Format(time.RFC3339),
	}

	// 发送到任务队列
	select {
	case s.taskChan <- task:
		s.jsonResponse(w, SubmitResponse{
			Success: true,
			Message: "任务已提交",
			TaskID:  task.ID,
		})
	default:
		s.jsonError(w, "任务队列已满", http.StatusServiceUnavailable)
	}
}

// handleListTasks 处理任务列表
func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	// TODO: 实现任务列表
	s.jsonResponse(w, []VideoTask{})
}

// handleGetTask 处理获取任务
func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	// TODO: 实现获取任务
	s.jsonError(w, "Not implemented", http.StatusNotImplemented)
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
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
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