// Package queue 提供集中式作业队列，所有视频状态通过 Unified State Machine 管理。
//
// 状态机：
//
//	discovered → queued → claimed → completed
//	                           ↘ failed → (重试 ≤3) → queued
//	                                              → completed (最终失败)
//
// 设计原则：
//   - 幂等：同一 video_id 多次入队 = 1 次
//   - 原子：状态转移使用 flock + 临时文件 + rename，保证 crash-safe
//   - 死锁检测：claimed 超过 30 分钟自动回退到 queued
//   - 重试上限：默认 3 次，超过标记为 failed
package queue

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ─── 状态常量 ──────────────────────────────────────────────────────────────────

const (
	StatusDiscovered = "discovered" // RSS 发现
	StatusQueued     = "queued"     // 等待处理
	StatusClaimed    = "claimed"    // 正在处理中
	StatusCompleted  = "completed"  // 处理成功
	StatusFailed     = "failed"     // 处理失败（可重试直到上限）
	StatusSkipped    = "skipped"    // 手动跳过

	DefaultMaxRetries = 3
	ClaimTimeout      = 30 * time.Minute // claimed 超时自动回退（daemon 每 30s 续租，活任务不会被误回收）
)

// ─── 数据结构 ──────────────────────────────────────────────────────────────────

// Video 表示队列中的一个视频
type Video struct {
	VideoID    string `json:"video_id"`
	URL        string `json:"url"`
	Title      string `json:"title,omitempty"`
	ChannelID  string `json:"channel_id,omitempty"`
	Source     string `json:"source"`               // "channel-sync", "ghibli", "manual"
	Status     string `json:"status"`                // 当前状态
	ClaimedBy  string `json:"claimed_by,omitempty"`  // 处理者标识
	ClaimedAt  string `json:"claimed_at,omitempty"`  // 处理开始时间
	RetryCount int    `json:"retry_count"`            // 当前重试次数
	MaxRetries int    `json:"max_retries"`            // 最大重试次数
	Error      string `json:"error,omitempty"`        // 最后错误信息
	BVID       string `json:"bvid,omitempty"`         // B站视频ID
	DiscoveredAt string `json:"discovered_at"`        // 发现时间
	UpdatedAt  string `json:"updated_at"`             // 最后更新时间
}

// QueueData 队列文件的顶层结构
type QueueData struct {
	Version int     `json:"version"`
	Videos  []Video `json:"videos"`
}

// Queue 队列管理器
//
// 锁与数据分离：flock 锁定独立的 queue.lock（inode 稳定），
// 数据文件 queue.json 仍用临时文件 + rename 原子替换，
// 避免"锁在即将被替换的旧 inode 上导致第二个进程对新 inode 加锁成功"的跨进程竞态。
type Queue struct {
	path         string // queue.json 的完整路径（数据，可被 rename 替换）
	lockPath     string // queue.lock（flock 目标，inode 永不更换）
	claimTimeout time.Duration // claimed 无续租多久视为死锁（默认 ClaimTimeout）
	mu           sync.Mutex
}

// ─── 构造 ──────────────────────────────────────────────────────────────────────

// New 创建或打开队列。dir 是 data 目录路径。
func New(dir string) *Queue {
	qDir := filepath.Join(dir, "queue")
	os.MkdirAll(qDir, 0755)
	return &Queue{
		path:         filepath.Join(qDir, "queue.json"),
		lockPath:     filepath.Join(qDir, "queue.lock"),
		claimTimeout: ClaimTimeout,
	}
}

// ─── 锁定 ──────────────────────────────────────────────────────────────────────

