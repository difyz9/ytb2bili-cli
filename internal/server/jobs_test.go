package server

import (
	"path/filepath"
	"testing"
	"time"
)

// ─── Phase4 M2：任务持久化与重启回放 ─────────────────────────────────────

func mkJob(id, url, status string, createdAt time.Time) *VideoTask {
	return &VideoTask{
		ID: id, URL: url, Title: "t", Status: status,
		CreatedAt: createdAt.Format(time.RFC3339),
		UpdatedAt: createdAt.Format(time.RFC3339),
	}
}

func TestJobStoreSaveLoad(t *testing.T) {
	js := newJobStore(t.TempDir())
	task := mkJob("job1", "https://youtu.be/abc", "pending", time.Now())
	if err := js.save(task); err != nil {
		t.Fatal(err)
	}
	got, err := js.load("job1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "job1" || got.Status != "pending" || got.URL != task.URL {
		t.Fatalf("roundtrip 失败: %+v", got)
	}

	// 更新后覆盖（模拟执行中状态推进）
	task.Status = "download"
	if err := js.save(task); err != nil {
		t.Fatal(err)
	}
	got, _ = js.load("job1")
	if got.Status != "download" {
		t.Fatalf("覆盖失败: %+v", got)
	}
}

func TestJobStoreReplayPending(t *testing.T) {
	js := newJobStore(t.TempDir())
	base := time.Now()
	jobs := []*VideoTask{
		mkJob("p1", "https://youtu.be/1", "pending", base),
		mkJob("r1", "https://youtu.be/2", "download", base.Add(time.Second)), // 执行中被中断
		mkJob("c1", "https://youtu.be/3", "completed", base.Add(2*time.Second)),
		mkJob("f1", "https://youtu.be/4", "failed", base.Add(3*time.Second)),
	}
	for _, j := range jobs {
		if err := js.save(j); err != nil {
			t.Fatal(err)
		}
	}

	pending, err := js.replayPending()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 2 {
		t.Fatalf("应回放 2 个未完成任务(pending/download)，实际 %d", len(pending))
	}
	if pending[0].ID != "p1" || pending[1].ID != "r1" {
		t.Fatalf("回放顺序/内容错误: %+v", pending)
	}

	// remove
	if err := js.remove("p1"); err != nil {
		t.Fatal(err)
	}
	if _, err := js.load("p1"); err == nil {
		t.Fatal("remove 后不应能加载")
	}
}

func TestJobStoreIgnoreCorruptEntry(t *testing.T) {
	dir := t.TempDir()
	js := newJobStore(dir)
	if err := js.save(mkJob("ok", "https://youtu.be/1", "pending", time.Now())); err != nil {
		t.Fatal(err)
	}
	// 手工制造损坏文件（同名不同内容模拟半截写入无法发生——rename 原子；这里模拟外部破坏）
	bad := mkJob("bad", "https://youtu.be/2", "pending", time.Now())
	if err := js.save(bad); err != nil {
		t.Fatal(err)
	}
	// list/replay 应跳过损坏行而不崩溃
	if err := writeBrokenJSON(filepath.Join(dir, "jobs", bad.ID+".json")); err != nil {
		t.Fatal(err)
	}
	pending, err := js.replayPending()
	if err != nil {
		t.Fatalf("损坏记录不应导致整体失败: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != "ok" {
		t.Fatalf("应只回放未损坏任务: %+v", pending)
	}
}
