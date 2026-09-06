package storage

import (
	"strconv"
	"sync"
	"testing"
)

// ─── P1：TaskStore 持锁原子更新，并发不丢字段 ──────────────────────────────

func TestTaskStoreConcurrentUpdateNoLost(t *testing.T) {
	dir := t.TempDir()
	ts := NewTaskStore(dir)
	if err := ts.Persist(ts.Prepare("job1", "https://youtu.be/x"), nil); err != nil {
		t.Fatal(err)
	}

	const workers = 8
	const perWorker = 40
	want := workers * perWorker

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				// 同一个字段读-改-写：Update 原子化前会互相覆盖导致计数丢失
				err := ts.Update("job1", func(t *Task) error {
					n, _ := strconv.Atoi(t.Result["counter"])
					t.Result["counter"] = strconv.Itoa(n + 1)
					return nil
				})
				if err != nil {
					t.Errorf("Update 失败: %v", err)
					return
				}
			}
		}(w)
	}
	wg.Wait()

	got, err := ts.Get("job1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Result["counter"] != strconv.Itoa(want) {
		t.Fatalf("并发 Update 后 counter=%s，期望 %d（存在丢失更新）", got.Result["counter"], want)
	}
}

// ─── P1：UpdateStep/SetBVID/SetCompleted 错误可返回（不再静默吞掉） ──────────

func TestTaskStoreUpdateErrorsReturned(t *testing.T) {
	dir := t.TempDir()
	ts := NewTaskStore(dir)

	// 不存在的任务 → 报错
	if err := ts.UpdateStep("nope", "download", "completed"); err == nil {
		t.Fatal("UpdateStep 对不存在任务应返回错误")
	}
	if err := ts.SetBVID("nope", "BV1"); err == nil {
		t.Fatal("SetBVID 对不存在任务应返回错误")
	}
	if err := ts.SetCompleted("nope"); err == nil {
		t.Fatal("SetCompleted 对不存在任务应返回错误")
	}
	// 非法 id 同样报错
	if err := ts.SetBVID("", "BV1"); err == nil {
		t.Fatal("SetBVID 空 id 应返回错误")
	}

	// fn 返回错误 → 中止且不写盘
	id := "job2"
	if err := ts.Persist(ts.Prepare(id, "https://youtu.be/y"), nil); err != nil {
		t.Fatal(err)
	}
	if err := ts.Update(id, func(t *Task) error {
		t.BVID = "BVshould-not-save"
		return strconv.ErrSyntax
	}); err == nil {
		t.Fatal("fn 返回错误时 Update 应返回错误")
	}
	if got, _ := ts.Get(id); got.BVID != "" {
		t.Fatalf("fn 出错后不应写盘，实际 bvid=%q", got.BVID)
	}

	// 正常链路生效
	if err := ts.UpdateStep(id, "download", "completed"); err != nil {
		t.Fatal(err)
	}
	if err := ts.SetBVID(id, "BV123"); err != nil {
		t.Fatal(err)
	}
	if err := ts.SetCompleted(id); err != nil {
		t.Fatal(err)
	}
	got, _ := ts.Get(id)
	if got.BVID != "BV123" || got.Status != "completed" ||
		got.Steps["download"].Status != "completed" {
		t.Fatalf("更新未生效: %+v", got)
	}
}
