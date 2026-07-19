package translator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/zolagz/ytb2bili-go/internal/config"
	"github.com/zolagz/ytb2bili-go/internal/llm"
)

const (
	sentenceBreak = "###SENTENCE_BREAK###"
)

// Config 翻译配置
type Config struct {
	APIKey      string
	BaseURL     string
	Model       string
	SourceLang  string
	TargetLang  string
	BatchSize   int
	MaxWorkers  int
	RetryCount  int
	ContextSize int
}

// Result 翻译结果
type Result struct {
	OriginalTexts   []string
	TranslatedTexts []string
	Duration        time.Duration
}

// SRTEntry SRT 条目
type SRTEntry struct {
	Index    int
	TimeCode string
	Text     string
}

// Translator 批量翻译器
type Translator struct {
	config Config
	client *http.Client
}

// New 创建翻译器
func New(config Config) *Translator {
	if config.BatchSize <= 0 {
		config.BatchSize = 3
	}
	if config.MaxWorkers <= 0 {
		config.MaxWorkers = 3
	}
	if config.RetryCount <= 0 {
		config.RetryCount = 2
	}
	if config.ContextSize < 0 {
		config.ContextSize = 2
	}
	if config.SourceLang == "" {
		config.SourceLang = "en"
	}
	if config.TargetLang == "" {
		config.TargetLang = "zh"
	}

	return &Translator{
		config: config,
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

// TranslateSRTFile 翻译 SRT 文件
func (t *Translator) TranslateSRTFile(ctx context.Context, inputPath, outputPath string) error {
	fmt.Printf("  读取字幕文件: %s\n", inputPath)

	// 1. 读取源文件
	raw, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("读取字幕文件失败: %w", err)
	}

	// 2. 解析 SRT
	entries, err := ParseSRT(string(raw))
	if err != nil {
		return fmt.Errorf("解析 SRT 文件失败: %w", err)
	}
	if len(entries) == 0 {
		return fmt.Errorf("字幕文件为空")
	}
	entries = DeduplicateRollingEntries(entries)
	if len(entries) == 0 {
		return fmt.Errorf("字幕去重后为空")
	}
	fmt.Printf("  解析完成: %d 条字幕\n", len(entries))

	// 3. 提取纯文本
	texts := make([]string, len(entries))
	for i, e := range entries {
		texts[i] = e.Text
	}

	// 4. 批量翻译
	fmt.Printf("  开始批量翻译 (batch=%d, workers=%d)...\n", t.config.BatchSize, t.config.MaxWorkers)
	result, err := t.TranslateTexts(ctx, texts)
	if err != nil {
		return fmt.Errorf("翻译失败: %w", err)
	}
	fmt.Printf("  翻译完成: %d/%d 条, 耗时 %v\n", len(result.TranslatedTexts), len(texts), result.Duration)

	// 5. 生成译文 SRT
	entries, translatedTexts := DeduplicateTranslations(entries, result.TranslatedTexts)
	content := GenerateSRT(entries, translatedTexts)
	fmt.Printf("  保存前去重完成: %d 条字幕\n", len(entries))

	// 6. 写入输出文件
	if err := writeFileAtomic(outputPath, []byte(content)); err != nil {
		return fmt.Errorf("保存翻译字幕失败: %w", err)
	}
	fmt.Printf("  保存到: %s\n", outputPath)

	return nil
}

func writeFileAtomic(path string, content []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(dir, ".translated-*.srt")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Chmod(0644); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

// TranslateTexts 批量翻译文本
func (t *Translator) TranslateTexts(ctx context.Context, texts []string) (*Result, error) {
	startTime := time.Now()

	if len(texts) == 0 {
		return &Result{
			OriginalTexts:   texts,
			TranslatedTexts: []string{},
			Duration:        time.Since(startTime),
		}, nil
	}

	totalGroups := (len(texts) + t.config.BatchSize - 1) / t.config.BatchSize
	fmt.Printf("  分组: %d 组, 每组最多 %d 句\n", totalGroups, t.config.BatchSize)

	// 并发翻译
	type taskResult struct {
		groupIndex int
		result     []string
		err        error
	}

	taskChan := make(chan int, totalGroups)
	resultChan := make(chan taskResult, totalGroups)

	// 启动 workers
	for i := 0; i < t.config.MaxWorkers; i++ {
		go func() {
			for groupIdx := range taskChan {
				start := groupIdx * t.config.BatchSize
				end := start + t.config.BatchSize
				if end > len(texts) {
					end = len(texts)
				}

				currentGroup := texts[start:end]

				// 获取上下文
				var prevContext, nextContext []string
				if start > 0 && t.config.ContextSize > 0 {
					prevStart := start - t.config.ContextSize
					if prevStart < 0 {
						prevStart = 0
					}
					prevContext = texts[prevStart:start]
				}
				if end < len(texts) && t.config.ContextSize > 0 {
					nextEnd := end + t.config.ContextSize
					if nextEnd > len(texts) {
						nextEnd = len(texts)
					}
					nextContext = texts[end:nextEnd]
				}

				translated, err := t.translateGroupWithRetry(ctx, currentGroup, prevContext, nextContext)
				resultChan <- taskResult{groupIndex: groupIdx, result: translated, err: err}
			}
		}()
	}

	// 发送任务
	go func() {
		for i := 0; i < totalGroups; i++ {
			taskChan <- i
		}
		close(taskChan)
	}()

	// 收集结果
	results := make(map[int][]string)
	var lastErr error

	for i := 0; i < totalGroups; i++ {
		res := <-resultChan
		if res.err != nil {
			lastErr = res.err
			continue
		}
		results[res.groupIndex] = res.result
		fmt.Printf("  组 %d/%d 翻译完成\n", res.groupIndex+1, totalGroups)
	}

	if lastErr != nil {
		return nil, lastErr
	}

	// 合并结果
	var allTranslated []string
	for i := 0; i < totalGroups; i++ {
		if groupResult, ok := results[i]; ok {
			allTranslated = append(allTranslated, groupResult...)
		}
	}

	return &Result{
		OriginalTexts:   texts,
		TranslatedTexts: allTranslated,
		Duration:        time.Since(startTime),
	}, nil
}

func (t *Translator) translateGroupWithRetry(ctx context.Context, texts []string, prevContext, nextContext []string) ([]string, error) {
	var lastErr error

	for attempt := 0; attempt <= t.config.RetryCount; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}

		result, err := t.translateGroup(ctx, texts, prevContext, nextContext)
		if err == nil {
			return result, nil
		}
		lastErr = err
	}

	return nil, fmt.Errorf("翻译失败 (重试 %d 次): %w", t.config.RetryCount, lastErr)
}

