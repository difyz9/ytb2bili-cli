package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zolagz/ytb2bili-go/internal/config"
	"github.com/zolagz/ytb2bili-go/internal/download"
	"github.com/zolagz/ytb2bili-go/internal/llm"
)

type VideoMeta struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

func Generate(info download.VideoInfo, cfg *config.Config) (*VideoMeta, error) {
	return GenerateContext(context.Background(), info, cfg)
}

func GenerateContext(ctx context.Context, info download.VideoInfo, cfg *config.Config) (*VideoMeta, error) {
	prompt := fmt.Sprintf(`Generate Bilibili video metadata (in Chinese) based on this YouTube video:

Title: %s
Description: %s

Please output in JSON format:
{
  "title": "Bilibili video title (max 80 chars, Chinese)",
  "description": "Bilibili video description (Chinese)",
  "tags": ["tag1", "tag2", "tag3", "tag4", "tag5"]
}`, info.Title, truncate(info.Description, 500))

	resp, err := (&llm.OpenAIClient{APIKey: cfg.LLMAPIKey, BaseURL: cfg.LLMBaseURL, Model: cfg.LLMModel}).Complete(ctx, prompt)
	if err != nil {
		return nil, err
	}

	// Try to parse JSON from response
	resp = strings.TrimSpace(resp)
	if idx := strings.Index(resp, "{"); idx >= 0 {
		resp = resp[idx:]
	}
	if idx := strings.LastIndex(resp, "}"); idx >= 0 {
		resp = resp[:idx+1]
	}

	var meta VideoMeta
	if err := json.Unmarshal([]byte(resp), &meta); err != nil {
		// Fallback: use original title
		return &VideoMeta{
			Title:       truncate(info.Title, 80),
			Description: truncate(info.Description, 500),
			Tags:        []string{},
		}, nil
	}

	if meta.Title == "" {
		meta.Title = truncate(info.Title, 80)
	}
	if meta.Tags == nil {
		meta.Tags = []string{}
	}

	return &meta, nil
}

func truncate(s string, n int) string {
	if len([]rune(s)) > n {
		return string([]rune(s)[:n])
	}
	return s
}
