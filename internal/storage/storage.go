package storage

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

func validTaskID(id string) bool {
	return id != "" && id != "." && id != ".." && !strings.ContainsAny(id, `/\\`)
}

func randID() string {
	b := make([]byte, 6)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *TaskStore) Create(url string) *Task {
	task := s.Prepare("", url)
	_ = s.Persist(task, nil)
	return task
}

// Prepare creates an in-memory task identity without writing it to disk.
// A caller-provided id allows adapters and the pipeline to share one identity.
func (s *TaskStore) Prepare(id, url string) *Task {
	if id == "" {
		id = randID()
	}
	now := time.Now().Format(time.RFC3339)
	return &Task{
		ID:        id,
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

}

// Persist writes a prepared task after planning. Plan steps are added to the
// status map so future registered steps do not require a storage migration.
func (s *TaskStore) Persist(task *Task, plan []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if task == nil || !validTaskID(task.ID) {
		return fmt.Errorf("invalid task id")
	}
	if task.Steps == nil {
		task.Steps = make(map[string]Step)
	}
	for _, name := range plan {
		if _, ok := task.Steps[name]; !ok {
			task.Steps[name] = Step{}
		}
	}
	return s.save(task)
}

func (s *TaskStore) Get(id string) (*Task, error) {
	if !validTaskID(id) {
		return nil, fmt.Errorf("invalid task id")
	}
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

// Delete removes a task record. It is used for read-only planning, which must
// not leave an executable pending task behind.
func (s *TaskStore) Delete(id string) error {
	if !validTaskID(id) {
		return fmt.Errorf("invalid task id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return os.Remove(s.path(id))
}

func (s *TaskStore) save(task *Task) error {
	task.UpdatedAt = time.Now().Format(time.RFC3339)
	data, _ := json.MarshalIndent(task, "", "  ")
	return atomicWriteFile(s.path(task.ID), data, 0644)
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
	if status == "running" {
		t.Status = "running"
	} else if status == "failed" {
		t.Status = "failed"
	}
	t.UpdatedAt = now
	_ = s.save(t)
}

// SetBVID 设置任务的 BVID
func (s *TaskStore) SetBVID(id, bvid string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path(id))
	if err != nil {
		return
	}
	var t Task
	if json.Unmarshal(data, &t) != nil {
		return
	}
	t.BVID = bvid
	t.UpdatedAt = time.Now().Format(time.RFC3339)
	_ = s.save(&t)
}

// SetCompleted 标记任务完成
func (s *TaskStore) SetCompleted(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path(id))
	if err != nil {
		return
	}
	var t Task
	if json.Unmarshal(data, &t) != nil {
		return
	}
	t.Status = "completed"
	t.UpdatedAt = time.Now().Format(time.RFC3339)
	_ = s.save(&t)
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
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].CreatedAt > tasks[j].CreatedAt })
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
	return atomicWriteFile(c.path, data, 0600)
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
// 按优先级自动识别语言后缀（优先匹配更具体的后缀）
// 支持：.en.zh.srt (翻译), .zh-hant.srt, .zh.srt, .en.srt, .ja.srt, .srt (BCut ASR fallback)
func BuildSubtitleCandidates(videoID, dlDir string) []SubtitleTrack {
	if dlDir == "" {
		return nil
	}

	entries, err := os.ReadDir(dlDir)
	if err != nil {
		return nil
	}

	// 有序后缀列表（长后缀优先匹配，避免 .zh.srt 被 .srt 提前匹配）
	type suffixLang struct {
		suffix string
		lang   string
	}
	suffixes := []suffixLang{
		{".en.zh.srt", "zh"}, // 翻译结果（从 en → zh，最优先）
		{".zh-hant.srt", "zh-TW"},
		{".zh-hans.srt", "zh"},
		{".zh.srt", "zh"},
		{".en.srt", "en"},
		{".ja.srt", "ja"},
		{".ko.srt", "ko"},
		{".srt", "en"}, // BCut ASR 原始转录（无语言后缀，作为英语 fallback）
	}

	seen := make(map[string]bool)
	var tracks []SubtitleTrack

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".srt") {
			continue
		}
		name := strings.ToLower(e.Name())

		var language string
		for _, sl := range suffixes {
			if strings.HasSuffix(name, sl.suffix) {
				language = sl.lang
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

// ListPendingVideos 列出所有有待上传字幕的视频ID（审核通过后将恢复监听）
func (s *SubtitleStore) ListPendingVideos() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil
	}

	var result []string
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		videoID := strings.TrimSuffix(e.Name(), ".json")
		tracks, err := s.loadTracks(videoID)
		if err != nil || len(tracks) == 0 {
			continue
		}
		// 有 pending 或 failed 状态的 → 需要继续监听
		needsWatch := false
		for _, t := range tracks {
			if t.Status == SubtitleStatusPending || t.Status == SubtitleStatusFailed {
				needsWatch = true
				break
			}
		}
		if !needsWatch {
			continue
		}
		// 检查是否全部已上传完成
		allDone := true
		for _, t := range tracks {
			if t.Status != SubtitleStatusUploaded && t.Status != SubtitleStatusMissing {
				allDone = false
				break
			}
		}
		if allDone {
			continue
		}
		result = append(result, videoID)
	}
	return result
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
	return atomicWriteFile(s.path(videoID), data, 0644)
}
