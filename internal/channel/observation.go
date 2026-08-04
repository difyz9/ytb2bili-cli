package channel

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Observation 一次同步观测到的单视频播放量快照（point-in-time）。
// 记录 data/observations/<日期>.json，供 velocity（播放增量/时间）计算。
type Observation struct {
	VideoID    string    `json:"video_id"`
	ChannelID  string    `json:"channel_id"`
	Title      string    `json:"title,omitempty"`
	Views      int       `json:"views"`
	ObservedAt time.Time `json:"observed_at"`
}

// observationsPath 返回某日观测文件路径（data/observations/<yyyy-mm-dd>.json）。
func (m *Monitor) observationsPath(t time.Time) string {
	return filepath.Join(m.obsDir, t.Format("2006-01-02")+".json")
}

// RecordObservation 把本次同步的观测快照追加到当日文件。
// 同日多次同步会合并保留（同视频不同时刻的观测对是 velocity 计算的输入）。
func (m *Monitor) RecordObservation(obs []Observation) error {
	if len(obs) == 0 {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	path := m.observationsPath(time.Now())
	var existing []Observation
	if data, err := os.ReadFile(path); err == nil {
		json.Unmarshal(data, &existing) // 文件损坏则忽略，从空开始
	}
	existing = append(existing, obs...)
	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