// lock 获取文件级独占锁（flock LOCK_EX），返回锁文件句柄。
// 锁目标为独立 queue.lock，不随 queue.json 的 rename 更换 inode。
func (q *Queue) lock() (*os.File, error) {
	f, err := os.OpenFile(q.lockPath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, fmt.Errorf("打开队列锁文件失败: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, fmt.Errorf("锁定队列失败: %w", err)
	}
	return f, nil
}

// unlock 释放文件锁
func unlock(f *os.File) {
	syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	f.Close()
}

// ─── 读写（原子操作） ───────────────────────────────────────────────────────────

// readAll 读取完整队列数据（调用方需持有锁）。
// 文件不存在或为空 → 返回空队列；非空但解析失败 → 返回错误（拒绝静默覆盖损坏数据）。
func (q *Queue) readAll() (*QueueData, error) {
	data, err := os.ReadFile(q.path)
	if err != nil {
		if os.IsNotExist(err) {
			return &QueueData{Version: 1, Videos: []Video{}}, nil
		}
		return nil, fmt.Errorf("读取队列文件失败: %w", err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return &QueueData{Version: 1, Videos: []Video{}}, nil
	}

	var qd QueueData
	if err := json.Unmarshal(data, &qd); err != nil {
		return nil, fmt.Errorf("队列文件损坏（%v）。已停止消费以防覆盖，恢复路径: %s.bak 或人工修复 %s", err, q.path, q.path)
	}
	if qd.Version != 0 && qd.Version != 1 {
		return nil, fmt.Errorf("未知队列版本 %d（%s），请使用兼容版本的工具修复", qd.Version, q.path)
	}
	if qd.Version == 0 {
		qd.Version = 1
	}
	if qd.Videos == nil {
		qd.Videos = []Video{}
	}
	return &qd, nil
}

// backup 将当前 queue.json 复制为 queue.json.bak（写入前的崩溃保险，best-effort）。
func (q *Queue) backup() {
	data, err := os.ReadFile(q.path)
	if err != nil {
		return // 首次写入尚无数据文件，无需备份
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return
	}
	os.WriteFile(q.path+".bak", data, 0644)
}

// writeAll 原子写入队列数据（临时文件 + rename），写入前保留 queue.json.bak
func (q *Queue) writeAll(data *QueueData) error {
	q.backup()
	dir := filepath.Dir(q.path)
	tmpPath := filepath.Join(dir, fmt.Sprintf(".queue.%d.tmp", rand.Int63()))
	tmp, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}
	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("编码队列失败: %w", err)
	}
	tmp.Close()
	if err := os.Rename(tmpPath, q.path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("写入队列失败: %w", err)
	}
	return nil
}

// ─── 核心操作 ──────────────────────────────────────────────────────────────────

// Add 添加视频到队列（幂等：同一 video_id 已存在或已提交则忽略），重试上限用默认值。
// url 格式：https://www.youtube.com/watch?v=VIDEO_ID
// source 标识来源（channel-sync, ghibli, manual）
func (q *Queue) Add(videoID, url, title, channelID, source string) (bool, error) {
	return q.AddWithRetries(videoID, url, title, channelID, source, DefaultMaxRetries)
}

// AddWithRetries 同 Add，但可指定该任务的最大重试次数（入队时写入任务快照）。
// maxRetries <= 0 时回退默认值。
func (q *Queue) AddWithRetries(videoID, url, title, channelID, source string, maxRetries int) (bool, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	f, err := q.lock()
	if err != nil {
		return false, err
	}
	defer unlock(f)

	data, err := q.readAll()
	if err != nil {
		return false, err
	}

	// 幂等检查：已在队列中
	for _, v := range data.Videos {
		if v.VideoID == videoID {
			return false, nil // 已存在，不重复添加
		}
	}

	// 三层防重复：检查 history.json
	if isInHistory(videoID, q.path) {
		return false, nil
	}

	if maxRetries <= 0 {
		maxRetries = DefaultMaxRetries
	}
	now := time.Now().Format(time.RFC3339)
	video := Video{
		VideoID:      videoID,
		URL:          url,
		Title:        title,
		ChannelID:    channelID,
		Source:       source,
		Status:       StatusQueued,
		MaxRetries:   maxRetries,
		DiscoveredAt: now,
		UpdatedAt:    now,
	}
	data.Videos = append(data.Videos, video)

	if err := q.writeAll(data); err != nil {
		return false, err
	}
	return true, nil
}

