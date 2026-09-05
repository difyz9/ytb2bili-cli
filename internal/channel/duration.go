package channel

import (
	"context"

	"github.com/zolagz/ytb2bili-go/internal/config"
	"github.com/zolagz/ytb2bili-go/internal/download"
	"github.com/zolagz/ytb2bili-go/internal/ytoauth"
)

// VideoDurationSec 返回视频时长（秒）。
// 优先用 OAuth 调 YouTube Data API（videos.list contentDetails，快且省流量），
// 无有效 token 时回退到 yt-dlp（download.InfoContext）获取。
func VideoDurationSec(ctx context.Context, cfg *config.Config, videoID string) (int, error) {
	if d, ok := dataAPIDuration(ctx, cfg, videoID); ok {
		return d, nil
	}
	info, err := download.InfoContext(ctx, "https://www.youtube.com/watch?v="+videoID, cfg.EffectiveCookiesPath())
	if err != nil {
		return 0, err
	}
	return info.Duration, nil
}

// ShouldSkipAsShort 判断视频是否因过短（Short）而应跳过入队。
// minDurationSec <= 0 表示不限制，直接返回 false。
func ShouldSkipAsShort(ctx context.Context, cfg *config.Config, videoID string, minDurationSec int) (skip bool, err error) {
	if minDurationSec <= 0 {
		return false, nil
	}
	dur, err := VideoDurationSec(ctx, cfg, videoID)
	if err != nil {
		return false, err
	}
	return dur > 0 && dur < minDurationSec, nil
}

// dataAPIDuration 尝试通过 YouTube Data API 查询单个视频时长；无有效 token 或失败时返回 ok=false。
func dataAPIDuration(ctx context.Context, cfg *config.Config, videoID string) (int, bool) {
	if cfg == nil || cfg.YouTubeOAuth == nil || cfg.YouTubeOAuth.ClientID == "" {
		return 0, false
	}
	store := ytoauth.NewTokenStore(cfg.DataDir)
	if !store.Exists() {
		return 0, false
	}
	oc := ytoauth.Config{
		ClientID:     cfg.YouTubeOAuth.ClientID,
		ClientSecret: cfg.YouTubeOAuth.ClientSecret,
		Scopes:       ytoauth.DefaultScopes(),
	}
	tok, err := ytoauth.ValidToken(ctx, oc, store)
	if err != nil || tok == nil {
		return 0, false
	}
	durs, err := ytoauth.FetchVideoDurations(ctx, tok.AccessToken, []string{videoID})
	if err != nil {
		return 0, false
	}
	if d, ok := durs[videoID]; ok && d > 0 {
		return d, true
	}
	return 0, false
}
