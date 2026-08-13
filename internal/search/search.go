package search

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Video 搜索结果中的视频信息
type Video struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Channel     string   `json:"channel"`
	ChannelID   string   `json:"channel_id,omitempty"`
	Description string   `json:"description"`
	Duration    string   `json:"duration"`
	DurationSec int      `json:"duration_sec,omitempty"`
	Views       string   `json:"views"`
	ViewCount   int64    `json:"view_count,omitempty"`
	PublishTime string   `json:"publish_time"`
	Thumbnails  []string `json:"thumbnails"`
	URL         string   `json:"url"`
	IsLive      bool     `json:"is_live,omitempty"`
	IsVerified  bool     `json:"is_verified,omitempty"`
}

// SearchResult 搜索结果
type SearchResult struct {
	Query       string  `json:"query"`
	Videos      []Video `json:"videos"`
	TotalFound  int     `json:"total_found"`
	Continuation string `json:"continuation,omitempty"`
}

// Searcher YouTube 搜索器
type Searcher struct {
	MaxResults int
	Retries    int
	Timeout    time.Duration
	Client     *http.Client
}

// New 创建搜索器
func New(maxResults int) *Searcher {
	if maxResults <= 0 {
		maxResults = 10
	}
	return &Searcher{
		MaxResults: maxResults,
		Retries:    3,
		Timeout:    15 * time.Second,
		Client:     &http.Client{Timeout: 15 * time.Second},
	}
}

// Search 搜索 YouTube 视频 (使用 InnerTube API)
func (s *Searcher) Search(query string) ([]Video, error) {
	return s.SearchWithFilter(query, nil)
}

// SearchWithFilter 使用过滤器搜索 YouTube 视频
func (s *Searcher) SearchWithFilter(query string, filter *SearchFilter) ([]Video, error) {
	var params string
	if filter != nil {
		params = filter.BuildParams()
	}

	payload := buildSearchPayload(query, params, "")
	payloadBytes, _ := json.Marshal(payload)

	var lastErr error
	for attempt := 0; attempt <= s.Retries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}

		req, err := http.NewRequest("POST", searchURL, bytes.NewReader(payloadBytes))
		if err != nil {
			lastErr = err
			continue
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", webUserAgent)
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")
		req.Header.Set("Origin", "https://www.youtube.com")
		req.Header.Set("Referer", "https://www.youtube.com/")

		resp, err := s.Client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode != 200 {
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body[:min(len(body), 200)]))
			continue
		}

		var data map[string]interface{}
		if err := json.Unmarshal(body, &data); err != nil {
			lastErr = fmt.Errorf("解析 JSON 失败: %w", err)
			continue
		}

		videos, err := parseInnerTubeResponse(data, s.MaxResults)
		if err != nil {
			lastErr = err
			continue
		}

		return videos, nil
	}

	return nil, fmt.Errorf("搜索失败 (重试 %d 次): %w", s.Retries, lastErr)
}

// SearchPaginated 分页搜索，返回结果和 continuation token
func (s *Searcher) SearchPaginated(query string, filter *SearchFilter, continuation string) (*SearchResult, error) {
	var params string
	if filter != nil {
		params = filter.BuildParams()
	}

	payload := buildSearchPayload(query, params, continuation)
	payloadBytes, _ := json.Marshal(payload)

	var lastErr error
	for attempt := 0; attempt <= s.Retries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}

		req, err := http.NewRequest("POST", searchURL, bytes.NewReader(payloadBytes))
		if err != nil {
			lastErr = err
			continue
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", webUserAgent)
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")
		req.Header.Set("Origin", "https://www.youtube.com")
		req.Header.Set("Referer", "https://www.youtube.com/")

		resp, err := s.Client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode != 200 {
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
			continue
		}

		var data map[string]interface{}
		if err := json.Unmarshal(body, &data); err != nil {
			lastErr = fmt.Errorf("解析 JSON 失败: %w", err)
			continue
		}

		videos, nextContinuation := parseInnerTubeResponseWithContinuation(data, s.MaxResults)

		return &SearchResult{
			Query:        query,
			Videos:       videos,
			TotalFound:   len(videos),
			Continuation: nextContinuation,
		}, nil
	}

	return nil, fmt.Errorf("分页搜索失败 (重试 %d 次): %w", s.Retries, lastErr)
}

// parseInnerTubeResponse 解析 InnerTube API 响应
func parseInnerTubeResponse(data map[string]interface{}, maxResults int) ([]Video, error) {
	videos, _ := parseInnerTubeResponseWithContinuation(data, maxResults)
	return videos, nil
}

