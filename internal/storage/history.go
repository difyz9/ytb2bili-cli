package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SubmittedVideo 已提交的视频记录
type SubmittedVideo struct {
	YouTubeID   string `json:"youtube_id"`
	BVID        string `json:"bvid"`
	Title       string `json:"title"`
	Channel     string `json:"channel"`
	SubmittedAt string `json:"submitted_at"`
}

// HistoryStore 历史记录存储
type HistoryStore struct {
	dir string
	mu  sync.Mutex
}

// NewHistoryStore 创建历史记录存储
func NewHistoryStore(dir string) *HistoryStore {
	os.MkdirAll(dir, 0755)
	return &HistoryStore{dir: dir}
}

func (s *HistoryStore) path() string {
	return filepath.Join(s.dir, "history.json")
}

// IsSubmitted 检查视频是否已提交
func (s *HistoryStore) IsSubmitted(youtubeID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	videos, err := s.load()
	if err != nil {
		return false
	}

	for _, v := range videos {
		if v.YouTubeID == youtubeID {
			return true
		}
	}
	return false
}

// GetSubmitted 获取已提交视频的信息
func (s *HistoryStore) GetSubmitted(youtubeID string) *SubmittedVideo {
	s.mu.Lock()
	defer s.mu.Unlock()

	videos, err := s.load()
	if err != nil {
		return nil
	}

	for _, v := range videos {
		if v.YouTubeID == youtubeID {
			return &v
		}
	}
	return nil
}

// Add 添加已提交的视频
func (s *HistoryStore) Add(video *SubmittedVideo) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	videos, err := s.load()
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		videos = []SubmittedVideo{}
	}

	video.SubmittedAt = time.Now().Format(time.RFC3339)
	videos = append(videos, *video)

	return s.save(videos)
}

// List 列出所有已提交的视频
func (s *HistoryStore) List() ([]SubmittedVideo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.load()
}

// Count 返回已提交视频数量
func (s *HistoryStore) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	videos, err := s.load()
	if err != nil {
		return 0
	}
	return len(videos)
}

func (s *HistoryStore) load() ([]SubmittedVideo, error) {
	data, err := os.ReadFile(s.path())
	if err != nil {
		return nil, err
	}

	var videos []SubmittedVideo
	if err := json.Unmarshal(data, &videos); err != nil {
		return nil, err
	}

	return videos, nil
}

func (s *HistoryStore) save(videos []SubmittedVideo) error {
	data, err := json.MarshalIndent(videos, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(s.path(), data, 0644)
}

// ─── pending 补偿记录 ────────────────────────────────────────────────────────────
//
// 投稿成功（已拿到 bvid）后如果本地 history.json 写入失败，绝不能把失败传给流水线
// （否则重试会再次投稿，造成 B 站重复视频）。此时把结果追加到 pending.jsonl：
//   - 处理/启动前调用 ReconcilePending 补录回 history.json；
//   - GetPendingBVID 作为防重守卫，pending 存在时拒绝再次投稿。

func (s *HistoryStore) pendingPath() string {
	return filepath.Join(s.dir, "pending.jsonl")
}

// RecordPending 把一次"已投稿但未能写入 history"的结果追加到 pending 补偿日志。
func (s *HistoryStore) RecordPending(v *SubmittedVideo) error {
	if v == nil {
		return fmt.Errorf("nil submission")
	}
	if v.SubmittedAt == "" {
		v.SubmittedAt = time.Now().Format(time.RFC3339)
	}
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal pending: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := os.OpenFile(s.pendingPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("打开 pending 文件失败: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("写入 pending 失败: %w", err)
	}
	return f.Sync()
}

// GetPendingBVID 返回该视频是否有待补录的投稿记录（有则返回 bvid）。
func (s *HistoryStore) GetPendingBVID(youtubeID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	lines, err := readLinesFile(s.pendingPath())
	if err != nil {
		return ""
	}
	for _, line := range lines {
		var v SubmittedVideo
		if json.Unmarshal([]byte(line), &v) != nil || v.YouTubeID != youtubeID {
			continue
		}
		return v.BVID
	}
	return ""
}

// ReconcilePending 把 pending.jsonl 中的记录补录进 history.json（已存在则跳过）。
// 返回本次补录条数。无法补录（history 写入仍失败）或格式损坏的行保留在文件中。
func (s *HistoryStore) ReconcilePending() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	lines, err := readLinesFile(s.pendingPath())
	if err != nil {
		return 0, nil // 无 pending 文件，正常
	}
	if len(lines) == 0 {
		return 0, nil
	}

	videos, err := s.load()
	if err != nil {
		if !os.IsNotExist(err) {
			return 0, fmt.Errorf("读取 history 失败，暂缓补录: %w", err)
		}
		videos = []SubmittedVideo{}
	}
	have := make(map[string]bool, len(videos))
	for _, v := range videos {
		have[v.YouTubeID] = true
	}

	reconciled := 0
	remaining := make([]string, 0, len(lines))
	for _, line := range lines {
		var v SubmittedVideo
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			remaining = append(remaining, line) // 损坏行保留，不静默丢弃
			continue
		}
		if have[v.YouTubeID] {
			reconciled++ // 已在 history，仅清理 pending 行
			continue
		}
		if v.SubmittedAt == "" {
			v.SubmittedAt = time.Now().Format(time.RFC3339)
		}
		videos = append(videos, v)
		have[v.YouTubeID] = true
		reconciled++
	}

	if err := s.save(videos); err != nil {
		return 0, fmt.Errorf("补录写入 history 失败: %w", err)
	}
	if err := writeLinesFile(s.pendingPath(), remaining); err != nil {
		return reconciled, fmt.Errorf("清理 pending 失败: %w", err)
	}
	return reconciled, nil
}

func readLinesFile(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	raw := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(raw) == 1 && raw[0] == "" {
		return nil, nil
	}
	lines := raw[:0]
	for _, l := range raw {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	return lines, nil
}

func writeLinesFile(path string, lines []string) error {
	if len(lines) == 0 {
		return os.Remove(path) // 全部补录完成，删除空 pending
	}
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	return os.WriteFile(path, []byte(b.String()), 0644)
}