func (t *Translator) translateGroup(ctx context.Context, texts []string, prevContext, nextContext []string) ([]string, error) {
	// 构建上下文
	var fullTexts []string
	targetStart := 0

	if len(prevContext) > 0 {
		fullTexts = append(fullTexts, prevContext...)
		targetStart = len(fullTexts)
	}
	fullTexts = append(fullTexts, texts...)
	targetEnd := len(fullTexts)
	if len(nextContext) > 0 {
		fullTexts = append(fullTexts, nextContext...)
	}

	// 构建提示词
	contextInfo := ""
	if len(prevContext) > 0 || len(nextContext) > 0 {
		contextInfo = fmt.Sprintf(`

上下文信息：
- 前置上下文：%d 句（仅供参考，不需要翻译）
- 目标翻译：%d 句（位于第 %d-%d 句，需要全部翻译）
- 后置上下文：%d 句（仅供参考，不需要翻译）

请只翻译目标部分（第 %d-%d 句），但要充分考虑前后文的连贯性。`,
			len(prevContext), len(texts), targetStart+1, targetEnd,
			len(nextContext), targetStart+1, targetEnd)
	}

	systemPrompt := fmt.Sprintf(`你是一个专业的视频字幕翻译专家。我将给你一段连续的%s字幕，其中包含 %d 句需要翻译的内容。%s

翻译要求：
1. 自然流畅：使用口语化表达，符合%s字幕习惯
2. 上下文连贯：理解整体语境，确保翻译前后呼应
3. 准确传神：忠实原文含义，保持语气和情感
4. 简洁明了：字幕需要快速阅读，避免冗长
5. 数量严格：必须输出 %d 句翻译，不多不少
6. 分隔符：每句翻译用"%s"分隔

输入格式：句子用"%s"分隔
输出格式：只返回目标部分的%s翻译，用"%s"分隔

注意：只返回翻译的%s文本，不要添加序号、解释或其他内容。`,
		getLangName(t.config.SourceLang),
		len(texts),
		contextInfo,
		getLangName(t.config.TargetLang),
		len(texts),
		sentenceBreak,
		sentenceBreak,
		getLangName(t.config.TargetLang),
		sentenceBreak,
		getLangName(t.config.TargetLang))

	// 组合输入
	combinedText := strings.Join(fullTexts, "\n"+sentenceBreak+"\n")

	// 调用 LLM
	response, err := t.callLLM(ctx, systemPrompt, combinedText)
	if err != nil {
		return nil, err
	}

	// 解析结果
	translated := strings.Split(response, sentenceBreak)
	for i := range translated {
		translated[i] = strings.TrimSpace(translated[i])
	}
	// Some models return the supplied context despite being asked for only the
	// target sentences. This shape is unambiguous, so retain the target slice.
	if len(translated) == len(fullTexts) && len(fullTexts) != len(texts) {
		translated = translated[targetStart:targetEnd]
	}
	if len(texts) == 1 && len(translated) > 1 {
		translated = []string{strings.Join(translated, "")}
	}
	if len(translated) != len(texts) {
		return nil, fmt.Errorf("翻译数量不匹配: 期望 %d 句，实际 %d 句", len(texts), len(translated))
	}

	return translated, nil
}