// parseInnerTubeResponseWithContinuation 解析 InnerTube API 响应，同时返回 continuation token
func parseInnerTubeResponseWithContinuation(data map[string]interface{}, maxResults int) ([]Video, string) {
	var videos []Video
	var continuation string

	// InnerTube 响应结构:
	// contents -> twoColumnSearchResultsRenderer -> primaryContents -> sectionListRenderer -> contents
	contents, ok := data["contents"].(map[string]interface{})
	if !ok {
		return videos, continuation
	}

	twoColumn, ok := contents["twoColumnSearchResultsRenderer"].(map[string]interface{})
	if !ok {
		return videos, continuation
	}

	primaryContents, ok := twoColumn["primaryContents"].(map[string]interface{})
	if !ok {
		return videos, continuation
	}

	sectionList, ok := primaryContents["sectionListRenderer"].(map[string]interface{})
	if !ok {
		return videos, continuation
	}

	sectionContents, ok := sectionList["contents"].([]interface{})
	if !ok {
		return videos, continuation
	}

	for _, section := range sectionContents {
		sectionMap, ok := section.(map[string]interface{})
		if !ok {
			continue
		}

		// 处理 itemSectionRenderer
		if itemSection, ok := sectionMap["itemSectionRenderer"].(map[string]interface{}); ok {
			items, ok := itemSection["contents"].([]interface{})
			if !ok {
				continue
			}

			for _, item := range items {
				itemMap, ok := item.(map[string]interface{})
				if !ok {
					continue
				}

				// 处理 videoRenderer
				if videoData, ok := itemMap["videoRenderer"].(map[string]interface{}); ok {
					video := parseVideoRenderer(videoData)
					if video.ID != "" {
						videos = append(videos, video)
						if len(videos) >= maxResults {
							return videos, continuation
						}
					}
				}

				// 处理 continuationItemRenderer (分页)
				if contItem, ok := itemMap["continuationItemRenderer"].(map[string]interface{}); ok {
					if contEndpoint, ok := contItem["continuationEndpoint"].(map[string]interface{}); ok {
						if contCommand, ok := contEndpoint["continuationCommand"].(map[string]interface{}); ok {
							if token, ok := contCommand["token"].(string); ok {
								continuation = token
							}
						}
					}
				}
			}
		}

		// 处理 continuationItemRenderer (在 section 级别)
		if contItem, ok := sectionMap["continuationItemRenderer"].(map[string]interface{}); ok {
			if contEndpoint, ok := contItem["continuationEndpoint"].(map[string]interface{}); ok {
				if contCommand, ok := contEndpoint["continuationCommand"].(map[string]interface{}); ok {
					if token, ok := contCommand["token"].(string); ok {
						continuation = token
					}
				}
			}
		}
	}

	return videos, continuation
}

