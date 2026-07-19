package bili

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/difyz9/bilibili-go-sdk/bilibili"

	"github.com/zolagz/ytb2bili-go/internal/auth"
)

// archiveViewData holds the full archive view response with duration info
type archiveViewData struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    *struct {
		Archive *struct {
			Duration int64 `json:"duration"`
		} `json:"archive"`
	} `json:"data"`
}

// getVideoDurationFromAPI 从B站API获取视频时长（秒），返回0表示未获取到
func getVideoDurationFromAPI(cred *auth.LoginInfo, bvid string) int64 {
	client := &http.Client{Timeout: 10 * time.Second}
	url := fmt.Sprintf("https://member.bilibili.com/x/vupre/web/archive/view?bvid=%s", bvid)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0
	}
	req.Header.Set("Cookie", BuildCookiesString(cred))
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0
	}

	var data archiveViewData
	if err := json.Unmarshal(body, &data); err != nil {
		return 0
	}
	if data.Data != nil && data.Data.Archive != nil {
		return data.Data.Archive.Duration
	}
	return 0
}

// sanitizeBCCSubtitle 清理BCC字幕：截断超出视频时长的字幕条目
func sanitizeBCCSubtitle(subtitle *bilibili.BCCSubtitle, videoDuration int64) int {
	if videoDuration <= 0 || subtitle == nil {
		return 0
	}
	cleaned := make([]bilibili.BCCSubtitleItem, 0, len(subtitle.Body))
	for _, item := range subtitle.Body {
		// 跳过起始时间已超视频时长的条目
		if item.From >= float64(videoDuration) {
			continue
		}
		// 截断结束时间
		if item.To > float64(videoDuration) {
			item.To = float64(videoDuration) - 0.001
		}
		cleaned = append(cleaned, item)
	}
	removed := len(subtitle.Body) - len(cleaned)
	subtitle.Body = cleaned
	return removed
}