// Next 取出下一个可用的视频（queued → claimed）
// 返回 (video, nil) 或 (nil, ErrNoAvailable)
// 自动处理死锁：过期 claimed 回退到 queued
func (q *Queue) Next(workerID string) (*Video, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	f, err := q.lock()
	if err != nil {
		return nil, err
	}
	defer unlock(f)

	data, err := q.readAll()
	if err != nil {
		return nil, err
	}

	now := time.Now()
	changed := false

	// 死锁检测：超时的 claimed → 回退到 queued
	for i, v := range data.Videos {
		if v.Status != StatusClaimed {
			continue
		}
		claimedAt, err := time.Parse(time.RFC3339, v.ClaimedAt)
		if err != nil {
			continue
		}
		if now.Sub(claimedAt) > q.claimTimeout {
			data.Videos[i].Status = StatusQueued
			data.Videos[i].ClaimedBy = ""
			data.Videos[i].ClaimedAt = ""
			data.Videos[i].UpdatedAt = now.Format(time.RFC3339)
			changed = true
		}
	}

	// 找下一个 queued
	for i, v := range data.Videos {
		if v.Status != StatusQueued {
			continue
		}
		data.Videos[i].Status = StatusClaimed
		data.Videos[i].ClaimedBy = workerID
		data.Videos[i].ClaimedAt = time.Now().Format(time.RFC3339Nano) // 纳秒精度：同秒内续租也能延长租约
		data.Videos[i].UpdatedAt = now.Format(time.RFC3339)

		if err := q.writeAll(data); err != nil {
			return nil, err
		}
		return &data.Videos[i], nil
	}

	// 没有可用的，但如果有 deadlock 修复了，保存
	if changed {
		q.writeAll(data)
	}
	return nil, nil
}

// Complete 标记视频处理成功（claimed → completed）
func (q *Queue) Complete(videoID, bvid string) error {
	return q.transition(videoID, func(v *Video) (bool, string) {
		if v.Status != StatusClaimed {
			return false, fmt.Sprintf("状态不是 claimed (当前: %s)", v.Status)
		}
		v.Status = StatusCompleted
		v.BVID = bvid
		return true, ""
	})
}

// Fail 标记视频处理失败（claimed → failed 或 queued 重试）
// 自动重试逻辑：retry_count < max_retries → 回到 queued，否则 failed
func (q *Queue) Fail(videoID, errMsg string) error {
	return q.transition(videoID, func(v *Video) (bool, string) {
		if v.Status != StatusClaimed {
			return false, fmt.Sprintf("状态不是 claimed (当前: %s)", v.Status)
		}
		v.RetryCount++
		v.Error = errMsg
		if v.RetryCount >= v.MaxRetries {
			v.Status = StatusFailed
		} else {
			v.Status = StatusQueued // 重新排队等待重试
			v.ClaimedBy = ""
			v.ClaimedAt = ""
		}
		return true, ""
	})
}

// RenewClaim 续租认领：仅当任务仍处于 claimed 且属于该 worker 时刷新 ClaimedAt。
// 长任务（TTS/上传/网络重试可能超过 ClaimTimeout）由处理方周期性调用，
// 避免被其它 worker 的"死锁回收"误判后重复认领。返回错误表示已不属于该 worker。
func (q *Queue) RenewClaim(videoID, workerID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	f, err := q.lock()
	if err != nil {
		return err
	}
	defer unlock(f)

	data, err := q.readAll()
	if err != nil {
		return err
	}
	for i := range data.Videos {
		v := &data.Videos[i]
		if v.VideoID != videoID {
			continue
		}
		if v.Status != StatusClaimed {
			return fmt.Errorf("任务 %s 不在 claimed 状态（当前 %s）", videoID, v.Status)
		}
		if v.ClaimedBy != workerID {
			return fmt.Errorf("任务 %s 属于 worker %s，worker %s 无权续租", videoID, v.ClaimedBy, workerID)
		}
		now := time.Now().Format(time.RFC3339Nano) // 纳秒精度，保证续租真正刷新租约
		v.ClaimedAt = now
		v.UpdatedAt = now
		return q.writeAll(data)
	}
	return fmt.Errorf("视频 %s 不在队列中", videoID)
}