// parseVideoRenderer 解析单个视频数据
func parseVideoRenderer(data map[string]interface{}) Video {
	video := Video{}

	// ID
	if id, ok := data["videoId"].(string); ok {
		video.ID = id
		video.URL = fmt.Sprintf("https://www.youtube.com/watch?v=%s", id)
	}

	// Title
	if title, ok := data["title"].(map[string]interface{}); ok {
		if runs, ok := title["runs"].([]interface{}); ok && len(runs) > 0 {
			if run, ok := runs[0].(map[string]interface{}); ok {
				video.Title, _ = run["text"].(string)
			}
		}
		if simpleText, ok := title["simpleText"].(string); ok {
			video.Title = simpleText
		}
	}

	// Channel
	if channel, ok := data["longBylineText"].(map[string]interface{}); ok {
		if runs, ok := channel["runs"].([]interface{}); ok && len(runs) > 0 {
			if run, ok := runs[0].(map[string]interface{}); ok {
				video.Channel, _ = run["text"].(string)
			}
		}
	}
	// Fallback: shortBylineText
	if video.Channel == "" {
		if channel, ok := data["shortBylineText"].(map[string]interface{}); ok {
			if runs, ok := channel["runs"].([]interface{}); ok && len(runs) > 0 {
				if run, ok := runs[0].(map[string]interface{}); ok {
					video.Channel, _ = run["text"].(string)
				}
			}
		}
	}

	// Channel ID
	if ownerText, ok := data["ownerText"].(map[string]interface{}); ok {
		if runs, ok := ownerText["runs"].([]interface{}); ok && len(runs) > 0 {
			if run, ok := runs[0].(map[string]interface{}); ok {
				if navigationEndpoint, ok := run["navigationEndpoint"].(map[string]interface{}); ok {
					if browseEndpoint, ok := navigationEndpoint["browseEndpoint"].(map[string]interface{}); ok {
						video.ChannelID, _ = browseEndpoint["browseId"].(string)
					}
				}
			}
		}
	}

	// Description
	if desc, ok := data["descriptionSnippet"].(map[string]interface{}); ok {
		if runs, ok := desc["runs"].([]interface{}); ok && len(runs) > 0 {
			var descParts []string
			for _, run := range runs {
				if runMap, ok := run.(map[string]interface{}); ok {
					if text, ok := runMap["text"].(string); ok {
						descParts = append(descParts, text)
					}
				}
			}
			video.Description = strings.Join(descParts, "")
		}
	}

	// Duration
	if duration, ok := data["lengthText"].(map[string]interface{}); ok {
		video.Duration, _ = duration["simpleText"].(string)
		video.DurationSec = parseDurationToSeconds(video.Duration)
	}

	// Views
	if views, ok := data["viewCountText"].(map[string]interface{}); ok {
		if simpleText, ok := views["simpleText"].(string); ok {
			video.Views = simpleText
			video.ViewCount = parseViewCount(simpleText)
		}
		if runs, ok := views["runs"].([]interface{}); ok && len(runs) > 0 {
			if run, ok := runs[0].(map[string]interface{}); ok {
				if text, ok := run["text"].(string); ok {
					video.Views = text
					video.ViewCount = parseViewCount(text)
				}
			}
		}
	}

	// Publish Time
	if publishTime, ok := data["publishedTimeText"].(map[string]interface{}); ok {
		video.PublishTime, _ = publishTime["simpleText"].(string)
	}

	// Thumbnails
	if thumbnail, ok := data["thumbnail"].(map[string]interface{}); ok {
		if thumbs, ok := thumbnail["thumbnails"].([]interface{}); ok {
			for _, thumb := range thumbs {
				if thumbMap, ok := thumb.(map[string]interface{}); ok {
					if thumbURL, ok := thumbMap["url"].(string); ok {
						video.Thumbnails = append(video.Thumbnails, thumbURL)
					}
				}
			}
		}
	}

	// Live status
	if badges, ok := data["badges"].([]interface{}); ok {
		for _, badge := range badges {
			if badgeMap, ok := badge.(map[string]interface{}); ok {
				if metadataBadge, ok := badgeMap["metadataBadgeRenderer"].(map[string]interface{}); ok {
					if label, ok := metadataBadge["label"].(string); ok {
						if strings.Contains(strings.ToLower(label), "live") {
							video.IsLive = true
						}
					}
				}
			}
		}
	}

	// Verified badge
	if ownerBadges, ok := data["ownerBadges"].([]interface{}); ok {
		for _, badge := range ownerBadges {
			if badgeMap, ok := badge.(map[string]interface{}); ok {
				if metadataBadge, ok := badgeMap["metadataBadgeRenderer"].(map[string]interface{}); ok {
					if style, ok := metadataBadge["style"].(string); ok {
						if style == "BADGE_STYLE_TYPE_VERIFIED" {
							video.IsVerified = true
						}
					}
				}
			}
		}
	}

	return video
}

// parseDurationToSeconds 将时长字符串转换为秒数
// 支持格式: "1:23:45", "23:45", "1:23"
func parseDurationToSeconds(duration string) int {
	if duration == "" {
		return 0
	}

	parts := strings.Split(duration, ":")
	if len(parts) == 0 {
		return 0
	}

	seconds := 0
	multiplier := 1

	for i := len(parts) - 1; i >= 0; i-- {
		var val int
		fmt.Sscanf(parts[i], "%d", &val)
		seconds += val * multiplier
		multiplier *= 60
	}

	return seconds
}

// parseViewCount 解析观看次数字符串
func parseViewCount(s string) int64 {
	// 移除 "views" 后缀
	s = strings.ReplaceAll(s, " views", "")
	s = strings.ReplaceAll(s, "view", "")
	s = strings.TrimSpace(s)

	// 处理 "1,234,567" 格式
	s = strings.ReplaceAll(s, ",", "")

	var count int64
	fmt.Sscanf(s, "%d", &count)
	return count
}

// SearchAndPrint 搜索并打印结果
func SearchAndPrint(query string, maxResults int) error {
	searcher := New(maxResults)
	videos, err := searcher.Search(query)
	if err != nil {
		return err
	}

	if len(videos) == 0 {
		fmt.Println("未找到结果")
		return nil
	}

	fmt.Printf("🔍 搜索: %s\n", query)
	fmt.Printf("📺 找到 %d 个视频:\n\n", len(videos))

	for i, v := range videos {
		fmt.Printf("%d. %s\n", i+1, v.Title)
		fmt.Printf("   频道: %s\n", v.Channel)
		fmt.Printf("   时长: %s | 观看: %s | %s\n", v.Duration, v.Views, v.PublishTime)
		fmt.Printf("   链接: %s\n\n", v.URL)
	}

	return nil
}

