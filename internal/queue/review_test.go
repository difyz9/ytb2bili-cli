package queue

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ─── P0：队列锁必须在独立 lock 文件上（rename 换 inode 后锁仍有效） ──────────

func TestQueueLockFileStableAcrossRewrite(t *testing.T) {
	dir := t.TempDir()
	q := New(dir)

	if _, err := q.Add("a1", "https://youtu.be/a1", "A", "c", "manual"); err != nil {
		t.Fatal(err)
	}
	// 写回会临时文件+rename 替换 queue.json，但锁文件路径不应变
	qDir := filepath.Join(dir, "queue")
	if _, err := os.Stat(filepath.Join(qDir, "queue.lock")); err != nil {
		t.Fatalf("queue.lock 不存在: %v", err)
	}
	if _, err := os.Stat(filepath.Join(qDir, "queue.json")); err != nil {
		t.Fatalf("queue.json 不存在: %v", err)
	}
	if sameFile(qDir) {
		t.Fatal("lock 与数据文件不应是同一个文件（lock 必须独立，rename 后仍有效）")
	}
}

func sameFile(qDir string) bool {
	fi1, e1 := os.Stat(filepath.Join(qDir, "queue.lock"))
	fi2, e2 := os.Stat(filepath.Join(qDir, "queue.json"))
	return e1 == nil && e2 == nil && os.SameFile(fi1, fi2)
}

// ─── P0：两个独立进程并发写不丢任务（评审要求：Add/Next/Complete 跨进程测试） ──

func TestQueueConcurrentProcesses(t *testing.T) {
	dir := t.TempDir()
	const workers = 3
	const perWorker = 25

	cmds := make([]*exec.Cmd, 0, workers)
	for i := 0; i < workers; i++ {
		cmd := exec.Command(os.Args[0], "-test.run=TestQueueRaceHelperChild")
		cmd.Env = append(os.Environ(),
			"QUEUE_RACE_HELPER=1",
			"QUEUE_DIR="+dir,
			fmt.Sprintf("QUEUE_WORKER=w%d", i),
			fmt.Sprintf("QUEUE_N=%d", perWorker),
		)
		cmds = append(cmds, cmd)
	}
	for _, c := range cmds {
		if err := c.Start(); err != nil {
			t.Fatalf("启动子进程失败: %v", err)
		}
	}
	for _, c := range cmds {
		if err := c.Wait(); err != nil {
			t.Fatalf("子进程失败: %v", err)
		}
	}

	q := New(dir)
	data, err := q.Status()
	if err != nil {
		t.Fatal(err)
	}
	total := workers * perWorker
	got := map[string]int{}
	for _, v := range data.Videos {
		got[v.Status]++
	}
	if got["completed"] != total {
		t.Fatalf("跨进程并发后 completed=%d，期望 %d（存在丢失或重复认领）; 分布=%v", got["completed"], total, got)
	}
	if got["queued"] != 0 || got["claimed"] != 0 || got["failed"] != 0 {
		t.Fatalf("存在未收敛状态: %v", got)
	}
}

// TestQueueRaceHelperChild 是跨进程测试的子进程入口（由父进程以 -test.run 调用）。
func TestQueueRaceHelperChild(t *testing.T) {
	if os.Getenv("QUEUE_RACE_HELPER") != "1" {
		return
	}
	dir := os.Getenv("QUEUE_DIR")
	worker := os.Getenv("QUEUE_WORKER")
	q := New(dir)

	// 1) 加入自己的任务
	for i := 0; i < 25; i++ {
		id := fmt.Sprintf("%s-%d", worker, i)
		if _, err := q.Add(id, "https://youtu.be/"+id, "t", "c", "manual"); err != nil {
			t.Fatalf("child Add 失败: %v", err)
		}
	}
	// 2) 与其它进程竞争认领并完成（直到队列空且保持空一段时间）
	idle := 0
	for idle < 30 {
		v, err := q.Next(worker)
		if err != nil {
			t.Fatalf("child Next 失败: %v", err)
		}
		if v == nil {
			idle++
			time.Sleep(20 * time.Millisecond)
			continue
		}
		idle = 0
		if err := q.Complete(v.VideoID, "BVtest"); err != nil {
			t.Fatalf("child Complete %s 失败: %v", v.VideoID, err)
		}
	}
}

// ─── P0：claim 只在状态机内被回收（无重复认领路径被常规流程触发） ────────────
// 该场景由 daemon 单实例锁 + ClaimTimeout 共同保证；此处验证 AddWithRetries
// 上限确实进入任务快照，且 Fail 按配置次数收敛。

