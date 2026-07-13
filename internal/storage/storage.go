package storage

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ─── Task ────────────────────────────────────────────────────────────────────

type Task struct {
	ID        string            `json:"id"`
	Status    string            `json:"status"`
	SourceURL string            `json:"source_url"`
	Title     string            `json:"title"`
	BVID      string            `json:"bvid,omitempty"`
	CreatedAt string            `json:"created_at"`
	UpdatedAt string            `json:"updated_at"`
	Steps     map[string]Step   `json:"steps"`
	Result    map[string]string `json:"result"`
}

type Step struct {
	Status      string `json:"status"`
	StartedAt   string `json:"started_at,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`
	Error       string `json:"error,omitempty"`
}

type TaskStore struct {
	dir string
	mu  sync.Mutex
}

func NewTaskStore(dir string) *TaskStore {
	os.MkdirAll(dir, 0755)
	return &TaskStore{dir: dir}
}

func (s *TaskStore) path(id string) string {
	return filepath.Join(s.dir, id+".json")
}

func randID() string {
	b := make([]byte, 6)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *TaskStore) Create(url string) *Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().Format(time.RFC3339)
	task := &Task{
		ID:        randID(),
		Status:    "pending",
		SourceURL: url,
		CreatedAt: now,
		UpdatedAt: now,
		Steps: map[string]Step{
			"download":   {},
			"transcribe": {},
			"translate":  {},
			"metadata":   {},
			"upload":     {},
		},
		Result: make(map[string]string),
	}
	s.save(task)
	return task
}

func (s *TaskStore) Get(id string) (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path(id))
	if err != nil {
		return nil, err
	}
	var t Task
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *TaskStore) save(task *Task) {
	task.UpdatedAt = time.Now().Format(time.RFC3339)
	data, _ := json.MarshalIndent(task, "", "  ")
	os.WriteFile(s.path(task.ID), data, 0644)
}

func (s *TaskStore) UpdateStep(id, stepName, status string, errMsg ...string) {
	t, err := s.Get(id)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	step := t.Steps[stepName]
	now := time.Now().Format(time.RFC3339)
	step.Status = status
	if status == "running" && step.StartedAt == "" {
		step.StartedAt = now
	}
	if status == "completed" || status == "failed" {
		step.CompletedAt = now
	}
	if len(errMsg) > 0 {
		step.Error = errMsg[0]
	}
	t.Steps[stepName] = step
	if status == "failed" {
		t.Status = "failed"
	}
	t.UpdatedAt = now
	s.save(t)
}

// SetBVID 设置任务的 BVID
func (s *TaskStore) SetBVID(id, bvid string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := s.Get(id)
	if err != nil {
		return
	}
	t.BVID = bvid
	t.UpdatedAt = time.Now().Format(time.RFC3339)
	s.save(t)
}

// SetCompleted 标记任务完成
func (s *TaskStore) SetCompleted(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := s.Get(id)
	if err != nil {
		return
	}
	t.Status = "completed"
	t.UpdatedAt = time.Now().Format(time.RFC3339)
	s.save(t)
}

func (s *TaskStore) List() []*Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, _ := os.ReadDir(s.dir)
	var tasks []*Task
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			data, err := os.ReadFile(filepath.Join(s.dir, e.Name()))
			if err != nil {
				continue
			}
			var t Task
			json.Unmarshal(data, &t)
			tasks = append(tasks, &t)
		}
	}
	return tasks
}

// ─── Credential ──────────────────────────────────────────────────────────────

type CredentialStore struct {
	path string
}

func NewCredentialStore(dir string) *CredentialStore {
	os.MkdirAll(dir, 0755)
	return &CredentialStore{path: filepath.Join(dir, "bilibili.json")}
}

func (c *CredentialStore) Save(cred interface{}) error {
	data, _ := json.MarshalIndent(cred, "", "  ")
	return os.WriteFile(c.path, data, 0644)
}