// ExtractVideoID 从 URL 或文本中提取视频 ID
// 使用字符串匹配方式（与 pipeline.ExtractYouTubeID 共享逻辑）
func ExtractVideoID(input string) string {
	// 直接的视频 ID (11 位)
	if len(input) == 11 {
		for _, c := range input {
			if !isVideoIDChar(c) {
				return ""
			}
		}
		return input
	}

	// YouTube URL patterns
	for _, prefix := range []string{"v=", "youtu.be/", "shorts/", "embed/"} {
		if idx := strings.Index(input, prefix); idx >= 0 {
			start := idx + len(prefix)
			end := strings.IndexAny(input[start:], "?&#/")
			if end < 0 {
				end = len(input) - start
			}
			id := input[start : start+end]
			if len(id) == 11 {
				return id
			}
		}
	}
	return ""
}

// isVideoIDChar 检查字符是否属于 YouTube 视频 ID 字符集
func isVideoIDChar(c rune) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_'
}

// min 返回两个整数中较小的一个
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// SearchWithOptions 搜索 YouTube 视频 (完整选项)
func (s *Searcher) SearchWithOptions(query string, opts ...SearchOption) (*SearchResult, error) {
	config := &searchConfig{
		maxResults: s.MaxResults,
	}

	for _, opt := range opts {
		opt(config)
	}

	filter := &SearchFilter{
		SortBy:     config.sortBy,
		UploadDate: config.uploadDate,
		Type:       config.videoType,
		Duration:   config.duration,
		Features:   config.features,
	}

	return s.SearchPaginated(query, filter, config.continuation)
}

// SearchOption 搜索选项
type SearchOption func(*searchConfig)

type searchConfig struct {
	maxResults  int
	sortBy      string
	uploadDate  string
	videoType   string
	duration    string
	features    []string
	continuation string
}

// WithSortBy 设置排序方式
func WithSortBy(sortBy string) SearchOption {
	return func(c *searchConfig) { c.sortBy = sortBy }
}

// WithUploadDate 设置上传日期过滤
func WithUploadDate(date string) SearchOption {
	return func(c *searchConfig) { c.uploadDate = date }
}

// WithType 设置内容类型
func WithType(typ string) SearchOption {
	return func(c *searchConfig) { c.videoType = typ }
}

// WithDuration 设置时长过滤
func WithDuration(dur string) SearchOption {
	return func(c *searchConfig) { c.duration = dur }
}

// WithFeatures 设置功能过滤
func WithFeatures(features ...string) SearchOption {
	return func(c *searchConfig) { c.features = features }
}

// WithContinuation 设置分页 token
func WithContinuation(token string) SearchOption {
	return func(c *searchConfig) { c.continuation = token }
}

// WithMaxResults 设置最大结果数
func WithMaxResults(max int) SearchOption {
	return func(c *searchConfig) { c.maxResults = max }
}

// ─── Content Safety Filter ─────────────────────────────────────────────────────

// NegativeSuffix 搜索负向屏蔽词（自动追加到每条 query 末尾）
// 排除政治、色情、暴力、新闻、无关杂项
const NegativeSuffix = "-politics -political -election -government -war -military -news -documentary -protest -conflict -adult -nsfw -violence -crime -religion -debate -history -documentary -celebrity -sports -gaming -reaction -prank -meme"

// BlocklistWords 标题/简介黑名单关键词（命中任一即丢弃）
var BlocklistWords = []string{
	// 政治/敏感
	"election", "president", "policy debate", "government",
	"protest", "war", "military", "political", "politics",
	"documentary", "country conflict",

	// 色情/暴力
	"nsfw", "adult", "gore", "murder", "violent", "18+",

	// 无关杂项
	"celebrity", "movie review", "sports", "podcast debate",
	"daily vlog", "reaction video", "prank", "challenge",
	"mukbang", "gaming", "gameplay", "music video",
	"trailer", "review movie", "tiktok", "shorts compilation",
	"funny moments", "best moments", "highlights",
	"vine", "meme compilation",
}

// FilterByContent 通过标题/简介过滤视频
// 返回 true = 丢弃（命中黑名单），false = 保留
func FilterByContent(v Video) bool {
	title := strings.ToLower(v.Title)
	desc := strings.ToLower(v.Description)
	combined := title + " " + desc

	for _, banned := range BlocklistWords {
		if strings.Contains(combined, strings.ToLower(banned)) {
			return true
		}
	}
	return false
}

// ApplySafeSearch 对搜索结果应用安全过滤
// 返回过滤后的视频列表（去重 + 黑名单 + 时长合理性）
func ApplySafeSearch(videos []Video, minViews int64, maxDurationSec int) []Video {
	seen := make(map[string]bool)
	var result []Video

	for _, v := range videos {
		// 去重
		if seen[v.ID] {
			continue
		}
		seen[v.ID] = true

		// 排除直播
		if v.IsLive {
			continue
		}

		// 内容黑名单
		if FilterByContent(v) {
			continue
		}

		// 观看数过滤
		if v.ViewCount < minViews {
			continue
		}

		// 时长过滤
		if maxDurationSec > 0 && v.DurationSec > maxDurationSec {
			continue
		}
		if v.DurationSec > 0 && v.DurationSec < 120 {
			continue // 排除 <2分钟短视频
		}

		result = append(result, v)
	}
	return result
}