func (t *Translator) callLLM(ctx context.Context, systemPrompt, userContent string) (string, error) {
	messages := []map[string]string{
		{"role": "system", "content": systemPrompt},
		{"role": "user", "content": userContent},
	}

	payload := map[string]interface{}{
		"model":       t.config.Model,
		"messages":    messages,
		"temperature": 0.3,
		"max_tokens":  4096,
	}
	payloadBytes, _ := json.Marshal(payload)

	req, _ := http.NewRequestWithContext(ctx, "POST", t.config.BaseURL+"/chat/completions", bytes.NewReader(payloadBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+t.config.APIKey)

	resp, err := t.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("LLM 请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("解析 LLM 响应失败: %s", string(body)[:min(200, len(body))])
	}

	if result.Error != nil {
		return "", fmt.Errorf("LLM 错误: %s", result.Error.Message)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("LLM 返回空结果")
	}

	return result.Choices[0].Message.Content, nil
}

// ParseSRT 解析 SRT 文件
func ParseSRT(content string) ([]SRTEntry, error) {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(content, "\n")

	var entries []SRTEntry
	var current SRTEntry
	var textLines []string
	stage := 0

	flush := func() {
		if stage == 2 && len(textLines) > 0 {
			current.Text = strings.Join(textLines, "\n")
			entries = append(entries, current)
		}
		current = SRTEntry{}
		textLines = nil
		stage = 0
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			flush()
			continue
		}

		switch stage {
		case 0:
			var idx int
			if _, err := fmt.Sscanf(line, "%d", &idx); err == nil {
				current.Index = idx
				stage = 1
			}
		case 1:
			if strings.Contains(line, "-->") {
				current.TimeCode = line
				stage = 2
			}
		case 2:
			textLines = append(textLines, line)
		}
	}
	flush()

	return entries, nil
}

// DeduplicateRollingEntries removes lines repeated by YouTube rolling captions.
// A line is emitted only when it is not present in the immediately preceding cue.
func DeduplicateRollingEntries(entries []SRTEntry) []SRTEntry {
	result := make([]SRTEntry, 0, len(entries))
	previous := map[string]struct{}{}
	for _, entry := range entries {
		current := map[string]struct{}{}
		var additions []string
		for _, line := range strings.Split(entry.Text, "\n") {
			line = strings.Join(strings.Fields(line), " ")
			if line == "" {
				continue
			}
			current[line] = struct{}{}
			if _, repeated := previous[line]; !repeated {
				additions = append(additions, line)
			}
		}
		previous = current
		if len(additions) == 0 {
			continue
		}
		entry.Index = len(result) + 1
		entry.Text = strings.Join(additions, " ")
		result = append(result, entry)
	}
	return result
}

// DeduplicateTranslations removes consecutive identical translated captions
// before writing the output file and keeps SRT indices contiguous.
func DeduplicateTranslations(entries []SRTEntry, texts []string) ([]SRTEntry, []string) {
	filteredEntries := make([]SRTEntry, 0, len(entries))
	filteredTexts := make([]string, 0, len(texts))
	previous := ""
	for i, entry := range entries {
		text := entry.Text
		if i < len(texts) && strings.TrimSpace(texts[i]) != "" {
			text = strings.TrimSpace(texts[i])
		}
		normalized := strings.Join(strings.Fields(text), " ")
		if normalized == "" || normalized == previous {
			continue
		}
		entry.Index = len(filteredEntries) + 1
		filteredEntries = append(filteredEntries, entry)
		filteredTexts = append(filteredTexts, text)
		previous = normalized
	}
	return filteredEntries, filteredTexts
}