func (c *CredentialStore) Load(v interface{}) error {
	data, err := os.ReadFile(c.path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func (c *CredentialStore) Exists() bool {
	_, err := os.Stat(c.path)
	return err == nil
}

func (c *CredentialStore) Delete() {
	os.Remove(c.path)
}

// ─── Subtitle Track ──────────────────────────────────────────────────────────

const (
	SubtitleStatusPending  = "pending"
	SubtitleStatusUploaded = "uploaded"
	SubtitleStatusFailed   = "failed"
	SubtitleStatusMissing  = "missing"
)

// SubtitleTrack 追踪单个字幕文件到B站的上传状态
type SubtitleTrack struct {
	VideoID    string `json:"video_id"`
	BVID       string `json:"bvid"`
	FilePath   string `json:"file_path"`
	FileName   string `json:"file_name"`
	Language   string `json:"language"`
	Status     string `json:"status"` // pending / uploaded / failed / missing
	Error      string `json:"error,omitempty"`
	RetryCount int    `json:"retry_count"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
	UploadedAt string `json:"uploaded_at,omitempty"`
}

// SubtitleStore 持久化字幕上传状态
type SubtitleStore struct {
	dir string
	mu  sync.Mutex
}

func NewSubtitleStore(dir string) *SubtitleStore {
	os.MkdirAll(dir, 0755)
	return &SubtitleStore{dir: dir}
}

func (s *SubtitleStore) path(videoID string) string {
	return filepath.Join(s.dir, videoID+".json")
}

// BuildSubtitleCandidates 扫描下载目录中的所有 SRT 字幕文件
// 自动识别语言后缀，支持：.en.srt, .zh.srt, .zh-Hans.srt, .ja.srt 等
func BuildSubtitleCandidates(videoID, dlDir string) []SubtitleTrack {
	if dlDir == "" {
		return nil
	}

	entries, err := os.ReadDir(dlDir)
	if err != nil {
		return nil
	}

	// 语言后缀 → language code 映射
	// 注意：翻译后的文件是 .en.zh.srt 格式，.zh.srt 需要优先匹配
	langSuffix := map[string]string{
		".en.zh.srt":   "zh",   // 翻译结果: en.zh.srt → zh
		".zh.srt":      "zh",
		".zh-hans.srt": "zh",
		".zh-hant.srt": "zh-TW",
		".en.srt":      "en",
		".ja.srt":      "ja",
	}

	seen := make(map[string]bool)
	var tracks []SubtitleTrack

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".srt") {
			continue
		}
		name := strings.ToLower(e.Name())

		var language string
		for suffix, lang := range langSuffix {
			if strings.HasSuffix(name, suffix) {
				language = lang
				break
			}
		}
		if language == "" {
			continue
		}
		if seen[language] {
			continue // 只保留每种语言的第一个
		}
		seen[language] = true

		filePath := filepath.Join(dlDir, e.Name())
		tracks = append(tracks, SubtitleTrack{
			VideoID:  videoID,
			FilePath: filePath,
			FileName: e.Name(),
			Language: language,
			Status:   SubtitleStatusPending,
		})
	}

	return tracks
}

// SyncFromDownload 扫描下载目录，创建/更新字幕追踪记录
func (s *SubtitleStore) SyncFromDownload(videoID, bvid, dlDir string) ([]SubtitleTrack, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Format(time.RFC3339)
	candidates := BuildSubtitleCandidates(videoID, dlDir)

	var current []SubtitleTrack

	// 尝试加载已有记录
	existing := make(map[string]*SubtitleTrack)
	if data, err := os.ReadFile(s.path(videoID)); err == nil {
		var prev []SubtitleTrack
		if json.Unmarshal(data, &prev) == nil {
			for i := range prev {
				existing[prev[i].Language] = &prev[i]
			}
		}
	}

	for _, c := range candidates {
		track := c
		track.BVID = bvid
		track.UpdatedAt = now

		if prev, ok := existing[c.Language]; ok {
			// 保留已上传/已失败的状态
			if prev.Status == SubtitleStatusUploaded {
				track.Status = SubtitleStatusUploaded
				track.UploadedAt = prev.UploadedAt
			} else if prev.Status == SubtitleStatusFailed {
				track.Status = SubtitleStatusPending // 重置为待重试
				track.RetryCount = prev.RetryCount
			}
			track.CreatedAt = prev.CreatedAt
		} else {
			track.CreatedAt = now
		}
		current = append(current, track)
	}

	s.saveTracks(videoID, current)
	return current, nil
}

// MarkUploaded 标记字幕上传成功
func (s *SubtitleStore) MarkUploaded(videoID, language string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tracks, err := s.loadTracks(videoID)
	if err != nil {
		return fmt.Errorf("加载字幕记录失败: %w", err)
	}

	now := time.Now().Format(time.RFC3339)
	found := false
	for i := range tracks {
		if tracks[i].Language == language {
			tracks[i].Status = SubtitleStatusUploaded
			tracks[i].Error = ""
			tracks[i].UploadedAt = now
			tracks[i].UpdatedAt = now
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("未找到语言 %s 的字幕记录", language)
	}

	return s.saveTracks(videoID, tracks)
}

// MarkFailed 标记字幕上传失败
func (s *SubtitleStore) MarkFailed(videoID, language string, errMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tracks, err := s.loadTracks(videoID)
	if err != nil {
		return fmt.Errorf("加载字幕记录失败: %w", err)
	}

	now := time.Now().Format(time.RFC3339)
	for i := range tracks {
		if tracks[i].Language == language {
			tracks[i].Status = SubtitleStatusFailed
			tracks[i].Error = errMsg
			tracks[i].RetryCount++
			tracks[i].UpdatedAt = now
			break
		}
	}
	return s.saveTracks(videoID, tracks)
}

// GetPending 获取待上传的字幕列表
func (s *SubtitleStore) GetPending(videoID string) []SubtitleTrack {
	s.mu.Lock()
	defer s.mu.Unlock()

	tracks, err := s.loadTracks(videoID)
	if err != nil {
		return nil
	}

	var pending []SubtitleTrack
	for _, t := range tracks {
		if t.Status == SubtitleStatusPending || t.Status == SubtitleStatusFailed {
			pending = append(pending, t)
		}
	}
	return pending
}

// AllUploaded 检查该视频的所有字幕是否都已上传成功
func (s *SubtitleStore) AllUploaded(videoID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	tracks, err := s.loadTracks(videoID)
	if err != nil || len(tracks) == 0 {
		return false
	}

	for _, t := range tracks {
		if t.Status != SubtitleStatusUploaded {
			return false
		}
	}
	return true
}

// GetStatus 获取某个视频的字幕整体状态
func (s *SubtitleStore) GetStatus(videoID string) ([]SubtitleTrack, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tracks, err := s.loadTracks(videoID)
	if err != nil {
		return nil, false
	}

	allDone := true
	for _, t := range tracks {
		if t.Status != SubtitleStatusUploaded && t.Status != SubtitleStatusMissing {
			allDone = false
			break
		}
	}
	return tracks, allDone
}

func (s *SubtitleStore) loadTracks(videoID string) ([]SubtitleTrack, error) {
	data, err := os.ReadFile(s.path(videoID))
	if err != nil {
		return nil, err
	}
	var tracks []SubtitleTrack
	if err := json.Unmarshal(data, &tracks); err != nil {
		return nil, err
	}
	return tracks, nil
}

func (s *SubtitleStore) saveTracks(videoID string, tracks []SubtitleTrack) error {
	data, err := json.MarshalIndent(tracks, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path(videoID), data, 0644)
}