// ─── Standardized Keyword Library ─────────────────────────────────────────────

// KeywordCategory 关键词分类
type KeywordCategory struct {
	Category string   // 分类名: ai-agent, web-dev, llm-tech, media-agent
	Keywords []string // 关键词列表
}

// StandardKeywords 标准化搜索关键词库（五大类）
var StandardKeywords = []KeywordCategory{
	{
		Category: "ai-agent",
		Keywords: []string{
			"Hermes Agent OS Obsidian memory workflow tutorial",
			"Multi-agent orchestration kanban task system",
			"AI Agent DAG workflow editor React Flow",
			"AutoGen CrewAI LangGraph agent collaboration",
			"Model Context Protocol MCP agent skill development",
			"Local open source AI agent deployment",
			"Agent memory layer long term retrieval Obsidian",
			"AI video generation agent pipeline workflow",
			"build autonomous AI agent from scratch",
			"multi-agent task decomposition system design",
			"agent kanban task scheduling open source",
			"AI agent persistent memory implementation",
		},
	},
	{
		Category: "pi-agent",
		Keywords: []string{
			"pi.dev coding agent tutorial",
			"Pi agent CLI setup review",
			"Pi coding agent harness earendil",
			"Pi agent vs Claude Code comparison",
			"Pi agent tool calling workflow automation",
			"Pi AI agent review hands on",
			"Pi agent MCP integration tutorial",
			"Pi agent build software from scratch",
			"Pi agent CLI power user tips",
			"Pi agent autonomous coding workflow",
		},
	},
	{
		Category: "web-dev",
		Keywords: []string{
			"AI workflow automation tutorial",
			"AI tools for productivity tutorial",
			"prompt engineering tutorial beginners",
			"RAG knowledge base build tutorial",
			"AI coding assistant tutorial",
			"Local AI models app tutorial",
			"Docker AI agent deployment guide",
			"Full stack AI media generation pipeline",
		},
	},
	{
		Category: "llm-tech",
		Keywords: []string{
			"Grok agent coding benchmark comparison",
			"GPT agent system architecture deep dive",
			"Local LLM agent Ollama integration",
			"RAG multi-agent knowledge base implementation",
			"Open source agent engine performance optimization",
			"fine tuning LLM for tool calling",
			"open source LLM deployment tutorial",
			"AI model inference optimization",
		},
	},
	{
		Category: "media-agent",
		Keywords: []string{
			"AI video agent workflow intermediate asset management",
			"Multi-step media generation agent pipeline",
			"AI script storyboard voice synthesis agent",
			"Automated video production agent orchestration",
			"ComfyUI agent workflow automation",
			"AI image generation pipeline API tutorial",
		},
	},
}

// ExpandKeyword 展开关键词 ID 为完整搜索字符串
// 支持短 ID 格式: "ai-3", "web-1", "llm-2" 等
// 也支持直接传入原始关键词
func ExpandKeyword(keyword string) string {
	// 检查短 ID 格式 (category-N)
	parts := strings.SplitN(keyword, "-", 2)
	if len(parts) == 2 {
		categoryShort := parts[0]
		var idx int
		if _, err := fmt.Sscanf(parts[1], "%d", &idx); err == nil && idx >= 1 {
			// 分类简写映射
			catMap := map[string]string{
				"ai":    "ai-agent",
				"pi":    "pi-agent",
				"web":   "web-dev",
				"llm":   "llm-tech",
				"media": "media-agent",
			}
			if fullCat, ok := catMap[categoryShort]; ok {
				for _, cat := range StandardKeywords {
					if cat.Category == fullCat {
						if idx-1 < len(cat.Keywords) {
							return cat.Keywords[idx-1]
						}
					}
				}
			}
		}
	}
	// 直接返回原始关键词
	return keyword
}

// BuildSearchQuery 构建安全的搜索查询（追加负向屏蔽词）
func BuildSearchQuery(keyword string) string {
	return ExpandKeyword(keyword) + " " + NegativeSuffix
}

// ─── Video Scoring ────────────────────────────────────────────────────────────

// ScorerType 评分策略
type ScorerType string

const (
	ScorerPopular  ScorerType = "popular"  // 播放量优先（默认）
	ScorerFresh    ScorerType = "fresh"    // 时效优先
	ScorerBalanced ScorerType = "balanced" // 均衡评分
	ScorerNowcast  ScorerType = "nowcast"  // ytsubs nowcast：播放 vs 频道基线，捕捉超常/起势视频
)

