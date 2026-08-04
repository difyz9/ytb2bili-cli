// 订阅频道获取 — YouTube Data API v3 subscriptions.list
package ytoauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// ─── 数据结构 ────────────────────────────────────────────────────────────

// Subscription 一个订阅的频道
type Subscription struct {
	ChannelID           string `json:"channel_id"`
	ChannelTitle        string `json:"channel_title"`
	ChannelDescription  string `json:"channel_description,omitempty"`
	ChannelThumbnailURL string `json:"channel_thumbnail_url,omitempty"`
	ChannelCustomURL    string `json:"channel_custom_url,omitempty"`
	SubscribedAt        string `json:"subscribed_at,omitempty"`
}

// subscriptionsResponse YouTube API 响应
type subscriptionsResponse struct {
	Kind          string               `json:"kind"`
	NextPageToken string               `json:"nextPageToken"`
	Items         []subscriptionItem   `json:"items"`
	Error         *apiError            `json:"error,omitempty"`
}

type subscriptionItem struct {
	Snippet struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		ResourceID  struct {
			ChannelID string `json:"channelId"`
		} `json:"resourceId"`
		Thumbnails map[string]struct {
			URL string `json:"url"`
		} `json:"thumbnails"`
		PublishedAt string `json:"publishedAt"`
	} `json:"snippet"`
}

type apiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ─── 拉取订阅 ────────────────────────────────────────────────────────────

// FetchSubscriptions 分页拉取当前用户的订阅频道列表。
// accessToken 可以是 OAuth access_token（mine=true 必需）。
func FetchSubscriptions(ctx context.Context, accessToken string) ([]Subscription, error) {
	if accessToken == "" {
		return nil, errors.New("yt-oauth: access_token 为空，请先登录")
	}

	var all []Subscription
	pageToken := ""

	for {
		q := url.Values{}
		q.Set("part", "snippet")
		q.Set("mine", "true")
		q.Set("maxResults", "50")
		if pageToken != "" {
			q.Set("pageToken", pageToken)
		}

		reqURL := "https://www.googleapis.com/youtube/v3/subscriptions?" + q.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("Accept", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("请求订阅列表失败: %w", err)
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			var apiErr struct {
				Error *apiError `json:"error"`
			}
			json.Unmarshal(body, &apiErr)
			msg := "未知错误"
			if apiErr.Error != nil {
				msg = apiErr.Error.Message
			}
			return nil, fmt.Errorf("YouTube API 错误 (%d): %s", resp.StatusCode, msg)
		}

		var result subscriptionsResponse
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("解析订阅响应失败: %w", err)
		}

		for _, item := range result.Items {
			sub := Subscription{
				ChannelID:          item.Snippet.ResourceID.ChannelID,
				ChannelTitle:       item.Snippet.Title,
				ChannelDescription: item.Snippet.Description,
				SubscribedAt:       item.Snippet.PublishedAt,
			}
			if thumb, ok := item.Snippet.Thumbnails["default"]; ok {
				sub.ChannelThumbnailURL = thumb.URL
			}
			all = append(all, sub)
		}

		pageToken = result.NextPageToken
		if pageToken == "" {
			break
		}
	}

	return all, nil
}
