package cdp

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// 验证修复思路：会话绑定到无超时的 sessCtx，用看门狗 goroutine 防挂起
func TestSessionBindingFix(t *testing.T) {
	allocCtx, cancelA := chromedp.NewRemoteAllocator(context.Background(), "http://127.0.0.1:9223")
	defer cancelA()
	sessCtx, _ := chromedp.NewContext(allocCtx)

	// 看门狗：10s 未建立会话则杀掉 allocator
	done := make(chan error, 1)
	go func() {
		done <- chromedp.Run(sessCtx, chromedp.ActionFunc(func(ctx context.Context) error { return nil }))
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Skipf("Chrome not available: %v", err)
		}
	case <-time.After(10 * time.Second):
		cancelA()
		t.Fatal("session establishment timed out")
	}

	time.Sleep(300 * time.Millisecond)
	select {
	case <-sessCtx.Done():
		t.Fatalf("❌ sessCtx 已死: %v", sessCtx.Err())
	default:
		fmt.Println("✅ 会话存活（修复方案有效）")
	}
	// 再跑一次真实导航验证会话可用
	ctx, cancel := context.WithTimeout(sessCtx, 30*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate("about:blank")); err != nil {
		t.Fatalf("导航失败: %v", err)
	}
	fmt.Println("✅ 会话可正常导航")
}
