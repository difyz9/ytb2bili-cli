package transcriber

import (
	"context"
	"testing"
	"time"
)

// ─── P1：Bcut 轮询休眠可被 context 取消 ─────────────────────────────────────

func TestSleepCtxCancelPrompt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 已取消
	start := time.Now()
	err := sleepCtx(ctx, 5*time.Second)
	if time.Since(start) > time.Second {
		t.Fatalf("ctx 已取消时 sleepCtx 应立即返回，实际耗时 %v", time.Since(start))
	}
	if err == nil {
		t.Fatal("ctx 取消应返回错误")
	}
}

func TestSleepCtxWaits(t *testing.T) {
	start := time.Now()
	if err := sleepCtx(context.Background(), 50*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d < 30*time.Millisecond {
		t.Fatalf("sleepCtx 未等待足够时长: %v", d)
	}
}