// ScoredVideo 带评分的视频
type ScoredVideo struct {
	Video
	Score         float64 `json:"score"`
	ViewScore     float64 `json:"view_score"`
	RecencyScore  float64 `json:"recency_score"`
	DurationScore float64 `json:"duration_score"`
	NowcastScore  float64 `json:"nowcast_score,omitempty"` // 播放 vs 频道基线
	VelocityScore float64 `json:"velocity_score,omitempty"` // 播放速率 vs 期望斜率（起势）
	ReachScore    float64 `json:"reach_score,omitempty"`    // 播放/粉丝数（对数饱和）
	Confidence    float64 `json:"confidence_multiplier,omitempty"` // 置信度乘数 0.75-1.05
	BreakoutBoost float64 `json:"breakout_boost,omitempty"`       // Early Breakout 加成
	ExpectedViews float64 `json:"expected_views_now,omitempty"`   // 按年龄曲线预期的播放量
	Predicted48h  float64 `json:"predicted_views_48h,omitempty"`  // 预测 48h 播放量
}

// ScoreVideos 对视频列表进行多维评分并排序
func ScoreVideos(videos []Video, scorer ScorerType) []ScoredVideo {
	if len(videos) == 0 {
		return nil
	}

	// 计算最大播放量以归一化
	var maxViews int64
	for _, v := range videos {
		if v.ViewCount > maxViews {
			maxViews = v.ViewCount
		}
	}
	maxLogViews := math.Log(float64(maxViews + 1))

	now := time.Now()
	scored := make([]ScoredVideo, 0, len(videos))

	for _, v := range videos {
		viewScore, recencyScore, durationScore := videoBaseScores(v, maxLogViews, now)
		sv := ScoredVideo{
			Video:         v,
			ViewScore:     viewScore,
			RecencyScore:  recencyScore,
			DurationScore: durationScore,
		}

		// 按评分策略计算综合得分
		switch scorer {
		case ScorerPopular:
			sv.Score = sv.ViewScore*0.7 + sv.RecencyScore*0.3
		case ScorerFresh:
			sv.Score = sv.RecencyScore*0.8 + sv.ViewScore*0.2
		case ScorerBalanced:
			sv.Score = sv.ViewScore*0.34 + sv.RecencyScore*0.33 + sv.DurationScore*0.33
		default:
			sv.Score = sv.ViewScore*0.7 + sv.RecencyScore*0.3
		}

		scored = append(scored, sv)
	}

	// 按综合得分降序排列
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})

	return scored
}

// videoBaseScores 计算播放量/新鲜度/时长三个基础分。
func videoBaseScores(v Video, maxLogViews float64, now time.Time) (viewScore, recencyScore, durationScore float64) {
	// 播放量评分（对数归一化，防止极值倾斜）
	if maxLogViews > 0 {
		viewScore = math.Log(float64(v.ViewCount+1)) / maxLogViews
	}

	// 新鲜度评分
	pubTime := parsePublishedTime(v.PublishTime, now)
	if !pubTime.IsZero() {
		daysAgo := now.Sub(pubTime).Hours() / 24
		switch {
		case daysAgo <= 1:
			recencyScore = 1.0
		case daysAgo <= 7:
			recencyScore = 1.0 - (daysAgo-1)*0.5/6
		case daysAgo <= 30:
			recencyScore = 0.5 - (daysAgo-7)*0.4/23
		case daysAgo <= 90:
			recencyScore = 0.1 - math.Max(daysAgo-30, 0)*0.1/60
		default:
			recencyScore = 0
		}
	} else {
		recencyScore = 0.5 // 无法解析时给默认值
	}

	// 时长评分（10-30 分钟为最佳）
	switch {
	case v.DurationSec <= 0:
		durationScore = 0.5
	case v.DurationSec < 120:
		durationScore = 0.1 // < 2 分钟，太短
	case v.DurationSec <= 600:
		durationScore = 0.5 // 2-10 分钟
	case v.DurationSec <= 1800:
		durationScore = 1.0 // 10-30 分钟，最佳区间
	case v.DurationSec <= 3600:
		durationScore = 0.7 // 30-60 分钟
	default:
		durationScore = 0.3 // > 60 分钟，太长
	}
	return viewScore, recencyScore, durationScore
}

// ─── ytsubs nowcast 完整算法（Phase 1）─────────────────────────
// 参考 https://github.com/shayne/ytsubs generate_feed.py
// 核心分数 = (0.55*nowcast + 0.20*velocity + 0.15*reach + 0.05*duration) * confidence + breakout

// ageCurveFraction48h 48h 年龄曲线：视频发布后各时段应达到的基线播放比例。
//   0-8h：线性爬升到 60%；8-48h：缓升到 95%；48h+：平台期。
func ageCurveFraction48h(ageHours float64) float64 {
	switch {
	case ageHours <= 0:
		return 0.03
	case ageHours <= 8:
		return math.Max(0.03, 0.6*(ageHours/8.0))
	case ageHours < 48:
		return 0.6 + 0.35*((ageHours-8.0)/40.0)
	default:
		return 0.95
	}
}