// Reset 手动将视频重置为 queued（用于人工介入修复后）
func (q *Queue) Reset(videoID string) error {
	return q.transition(videoID, func(v *Video) (bool, string) {
		if v.Status != StatusFailed && v.Status != StatusSkipped {
			return false, fmt.Sprintf("只能重置 failed/skipped (当前: %s)", v.Status)
		}
		v.Status = StatusQueued
		v.ClaimedBy = ""
		v.ClaimedAt = ""
		v.RetryCount = 0
		v.Error = ""
		return true, ""
	})
}

// DaemonWorkerPrefix 标识 daemon 消费者的认领：崩溃恢复只回收本类遗留任务。
const DaemonWorkerPrefix = "daemon:"

// DaemonWorkerID 返回 daemon 专用 worker 标识（区别于 queue work / CLI 等手动消费者）。
func DaemonWorkerID() string {
	return DaemonWorkerPrefix + WorkerID()
}

// RequeueClaimed 崩溃恢复：只回收 daemon 遗留的 claimed（claimed_by 以 "daemon:" 开头）。
// 单实例锁保证同一时刻至多一个 daemon，因此此类认领只可能来自已退出的实例，可安全续跑。
// 其它消费者（如 queue work）的活跃认领不受影响，避免"重启后抢走正在处理的任务"导致重复下载/投稿；
// 它们意外退出后的遗留认领由 Next() 的 claim 超时回收兜底。
func (q *Queue) RequeueClaimed() (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	f, err := q.lock()
	if err != nil {
		return 0, err
	}
	defer unlock(f)

	data, err := q.readAll()
	if err != nil {
		return 0, err
	}

	now := time.Now().Format(time.RFC3339)
	reset := 0
	for i := range data.Videos {
		if data.Videos[i].Status == StatusClaimed && strings.HasPrefix(data.Videos[i].ClaimedBy, DaemonWorkerPrefix) {
			data.Videos[i].Status = StatusQueued
			data.Videos[i].ClaimedBy = ""
			data.Videos[i].ClaimedAt = ""
			data.Videos[i].UpdatedAt = now
			reset++
		}
	}
	if reset > 0 {
		if err := q.writeAll(data); err != nil {
			return reset, err
		}
	}
	return reset, nil
}

// Skip 手动跳过视频
func (q *Queue) Skip(videoID string) error {
	return q.transition(videoID, func(v *Video) (bool, string) {
		if v.Status == StatusCompleted || v.Status == StatusSkipped {
			return false, fmt.Sprintf("无法跳过一个已完成/已跳过的视频")
		}
		v.Status = StatusSkipped
		return true, ""
	})
}

// Remove 从队列中删除一个视频（任意状态）。
func (q *Queue) Remove(videoID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	f, err := q.lock()
	if err != nil {
		return err
	}
	defer unlock(f)

	data, err := q.readAll()
	if err != nil {
		return err
	}

	for i, v := range data.Videos {
		if v.VideoID != videoID {
			continue
		}
		data.Videos = append(data.Videos[:i], data.Videos[i+1:]...)
		return q.writeAll(data)
	}
	return fmt.Errorf("视频 %s 不在队列中", videoID)
}

// Clear 清空整个队列（所有视频记录）。
func (q *Queue) Clear() error {
	q.mu.Lock()
	defer q.mu.Unlock()

	f, err := q.lock()
	if err != nil {
		return err
	}
	defer unlock(f)

	data, err := q.readAll()
	if err != nil {
		return err
	}
	data.Videos = nil
	return q.writeAll(data)
}

