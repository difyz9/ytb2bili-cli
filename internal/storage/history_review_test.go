package storage

import (
	"os"
	"path/filepath"
	"testing"
)

// ─── P0：投稿成功但 history 写入失败的补偿（pending → reconcile） ───────────

func TestPendingRecordAndReconcile(t *testing.T) {
	dir := t.TempDir()
	h := NewHistoryStore(dir)

	sv := &SubmittedVideo{YouTubeID: "vid1", BVID: "BV1", Title: "t", Channel: "c"}
	if err := h.Add(sv); err != nil {
		t.Fatal(err)
	}

	// 模拟另一条投稿成功但 history.Add 失败 → 进入 pending
	pend := &SubmittedVideo{YouTubeID: "vid2", BVID: "BV2", Title: "t2", Channel: "c2"}
	if err := h.RecordPending(pend); err != nil {
		t.Fatal(err)
	}
	if got := h.GetPendingBVID("vid2"); got != "BV2" {
		t.Fatalf("GetPendingBVID=%q，期望 BV2", got)
	}
	if got := h.GetPendingBVID("vid1"); got != "" {
		t.Fatalf("已在 history 的不应出现在 pending: %q", got)
	}

	// Reconcile 把 vid2 补录进 history
	n, err := h.ReconcilePending()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("ReconcilePending 应补录 1 条，实际 %d", n)
	}
	if !h.IsSubmitted("vid2") {
		t.Fatal("补录后 vid2 应在 history 中")
	}
	// pending 文件应已清空
	if _, err := os.Stat(filepath.Join(dir, "pending.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("补录完成后 pending 文件应被删除: %v", err)
	}

	// 幂等：再次 Reconcile 无事发生
	n, err = h.ReconcilePending()
	if err != nil || n != 0 {
		t.Fatalf("二次 Reconcile 应为 0/无错误: n=%d err=%v", n, err)
	}
}

func TestPendingKeepsMalformedLines(t *testing.T) {
	dir := t.TempDir()
	h := NewHistoryStore(dir)

	pend := &SubmittedVideo{YouTubeID: "v9", BVID: "BV9", Title: "t", Channel: "c"}
	if err := h.RecordPending(pend); err != nil {
		t.Fatal(err)
	}
	// 追加一行损坏数据
	f, err := os.OpenFile(filepath.Join(dir, "pending.jsonl"), os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("{broken json\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()

	n, err := h.ReconcilePending()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("应补录 1 条有效记录，实际 %d", n)
	}
	// 损坏行应保留（不静默丢弃数据）
	data, err := os.ReadFile(filepath.Join(dir, "pending.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{broken json\n" {
		t.Fatalf("损坏行应保留在原文件: %q", string(data))
	}
}
