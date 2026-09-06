package server

// jobStore 为 HTTP/Feishu 提交的任务提供磁盘持久化与启动回放（Phase 4 M2）。
// 目标：服务重启不丢在途任务；任务状态由 jobs/<id>.json 单一持久源驱动执行。
//
// 生命周期：
//
//	提交时   save(status=pending)   → 入 taskChan
//	执行中   save(每一步事件)       → status 变为步骤名/running
//	结束     save(status=completed/failed + bvid/error)
//	重启回放  list() 中非 completed/failed 的重新入队（completed/failed 保留做记录）
//
// 回放安全性由既有机制保证：流水线步骤幂等（产物跳过）、投稿防重（history 守卫
// + pending.jsonl），因此 crash 后重跑不会重复投稿。
import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type jobStore struct {
	dir string
}

func newJobStore(dataDir string) *jobStore {
	dir := filepath.Join(dataDir, "jobs")
	os.MkdirAll(dir, 0755)
	return &jobStore{dir: dir}
}

func (js *jobStore) path(id string) string {
	return filepath.Join(js.dir, id+".json")
}

func (js *jobStore) save(t *VideoTask) error {
	if t == nil || t.ID == "" || strings.ContainsAny(t.ID, "/\\") {
		return fmt.Errorf("invalid task id")
	}
	t.UpdatedAt = time.Now().Format(time.RFC3339)
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal job: %w", err)
	}
	// 临时文件 + rename 原子写，崩溃不产生半截文件
	tmp, err := os.CreateTemp(js.dir, ".job-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp job: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp job: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, js.path(t.ID)); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename job: %w", err)
	}
	return nil
}

func (js *jobStore) load(id string) (*VideoTask, error) {
	data, err := os.ReadFile(js.path(id))
	if err != nil {
		return nil, err
	}
	var t VideoTask
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (js *jobStore) remove(id string) error {
	err := os.Remove(js.path(id))
	if err != nil && os.IsNotExist(err) {
		return nil
	}
	return err
}

// list 按创建时间升序返回全部任务记录。
func (js *jobStore) list() ([]*VideoTask, error) {
	entries, err := os.ReadDir(js.dir)
	if err != nil {
		return nil, err
	}
	var out []*VideoTask
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(js.dir, e.Name()))
		if err != nil {
			continue
		}
		var t VideoTask
		if json.Unmarshal(data, &t) != nil {
			continue
		}
		out = append(out, &t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt < out[j].CreatedAt })
	return out, nil
}

// replayPending 返回重启后需要继续执行的任务（status 非 completed/failed）。
func (js *jobStore) replayPending() ([]*VideoTask, error) {
	jobs, err := js.list()
	if err != nil {
		return nil, err
	}
	var pending []*VideoTask
	for _, t := range jobs {
		if t.Status != "completed" && t.Status != "failed" {
			pending = append(pending, t)
		}
	}
	return pending, nil
}