// transition 通用状态转移辅助函数
func (q *Queue) transition(videoID string, fn func(v *Video) (bool, string)) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	f, err := q.lock()
	if err != nil {
		return err
	}
	defer unlock(f)

	data, err := q.readAll()
	if err != nil {
		return err
	}

	for i, v := range data.Videos {
		if v.VideoID != videoID {
			continue
		}
		ok, msg := fn(&data.Videos[i])
		if !ok {
			return fmt.Errorf("转移失败: %s", msg)
		}
		data.Videos[i].UpdatedAt = time.Now().Format(time.RFC3339)
		return q.writeAll(data)
	}

	return fmt.Errorf("视频 %s 不在队列中", videoID)
}

// ─── 查询 ──────────────────────────────────────────────────────────────────────

// Status 返回队列中的视频列表，按更新时间降序排列
func (q *Queue) Status() (*QueueData, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	f, err := q.lock()
	if err != nil {
		return nil, err
	}
	defer unlock(f)

	data, err := q.readAll()
	if err != nil {
		return nil, err
	}

	// 按 updated_at 降序
	sort.Slice(data.Videos, func(i, j int) bool {
		return data.Videos[i].UpdatedAt > data.Videos[j].UpdatedAt
	})

	return data, nil
}

// GetByID 获取单个视频的状态
func (q *Queue) GetByID(videoID string) (*Video, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	f, err := q.lock()
	if err != nil {
		return nil, err
	}
	defer unlock(f)

	data, err := q.readAll()
	if err != nil {
		return nil, err
	}

	for _, v := range data.Videos {
		if v.VideoID == videoID {
			return &v, nil
		}
	}
	return nil, fmt.Errorf("视频 %s 不在队列中", videoID)
}

// Stats 返回队列统计信息
func (q *Queue) Stats() map[string]int {
	stats := map[string]int{
		"queued":    0,
		"claimed":   0,
		"completed": 0,
		"failed":    0,
		"skipped":   0,
		"total":     0,
	}

	data, err := q.Status()
	if err != nil {
		return stats
	}

	for _, v := range data.Videos {
		stats["total"]++
		switch v.Status {
		case StatusQueued:
			stats["queued"]++
		case StatusClaimed:
			stats["claimed"]++
		case StatusCompleted:
			stats["completed"]++
		case StatusFailed:
			stats["failed"]++
		case StatusSkipped:
			stats["skipped"]++
		}
	}
	return stats
}

// ─── 工具 ──────────────────────────────────────────────────────────────────────

// isInHistory 检查视频是否已经在提交历史中（三层防重复的最后一层）
func isInHistory(videoID, queuePath string) bool {
	// history.json 在 data/history/history.json
	historyPath := filepath.Join(filepath.Dir(filepath.Dir(queuePath)), "history", "history.json")
	data, err := os.ReadFile(historyPath)
	if err != nil {
		return false // 文件不存在或无法读取 = 没有历史
	}
	var entries []struct {
		YouTubeID string `json:"youtube_id"`
	}
	if err := json.Unmarshal(data, &entries); err != nil {
		return false
	}
	for _, e := range entries {
		if e.YouTubeID == videoID {
			return true
		}
	}
	return false
}

// IsValidStatus 检查是否为有效的状态值
func IsValidStatus(s string) bool {
	switch s {
	case StatusDiscovered, StatusQueued, StatusClaimed,
		StatusCompleted, StatusFailed, StatusSkipped:
		return true
	}
	return false
}

// ExtractVideoID 从 YouTube URL 中提取 video ID
func ExtractVideoID(url string) string {
	// https://www.youtube.com/watch?v=VIDEO_ID
	if idx := strings.Index(url, "v="); idx >= 0 {
		id := url[idx+2:]
		if amp := strings.Index(id, "&"); amp >= 0 {
			id = id[:amp]
		}
		if len(id) == 11 {
			return id
		}
	}
	// https://youtu.be/VIDEO_ID
	if strings.Contains(url, "youtu.be/") {
		parts := strings.Split(url, "/")
		for _, p := range parts {
			if len(p) == 11 && !strings.Contains(p, ".") {
				return p
			}
		}
	}
	return ""
}

// WorkerID 返回当前进程的唯一标识
func WorkerID() string {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}
	return fmt.Sprintf("%s:%d", hostname, os.Getpid())
}

// FormatDuration 格式化时间差
func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}