// ageCurveExpectedSlope 期望播放速率斜率（每小时应新增的基线比例）。
//   0-8h：高速期 7.5%/h；8-48h：缓速期 0.875%/h；48h+：长尾 0.1%/h。
func ageCurveExpectedSlope(ageHours float64) float64 {
	switch {
	case ageHours <= 0:
		return 0.075
	case ageHours <= 8:
		return 0.075
	case ageHours < 48:
		return 0.00875
	default:
		return 0.001
	}
}

// clamp 数值限幅。
func clampF(v, lo, hi float64) float64 {
	return math.Max(lo, math.Min(hi, v))
}

// normRatio 对数饱和归一化：value 越大越接近 1，cap 处为 1（默认 6 倍）。
func normRatio(value, cap float64) float64 {
	if value <= 0 {
		return 0
	}
	return clampF(math.Log1p(value)/math.Log1p(cap), 0, 1)
}

// normReach 播放/粉丝数归一化：sqrt(reach*10)，小频道高转化也能浮上来。
func normReach(reach float64) float64 {
	if reach <= 0 {
		return 0
	}
	return clampF(math.Sqrt(reach*10.0), 0, 1)
}

// confidenceMultiplier 置信度乘数（0.75-1.05）：
//   - 数据完整度：时长/发布时间缺失 → 降权
//   - 基线新鲜度：baselineUpdatedAt 距今 <24h → 1.0；越旧越低（>7 天 → 0.78）
//   - 基线规模：样本太少或基线为 0 → 降权
func confidenceMultiplier(v Video, baselineUpdatedAt time.Time, now time.Time, hasBaseline bool) float64 {
	confidence := 1.0

	// 数据完整度
	if v.DurationSec <= 0 {
		confidence -= 0.10
	}
	if v.PublishTime == "" {
		confidence -= 0.10
	}

	// 基线新鲜度（有基线时）
	if hasBaseline && !baselineUpdatedAt.IsZero() {
		age := now.Sub(baselineUpdatedAt).Hours()
		switch {
		case age < 24:
			confidence *= 1.0
		case age < 72:
			confidence *= 0.95
		case age < 168:
			confidence *= 0.88
		default:
			confidence *= 0.78
		}
	} else if !hasBaseline {
		// 无频道基线（用候选集参照）→ 保守降权（映射后 <1.0）
		confidence *= 0.80
	}

	// 映射到 0.75-1.05 区间
	return clampF(0.75+0.30*clampF(confidence, 0, 1), 0.75, 1.05)
}

// earlyBreakoutBoost 早期爆发加成（+0~0.12）：
//   新视频（<24h）同时 nowcast 强（>1.2 倍基线）且 velocity 强（>1.2 倍期望）→ 加成。
//   公式 0.03*ln(1+nowcast*velocity)，封顶 0.12。
func earlyBreakoutBoost(ageHours, relativeNowcast, velocityShock float64) float64 {
	if ageHours > 24 || relativeNowcast < 1.2 || velocityShock < 1.2 {
		return 0
	}
	return clampF(0.03*math.Log1p(relativeNowcast*velocityShock), 0, 0.12)
}

// NowcastBaseline 频道基线信息（扩展：含粉丝数、基线时间戳）。
type NowcastBaseline struct {
	Baseline     float64   // 48h 预期播放量（频道常态）
	Subscribers  int64     // 频道粉丝数
	UpdatedAt    time.Time // 基线采集时间（用于置信度）
	HasBaseline  bool      // 是否有真实基线
}

// ScoreVideosNowcast 按 ytsubs nowcast 完整算法评分（Phase 1 增强版）。
// baselines: map[channelID]NowcastBaseline；频道缺失时退化为纯播放/时效评分。
func ScoreVideosNowcast(videos []Video, baselines map[string]float64) []ScoredVideo {
	// 兼容旧签名：包装为完整版（无粉丝数/时间戳信息）
	full := make(map[string]NowcastBaseline, len(baselines))
	for cid, b := range baselines {
		full[cid] = NowcastBaseline{Baseline: b, HasBaseline: b > 0}
	}
	return ScoreVideosNowcastFull(videos, full)
}

