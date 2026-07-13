package search

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
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

	req, err := http.NewRequest("POST", searchURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", webUserAgent)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Origin", "https://www.youtube.com")
	req.Header.Set("Referer", "https://www.youtube.com/")

	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("解析 JSON 失败: %w", err)
	}

	videos, nextContinuation := parseInnerTubeResponseWithContinuation(data, s.MaxResults)

	return &SearchResult{
		Query:        query,
		Videos:       videos,
		TotalFound:   len(videos),
		Continuation: nextContinuation,
	}, nil
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
func ExtractVideoID(input string) string {
	// 直接的视频 ID (11 位)
	if matched, _ := regexp.MatchString(`^[a-zA-Z0-9_-]{11}$`, input); matched {
		return input
	}

	// YouTube URL
	patterns := []string{
		`(?:youtube\.com/watch\?v=|youtu\.be/|youtube\.com/embed/)([a-zA-Z0-9_-]{11})`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(input); len(matches) > 1 {
			return matches[1]
		}
	}

	return ""
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

// StandardKeywords 标准化搜索关键词库（四大类）
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
		Category: "web-dev",
		Keywords: []string{
			"Next.js React Flow workflow editor App Router",
			"xyflow react drag drop agent dashboard development",
			"FastAPI backend agent API design tutorial",
			"TypeScript AI workflow frontend architecture",
			"Docker Hermes agent deployment guide",
			"Full stack AI media generation pipeline",
			"Tailwind CSS agent UI design tutorial",
			"Python async agent backend architecture",
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
