package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/zolagz/ytb2bili-go/internal/config"
	"github.com/zolagz/ytb2bili-go/internal/download"
	"github.com/zolagz/ytb2bili-go/internal/llm"
	"github.com/zolagz/ytb2bili-go/internal/translator"
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
	return parseMetaResponse(resp, &VideoMeta{
		Title:       truncate(info.Title, 80),
		Description: truncate(info.Description, 500),
		Tags:        []string{},
	}), nil
}

// GenerateFromSRT 读取字幕文件，基于转写内容生成 Bilibili 元数据（中文标题/描述/标签）。
func GenerateFromSRT(ctx context.Context, srtPath string, cfg *config.Config) (*VideoMeta, error) {
	data, err := os.ReadFile(srtPath)
	if err != nil {
		return nil, fmt.Errorf("读取字幕失败: %w", err)
	}
	entries, err := translator.ParseSRT(string(data))
	if err != nil {
		return nil, fmt.Errorf("解析字幕失败: %w", err)
	}
	var sb strings.Builder
	for _, e := range entries {
		if strings.TrimSpace(e.Text) == "" {
			continue
		}
		sb.WriteString(strings.TrimSpace(e.Text))
		sb.WriteString("\n")
	}
	return GenerateFromSubtitle(ctx, "", sb.String(), cfg)
}

// GenerateFromSubtitle 基于字幕转写文本生成 Bilibili 元数据。
// originalTitle 为可选的原始视频标题（参考用），subtitleText 为去时间戳的转写内容。
func GenerateFromSubtitle(ctx context.Context, originalTitle, subtitleText string, cfg *config.Config) (*VideoMeta, error) {
	if originalTitle == "" {
		originalTitle = "(无原始标题)"
	}
	prompt := fmt.Sprintf(`Generate Bilibili video metadata (in Chinese) based on this video's transcript:

Original title (for reference): %s

Transcript:
%s

Please output in JSON format:
{
  "title": "Bilibili video title (max 80 chars, Chinese)",
  "description": "Bilibili video description (Chinese)",
  "tags": ["tag1", "tag2", "tag3", "tag4", "tag5"]
}`, originalTitle, truncate(subtitleText, 4000))

	resp, err := (&llm.OpenAIClient{APIKey: cfg.LLMAPIKey, BaseURL: cfg.LLMBaseURL, Model: cfg.LLMModel}).Complete(ctx, prompt)
	if err != nil {
		return nil, err
	}
	return parseMetaResponse(resp, &VideoMeta{
		Title: truncate(originalTitle, 80),
		Tags:  []string{},
	}), nil
}

// parseMetaResponse 从 LLM 响应中提取 JSON；解析失败时回退到 fallback。
func parseMetaResponse(resp string, fallback *VideoMeta) *VideoMeta {
	resp = strings.TrimSpace(resp)
	if idx := strings.Index(resp, "{"); idx >= 0 {
		resp = resp[idx:]
	}
	if idx := strings.LastIndex(resp, "}"); idx >= 0 {
		resp = resp[:idx+1]
	}
	var meta VideoMeta
	if err := json.Unmarshal([]byte(resp), &meta); err != nil {
		return fallback
	}
	if meta.Title == "" {
		meta.Title = fallback.Title
	}
	if meta.Tags == nil {
		meta.Tags = []string{}
	}
	// B站标题上限 80 字符，LLM 可能超长，硬性截断兜底。
	if before := meta.Title; len([]rune(before)) > maxBiliTitleLen {
		meta.Title = ClampTitle(before)
		log.Printf("⚠ 标题超过 %d 字符已截断: %s", maxBiliTitleLen, before)
	}
	return &meta
}

// maxBiliTitleLen B站标题最大长度（字符数）。
const maxBiliTitleLen = 80

// ClampTitle 将标题截断到 B站允许的最大长度（80 字符），按字符数计算（中文按 1 字）。
func ClampTitle(s string) string {
	return truncate(s, maxBiliTitleLen)
}

func truncate(s string, n int) string {
	if len([]rune(s)) > n {
		return string([]rune(s)[:n])
	}
	return s
}