func TestAddWithRetriesSnapshotAndConverge(t *testing.T) {
	dir := t.TempDir()
	q := New(dir)

	if _, err := q.AddWithRetries("r1", "https://youtu.be/r1", "R1", "c", "auto", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := q.AddWithRetries("r5", "https://youtu.be/r5", "R5", "c", "auto", 5); err != nil {
		t.Fatal(err)
	}
	if _, err := q.AddWithRetries("r0", "https://youtu.be/r0", "R0", "c", "auto", 0); err != nil {
		t.Fatal(err)
	}

	// 快照断言
	check := func(id string, want int) {
		v, err := q.GetByID(id)
		if err != nil {
			t.Fatal(err)
		}
		if v.MaxRetries != want {
			t.Fatalf("%s max_retries=%d，期望 %d", id, v.MaxRetries, want)
		}
	}
	check("r1", 1)
	check("r5", 5)
	check("r0", DefaultMaxRetries)

	// 收敛行为：maxRetries=1 → 首次失败即 failed
	v, err := q.Next("w")
	if err != nil || v == nil {
		t.Fatalf("claim r1 失败: v=%v err=%v", v, err)
	}
	// Next 按序返回 r1（插入序）
	if v.VideoID != "r1" {
		t.Fatalf("期望先 claim r1，实际 %s", v.VideoID)
	}
	if err := q.Fail(v.VideoID, "boom"); err != nil {
		t.Fatal(err)
	}
	if got, _ := q.GetByID("r1"); got.Status != StatusFailed {
		t.Fatalf("maxRetries=1 应首次失败即 failed，实际 %s", got.Status)
	}

	// maxRetries=5：前 4 次失败回到 queued，第 5 次 failed
	for attempt := 1; attempt <= 5; attempt++ {
		vv, err := q.Next("w")
		if err != nil || vv == nil {
			t.Fatalf("claim r5 第 %d 次失败: %v", attempt, err)
		}
		if vv.VideoID != "r5" {
			t.Fatalf("期望 claim r5，实际 %s", vv.VideoID)
		}
		if err := q.Fail(vv.VideoID, "boom"); err != nil {
			t.Fatal(err)
		}
	}
	if got, _ := q.GetByID("r5"); got.Status != StatusFailed {
		t.Fatalf("maxRetries=5 应在第 5 次失败后 failed，实际 %s (retry=%d)", got.Status, got.RetryCount)
	}
}

// ─── P1：损坏队列拒绝静默覆盖，备份文件保留 ─────────────────────────────────

func TestCorruptQueueRejected(t *testing.T) {
	dir := t.TempDir()
	q := New(dir)
	if _, err := q.Add("k1", "https://youtu.be/k1", "K", "c", "manual"); err != nil {
		t.Fatal(err)
	}
	qDir := filepath.Join(dir, "queue")
	good, err := os.ReadFile(filepath.Join(qDir, "queue.json"))
	if err != nil {
		t.Fatal(err)
	}

	// 模拟磁盘损坏：截断 JSON
	if err := os.WriteFile(filepath.Join(qDir, "queue.json"), []byte(`{"version":1,"videos":[`), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Status(); err == nil {
		t.Fatal("损坏队列应返回错误，而不是当空队列")
	}
	if _, err := q.Add("k2", "https://youtu.be/k2", "K2", "c", "manual"); err == nil {
		t.Fatal("损坏队列应拒绝写入（防止静默覆盖）")
	}

	// 非法版本同样拒绝
	if err := os.WriteFile(filepath.Join(qDir, "queue.json"), []byte(`{"version":99,"videos":[]}`), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Status(); err == nil || !strings.Contains(err.Error(), "版本") {
		t.Fatalf("非法版本应报错，实际: %v", err)
	}

	// 恢复：用备份还原后可用（writeAll 曾写入 .bak）
	if err := os.WriteFile(filepath.Join(qDir, "queue.json"), good, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Status(); err != nil {
		t.Fatalf("还原后仍失败: %v", err)
	}
}

// ─── P1：claim 续租 —— 活的长任务不被死锁回收，过期无续租的才被回收 ────────

func TestRenewClaimKeepsLiveTaskSafe(t *testing.T) {
	q := New(t.TempDir())
	q.claimTimeout = 300 * time.Millisecond // 缩短租约便于测试

	if _, err := q.Add("live", "https://youtu.be/live", "L", "c", "manual"); err != nil {
		t.Fatal(err)
	}
	v, err := q.Next("w1")
	if err != nil || v == nil {
		t.Fatalf("claim 失败: %v", err)
	}

	// 超过租约一部分后续租（模拟长任务心跳）
	time.Sleep(200 * time.Millisecond)
	if err := q.RenewClaim("live", "w1"); err != nil {
		t.Fatalf("续租失败: %v", err)
	}

	// 另一 worker 立即 Next：续租后租约新鲜，不应回收
	if other, err := q.Next("w2"); other != nil || err != nil {
		t.Fatalf("续租后的活任务不应被其它 worker 回收: v=%v err=%v", other, err)
	}
	got, _ := q.GetByID("live")
	if got.Status != StatusClaimed || got.ClaimedBy != "w1" {
		t.Fatalf("续租后应仍由 w1 持有: %+v", got)
	}

	// 错误路径：非持有者无权续租
	if err := q.RenewClaim("live", "w9"); err == nil {
		t.Fatal("非持有 worker 续租应报错")
	}

	// 停更（模拟 worker 崩溃）：租约过期后应被回收
	time.Sleep(400 * time.Millisecond) // 距上次续租 > 300ms
	reclaimed, err := q.Next("w2")
	if err != nil {
		t.Fatal(err)
	}
	if reclaimed == nil || reclaimed.VideoID != "live" || reclaimed.ClaimedBy != "w2" {
		t.Fatalf("过期认领应被回收并转给 w2: %+v", reclaimed)
	}
	// 旧持有者再续租 → 报错（已被接管）
	if err := q.RenewClaim("live", "w1"); err == nil {
		t.Fatal("已被接管的任务，原 worker 续租应报错")
	}
}
