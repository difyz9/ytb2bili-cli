// Package queue 审计事件与失败分类（Phase 4 M0：可观测性地基）。
//
// 所有队列状态转移都会以追加方式写入 <data>/audit/events.jsonl：
//   - 零侵入：best-effort，失败不影响业务（不返回错误、不 fsync）
//   - 事件模型即未来 SQLite jobs/job_steps 表的字段蓝本
//   - 失败错误统一分类（网络/认证/外部API/资源/本地/未知），支撑失败统计与告警
package queue

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// AuditEvent 一条队列审计事件。
type AuditEvent struct {
	Ts         string `json:"ts"`
	Type       string `json:"type"` // queued | claimed | completed | failed | retry | requeued | reset
	VideoID    string `json:"video_id,omitempty"`
	Source     string `json:"source,omitempty"`
	Worker     string `json:"worker,omitempty"`
	BVID       string `json:"bvid,omitempty"`
	Error      string `json:"error,omitempty"`
	ErrorClass string `json:"error_class,omitempty"`
	RetryCount int    `json:"retry_count,omitempty"`
	MaxRetries int    `json:"max_retries,omitempty"`
	DurationMS int64  `json:"duration_ms,omitempty"`
}

// 失败分类
const (
	ClassAuth     = "auth"      // 认证/登录/cookie/权限
	ClassNetwork  = "network"   // 网络/连接/超时
	ClassExternal = "external"  // 外部 API 明确报错（B站/YouTube/翻译服务商等）
	ClassResource = "resource"  // 资源缺失/格式/依赖（文件、ffmpeg、模型等）
	ClassLocal    = "local"     // 本地存储/IO
	ClassUnknown  = "unknown"
)

// ClassifyError 把错误消息归类到失败分类（关键词启发式，供统计/告警聚合）。
// 顺序即优先级：认证/网络/本地等强信号在前，泛化的"XX失败"操作名兜底在后。
func ClassifyError(msg string) string {
	if msg == "" {
		return ClassUnknown
	}
	m := strings.ToLower(msg)
	switch {
	case containsAny(m, "sign in to confirm", "bot", "login", "cookie", "登录", "凭据", "认证", "403", "401", "secretid", "token", "auth", "账号"):
		return ClassAuth
	case containsAny(m, "timeout", "timed out", "i/o timeout", "eof", "reset by peer", "connection", "connect", "网络", "连接", "dial", "504", "502", "503", "read: connection"):
		return ClassNetwork
	case containsAny(m, "写入", "读取", "权限", "permission", "no space", "磁盘", "rename", "open "):
		return ClassLocal
	case containsAny(m, "不存在", "not found", "missing", "未安装", "无 ffmpeg", "ffmpeg", "模型", "whisper", "exit status", "损坏", "格式"):
		return ClassResource
	case containsAny(m, "下载失败", "上传失败", "投稿失败", "转写失败", "翻译失败", "tts 合成", "b站", "youtube", "bilibili", "deepseek", "腾讯", "code=", "status "):
		return ClassExternal
	}
	return ClassUnknown
}

func containsAny(m string, subs ...string) bool {
	for _, s := range subs {
		if strings.Contains(m, s) {
			return true
		}
	}
	return false
}

// AuditStore 追加写审计事件（O_APPEND 单行原子追加，best-effort）。
type AuditStore struct {
	path string
}

// OpenAudit 打开审计存储（目录不存在则创建）。
func OpenAudit(dataDir string) *AuditStore {
	dir := filepath.Join(dataDir, "audit")
	os.MkdirAll(dir, 0755)
	return &AuditStore{path: filepath.Join(dir, "events.jsonl")}
}

// emit 追加一条事件。任何失败仅静默忽略（审计不应拖垮业务路径）。
func (a *AuditStore) emit(ev AuditEvent) {
	if a == nil {
		return
	}
	if ev.Ts == "" {
		ev.Ts = time.Now().Format(time.RFC3339Nano)
	}
	line, err := json.Marshal(ev)
	if err != nil {
		return
	}
	f, err := os.OpenFile(a.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	f.Write(append(line, '\n'))
	f.Close()
}

// Read 读取全部审计事件（按时间顺序）。文件不存在返回空切片。
func (a *AuditStore) Read() ([]AuditEvent, error) {
	data, err := os.ReadFile(a.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var events []AuditEvent
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var ev AuditEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue // 跳过损坏行，不阻塞统计
		}
		events = append(events, ev)
	}
	return events, nil
}

// FailureSummary 聚合失败事件分类统计与最近错误。
type FailureSummary struct {
	Total        int                       `json:"total"`
	Completed    int                       `json:"completed"`
	Queued       int                       `json:"queued"`
	ByClass      map[string]int            `json:"by_class"`
	RecentErrors []AuditEvent              `json:"recent_errors,omitempty"`
}

// SummarizeFailures 统计失败分类与最近错误（最多 recent N 条）。
func (a *AuditStore) SummarizeFailures(recent int) (*FailureSummary, error) {
	events, err := a.Read()
	if err != nil {
		return nil, err
	}
	s := &FailureSummary{ByClass: map[string]int{}}
	for _, ev := range events {
		switch ev.Type {
		case "completed":
			s.Completed++
		case "queued":
			s.Queued++
		case "failed":
			s.Total++
			cls := ev.ErrorClass
			if cls == "" {
				cls = ClassUnknown
			}
			s.ByClass[cls]++
		}
	}
	if recent > 0 {
		for i := len(events) - 1; i >= 0 && len(s.RecentErrors) < recent; i-- {
			if events[i].Type == "failed" {
				s.RecentErrors = append(s.RecentErrors, events[i])
			}
		}
	}
	return s, nil
}

// String 简要输出分类（CLI 展示用）。
func (s *FailureSummary) String() string {
	if s == nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "审计: queued=%d completed=%d failed=%d\n", s.Queued, s.Completed, s.Total)
	if len(s.ByClass) == 0 {
		return b.String()
	}
	b.WriteString("失败分类:\n")
	for _, cls := range []string{ClassAuth, ClassNetwork, ClassExternal, ClassResource, ClassLocal, ClassUnknown} {
		if n := s.ByClass[cls]; n > 0 {
			b.WriteString(fmt.Sprintf("  %-9s %d\n", cls, n))
		}
	}
	return b.String()
}