// ScoreVideosNowcastFull ytsubs nowcast 完整评分（Phase 1）。
func ScoreVideosNowcastFull(videos []Video, baselines map[string]NowcastBaseline) []ScoredVideo {
	if len(videos) == 0 {
		return nil
	}
	now := time.Now()
	scored := make([]ScoredVideo, 0, len(videos))

	// 候选集播放量中位数作为"频道基线缺失"时的参照
	var viewCounts []float64
	var maxViews int64
	for _, v := range videos {
		viewCounts = append(viewCounts, float64(v.ViewCount))
		if v.ViewCount > maxViews {
			maxViews = v.ViewCount
		}
	}
	setReference := medianF(viewCounts)
	maxLogViews := math.Log(float64(maxViews + 1))

	for _, v := range videos {
		// 基础分（view/recency/duration）
		viewScore, recencyScore, durationScore := videoBaseScores(v, maxLogViews, now)

		// 基线解析
		bl, hasBaseline := baselines[v.ChannelID]
		baseline := bl.Baseline
		if !hasBaseline || baseline <= 0 {
			baseline = setReference
			bl = NowcastBaseline{Baseline: baseline, HasBaseline: false}
		}

		// 年龄
		pubTime := parsePublishedTime(v.PublishTime, now)
		ageHours := 24.0
		if !pubTime.IsZero() {
			ageHours = math.Max(0.5, now.Sub(pubTime).Hours())
		}

		// 1. Nowcast vs Expected（55%）：当前播放 / 按年龄曲线预期的播放
		expectedFraction := ageCurveFraction48h(ageHours)
		expectedViewsNow := math.Max(1.0, baseline*expectedFraction)
		relativeNowcast := float64(v.ViewCount+1) / expectedViewsNow
		nowcastScore := normRatio(relativeNowcast, 6.0)

		// 2. Velocity Shock（20%）：实际播放速率 vs 期望斜率
		expectedSlope := ageCurveExpectedSlope(ageHours)
		expectedVPH := math.Max(1.0, baseline*expectedSlope)
		actualVPH := float64(v.ViewCount+1) / math.Max(ageHours, 1.0)
		velocityShock := actualVPH / expectedVPH
		velocityScore := normRatio(velocityShock, 4.0) * recencyScore

		// 3. Subscriber Reach（15%）：播放/粉丝数（对数饱和）
		var reachScore float64
		if bl.Subscribers > 0 {
			reach := float64(v.ViewCount) / float64(bl.Subscribers)
			reachScore = normReach(reach)
		} else {
			reachScore = viewScore // 无粉丝数 → 退化为绝对播放量
		}

		// 4. Duration Prior（5%）
		// 复用 durationScore（10-30 分钟最佳）

		// 置信度乘数
		confidence := confidenceMultiplier(v, bl.UpdatedAt, now, bl.HasBaseline)

		// Early Breakout Boost
		breakout := earlyBreakoutBoost(ageHours, relativeNowcast, velocityShock)

		// 综合（对齐 ytsubs 权重）
		baseScore := 0.55*nowcastScore + 0.20*velocityScore + 0.15*reachScore + 0.05*durationScore
		coreScore := (baseScore * confidence) + breakout

		// 预测 48h 播放（复盘/阈值用）
		var predicted48h float64
		if ageHours >= 48 {
			predicted48h = float64(v.ViewCount)
		} else {
			predicted48h = float64(v.ViewCount) * 0.95 / math.Max(expectedFraction, 0.03)
		}

		sv := ScoredVideo{
			Video:         v,
			Score:         coreScore,
			ViewScore:     viewScore,
			RecencyScore:  recencyScore,
			DurationScore: durationScore,
			NowcastScore:  nowcastScore,
			VelocityScore: velocityScore,
			ReachScore:    reachScore,
			Confidence:    confidence,
			BreakoutBoost: breakout,
			ExpectedViews: expectedViewsNow,
			Predicted48h:  predicted48h,
		}
		scored = append(scored, sv)
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})
	return scored
}

// medianF 计算 []float64 的中位数。
func medianF(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	sorted := append([]float64(nil), xs...)
	sort.Float64s(sorted)
	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}

// parsePublishedTime 解析 YouTube 发布时间的相对字符串
// 例如: "3 hours ago", "1 year ago", "2 months ago"
func parsePublishedTime(s string, now time.Time) time.Time {
	if s == "" {
		return time.Time{}
	}

	// 移除常见前缀
	cleaned := strings.TrimPrefix(s, "Streamed ")
	cleaned = strings.TrimPrefix(cleaned, "Premiered ")
	cleaned = strings.TrimPrefix(cleaned, "Started ")

	parts := strings.Fields(cleaned)
	// 期望格式: "X unit(s) ago"
	if len(parts) < 3 || parts[len(parts)-1] != "ago" {
		return time.Time{}
	}

	numStr := parts[0]
	unit := strings.TrimSuffix(parts[1], "s") // 复数 → 单数

	num, err := strconv.Atoi(numStr)
	if err != nil || num <= 0 {
		return time.Time{}
	}

	switch unit {
	case "second":
		return now.Add(-time.Duration(num) * time.Second)
	case "minute":
		return now.Add(-time.Duration(num) * time.Minute)
	case "hour":
		return now.Add(-time.Duration(num) * time.Hour)
	case "day":
		return now.AddDate(0, 0, -num)
	case "week":
		return now.AddDate(0, 0, -num*7)
	case "month":
		return now.AddDate(0, -num, 0)
	case "year":
		return now.AddDate(-num, 0, 0)
	}

	return time.Time{}
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