// GenerateSRT 生成 SRT 内容
func GenerateSRT(entries []SRTEntry, translatedTexts []string) string {
	var sb strings.Builder
	for i, entry := range entries {
		fmt.Fprintf(&sb, "%d\n", entry.Index)
		fmt.Fprintf(&sb, "%s\n", entry.TimeCode)
		if i < len(translatedTexts) && translatedTexts[i] != "" {
			fmt.Fprintf(&sb, "%s\n\n", translatedTexts[i])
		} else {
			fmt.Fprintf(&sb, "%s\n\n", entry.Text)
		}
	}
	return sb.String()
}

func getLangName(code string) string {
	names := map[string]string{
		"en":      "英文",
		"zh":      "中文",
		"zh-CN":   "中文",
		"zh-Hans": "中文简体",
		"ja":      "日文",
		"ko":      "韩文",
	}
	if name, ok := names[code]; ok {
		return name
	}
	return code
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ParseSRTTime 解析 SRT 时间码
func ParseSRTTime(t string) float64 {
	t = strings.ReplaceAll(t, ",", ".")
	parts := strings.Split(t, ":")
	if len(parts) != 3 {
		return 0
	}
	h, _ := strconv.ParseFloat(parts[0], 64)
	m, _ := strconv.ParseFloat(parts[1], 64)
	s, _ := strconv.ParseFloat(parts[2], 64)
	return h*3600 + m*60 + s
}

// SRT 翻译 SRT 字幕文件（兼容旧接口）
func SRT(inputPath, sourceLang, targetLang string, cfg interface{}) (string, error) {
	return SRTContext(context.Background(), inputPath, sourceLang, targetLang, cfg)
}

func SRTContext(ctx context.Context, inputPath, sourceLang, targetLang string, cfg interface{}) (string, error) {
	var apiKey, baseURL, model string

	switch c := cfg.(type) {
	case *Config:
		apiKey = c.APIKey
		baseURL = c.BaseURL
		model = c.Model
	case *config.Config:
		apiKey = c.LLMAPIKey
		baseURL = c.LLMBaseURL
		model = c.LLMModel
	default:
		return "", fmt.Errorf("unsupported config type")
	}

	// 生成输出路径
	outputPath := TranslatedSRTPath(inputPath, targetLang)

	// 创建翻译器
	translator := New(Config{
		APIKey:      apiKey,
		BaseURL:     baseURL,
		Model:       model,
		SourceLang:  sourceLang,
		TargetLang:  targetLang,
		BatchSize:   3,
		MaxWorkers:  3,
		ContextSize: 3,
	})

	// 翻译
	if err := translator.TranslateSRTFile(ctx, inputPath, outputPath); err != nil {
		return "", err
	}

	return outputPath, nil
}

// TranslatedSRTPath returns a stable output path without appending the target
// language twice when an already translated file is passed in.
func TranslatedSRTPath(inputPath, targetLang string) string {
	ext := filepath.Ext(inputPath)
	base := strings.TrimSuffix(inputPath, ext)
	suffix := "." + targetLang
	if strings.HasSuffix(strings.ToLower(base), strings.ToLower(suffix)) {
		return base + ".srt"
	}
	return base + suffix + ".srt"
}

// CallLLM 调用 LLM（兼容旧接口）
func CallLLM(prompt string, cfg interface{}) (string, error) {
	return CallLLMContext(context.Background(), prompt, cfg)
}

func CallLLMContext(ctx context.Context, prompt string, cfg interface{}) (string, error) {
	var apiKey, baseURL, model string

	switch c := cfg.(type) {
	case *Config:
		apiKey = c.APIKey
		baseURL = c.BaseURL
		model = c.Model
	case *config.Config:
		apiKey = c.LLMAPIKey
		baseURL = c.LLMBaseURL
		model = c.LLMModel
	default:
		return "", fmt.Errorf("unsupported config type")
	}

	return (&llm.OpenAIClient{APIKey: apiKey, BaseURL: baseURL, Model: model}).Complete(ctx, prompt)
}
