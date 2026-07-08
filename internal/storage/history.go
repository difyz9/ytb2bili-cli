package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
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
	data, _ := json.MarshalIndent(videos, "", "  ")
	return os.WriteFile(s.path(), data, 0644)
}
