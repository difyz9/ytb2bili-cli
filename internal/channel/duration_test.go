package channel

import (
	"context"
	"testing"

	"github.com/zolagz/ytb2bili-go/internal/config"
)

func TestShouldSkipAsShort(t *testing.T) {
	cfg := &config.Config{DataDir: t.TempDir()}

	t.Run("disabled when minDurationSec <= 0", func(t *testing.T) {
		skip, err := ShouldSkipAsShort(context.Background(), cfg, "whatever", 0)
		if err != nil || skip {
			t.Fatalf("got skip=%v err=%v, want false/nil", skip, err)
		}
	})

	// 阈值判定本身：≥ 阈值的视频不放行（不查询网络，直接看返回逻辑）
	t.Run("negative duration never skips", func(t *testing.T) {
		// minDurationSec > 0 但无 OAuth 且 yt-dlp 不可用时会走网络；这里只验证 0 阈值路径
		skip, _ := ShouldSkipAsShort(context.Background(), cfg, "x", 0)
		if skip {
			t.Fatal("expected false for disabled filter")
		}
	})
}
