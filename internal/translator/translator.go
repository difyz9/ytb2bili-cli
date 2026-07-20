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
	"sync"
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
	OriginalTexts      []string
	TranslatedTexts    []string
	Duration           time.Duration
	Errors             []error
	SkippedTranslation bool
	DetectedLanguage   string
}

type translateTask struct {
	groupIndex  int
	texts       []string
	prevContext []string
	nextContext []string
}

type translateResult struct {
	groupIndex int
	texts      []string
	err        error
}

// SRTEntry SRT 条目
type SRTEntry struct {
	Index    int
	TimeCode string
	Text     string
}

// translationPlan separates semantic translation units from the immutable SRT
// structure. Repeated lines in rolling captions share one translated unit and
// are projected back to every original cue after translation.
type translationPlan struct {
	units      []string
	entryUnits [][]int
}

// Translator 批量翻译器
type Translator struct {
	config Config
	client *http.Client
}

// New 创建翻译器
func New(config Config) *Translator {
	if config.BatchSize <= 0 {
		config.BatchSize = 25
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
		config.TargetLang = "zh-Hans"
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
	fmt.Printf("  解析完成: %d 条字幕\n", len(entries))

	// 3. 建立滚动字幕翻译计划。同一原子文本行只翻译一次，再回填到每个
	// 原始 cue，从而保持重复片段的译法稳定，同时不改变 SRT 结构。
	plan, err := buildTranslationPlan(entries)
	if err != nil {
		return fmt.Errorf("建立字幕翻译计划失败: %w", err)
	}
	fmt.Printf("  语义翻译单元: %d 条 (回填到 %d 条原始字幕)\n", len(plan.units), len(entries))

	// 4. 批量翻译
	fmt.Printf("  开始批量翻译 (batch=%d, workers=%d)...\n", t.config.BatchSize, t.config.MaxWorkers)
	result, err := t.TranslateTexts(ctx, plan.units)
	if err != nil {
		return fmt.Errorf("翻译失败: %w", err)
	}
	fmt.Printf("  翻译完成: %d/%d 个语义单元, 耗时 %v\n", len(result.TranslatedTexts), len(plan.units), result.Duration)
	translatedTexts, err := plan.project(result.TranslatedTexts)
	if err != nil {
		return fmt.Errorf("回填翻译结果失败: %w", err)
	}

	// 5. 一对一生成译文 SRT。翻译器必须保留输入字幕的条数、序号和时间轴；
	// 滚动字幕清理属于独立的显式预处理步骤，不在翻译过程中执行。
	if len(translatedTexts) != len(entries) {
		return fmt.Errorf("翻译结果数量不匹配: 输入 %d 条，输出 %d 条", len(entries), len(translatedTexts))
	}
	content := GenerateSRT(entries, translatedTexts)
	fmt.Printf("  保留原始字幕结构: %d 条\n", len(entries))

	// 6. 写入输出文件
	if err := writeFileAtomic(outputPath, []byte(content)); err != nil {
		return fmt.Errorf("保存翻译字幕失败: %w", err)
	}
	fmt.Printf("  保存到: %s\n", outputPath)

	return nil
}

func buildTranslationPlan(entries []SRTEntry) (*translationPlan, error) {
	plan := &translationPlan{entryUnits: make([][]int, len(entries))}
	unitByText := make(map[string]int)
	for entryIndex, entry := range entries {
		lines := strings.Split(strings.ReplaceAll(entry.Text, "\r\n", "\n"), "\n")
		for _, line := range lines {
			normalized := strings.Join(strings.Fields(line), " ")
			if normalized == "" {
				continue
			}
			unitIndex, exists := unitByText[normalized]
			if !exists {
				unitIndex = len(plan.units)
				unitByText[normalized] = unitIndex
				plan.units = append(plan.units, normalized)
			}
			plan.entryUnits[entryIndex] = append(plan.entryUnits[entryIndex], unitIndex)
		}
		if len(plan.entryUnits[entryIndex]) == 0 {
			return nil, fmt.Errorf("第 %d 条字幕没有可翻译文本", entry.Index)
		}
	}
	return plan, nil
}

func (p *translationPlan) project(translatedUnits []string) ([]string, error) {
	if len(translatedUnits) != len(p.units) {
		return nil, fmt.Errorf("语义单元数量不匹配: 期望 %d，实际 %d", len(p.units), len(translatedUnits))
	}
	projected := make([]string, len(p.entryUnits))
	for entryIndex, unitIndexes := range p.entryUnits {
		lines := make([]string, 0, len(unitIndexes))
		for _, unitIndex := range unitIndexes {
			if unitIndex < 0 || unitIndex >= len(translatedUnits) {
				return nil, fmt.Errorf("第 %d 条字幕引用了无效语义单元 %d", entryIndex+1, unitIndex)
			}
			translation := strings.TrimSpace(translatedUnits[unitIndex])
			if translation == "" {
				return nil, fmt.Errorf("第 %d 个语义单元翻译为空", unitIndex+1)
			}
			lines = append(lines, translation)
		}
		projected[entryIndex] = strings.Join(lines, "\n")
	}
	return projected, nil
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
	shouldTranslate, detectedLanguage, decisionErr := t.shouldTranslate(ctx, texts)
	if decisionErr != nil {
		fmt.Printf("  翻译语言判定失败，将继续翻译: %v\n", decisionErr)
		shouldTranslate = true
	}
	if !shouldTranslate {
		copied := append([]string(nil), texts...)
		return &Result{
			OriginalTexts: texts, TranslatedTexts: copied,
			Duration: time.Since(startTime), SkippedTranslation: true,
			DetectedLanguage: detectedLanguage,
		}, nil
	}

	totalGroups := (len(texts) + t.config.BatchSize - 1) / t.config.BatchSize
	fmt.Printf("  分组: %d 组, 每组最多 %d 句\n", totalGroups, t.config.BatchSize)

	taskChan := make(chan translateTask)
	resultChan := make(chan translateResult, totalGroups)

	// 启动 workers
	var workers sync.WaitGroup
	for i := 0; i < t.config.MaxWorkers; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for task := range taskChan {
				translated, err := t.translateGroupWithRetry(ctx, task.texts, task.prevContext, task.nextContext)
				if err != nil {
					err = fmt.Errorf("第 %d/%d 组: %w", task.groupIndex+1, totalGroups, err)
				}
				select {
				case resultChan <- translateResult{groupIndex: task.groupIndex, texts: translated, err: err}:
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	// 发送任务
	go func() {
		defer close(taskChan)
		for groupIndex := 0; groupIndex < totalGroups; groupIndex++ {
			start := groupIndex * t.config.BatchSize
			end := min(start+t.config.BatchSize, len(texts))
			prevStart := start
			if t.config.ContextSize > 0 {
				prevStart = max(0, start-t.config.ContextSize)
			}
			nextEnd := end
			if t.config.ContextSize > 0 {
				nextEnd = min(len(texts), end+t.config.ContextSize)
			}
			task := translateTask{
				groupIndex: groupIndex, texts: texts[start:end],
				prevContext: texts[prevStart:start], nextContext: texts[end:nextEnd],
			}
			select {
			case taskChan <- task:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		workers.Wait()
		close(resultChan)
	}()

	// 收集结果
	results := make(map[int][]string)
	var translationErrors []error
	for res := range resultChan {
		if res.err != nil {
			translationErrors = append(translationErrors, res.err)
			continue
		}
		results[res.groupIndex] = res.texts
		fmt.Printf("  组 %d/%d 翻译完成\n", res.groupIndex+1, totalGroups)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(translationErrors) > 0 {
		return nil, fmt.Errorf("%d 个翻译组失败，首个错误: %w", len(translationErrors), translationErrors[0])
	}

	// 合并结果
	var allTranslated []string
	for i := 0; i < totalGroups; i++ {
		groupResult, ok := results[i]
		if !ok {
			return nil, fmt.Errorf("翻译结果缺少第 %d/%d 组", i+1, totalGroups)
		}
		allTranslated = append(allTranslated, groupResult...)
	}
	if len(allTranslated) != len(texts) {
		return nil, fmt.Errorf("翻译总数不匹配: 期望 %d 条，实际 %d 条", len(texts), len(allTranslated))
	}

	return &Result{
		OriginalTexts:    texts,
		TranslatedTexts:  allTranslated,
		Duration:         time.Since(startTime),
		Errors:           translationErrors,
		DetectedLanguage: detectedLanguage,
	}, nil
}

func (t *Translator) translateGroupWithRetry(ctx context.Context, texts []string, prevContext, nextContext []string) ([]string, error) {
	var lastErr error

	for attempt := 0; attempt <= t.config.RetryCount; attempt++ {
		if attempt > 0 {
			delay := time.Duration(attempt) * time.Second
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
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
	if len(texts) == 0 {
		return []string{}, nil
	}

	// 构建提示词
	contextInfo := ""
	if len(prevContext) > 0 || len(nextContext) > 0 {
		contextInfo = fmt.Sprintf(`

上下文信息：
- 前置上下文：%d 句（仅供参考，不需要翻译）
- 目标翻译：%d 句（需要逐条翻译）
- 后置上下文：%d 句（仅供参考，不需要翻译）

			请只翻译 target_subtitles，但要充分考虑前后文的连贯性。`,
			len(prevContext), len(texts), len(nextContext))
	}

	systemPrompt := fmt.Sprintf(`你是一个专业的视频字幕翻译专家。我将给你一段连续的%s字幕，其中包含 %d 句需要翻译的内容。%s

翻译要求：
1. 自然流畅：使用口语化表达，符合%s字幕习惯
2. 上下文连贯：理解整体语境，确保翻译前后呼应
3. 准确传神：忠实原文含义，保持语气和情感
4. 简洁明了：字幕需要快速阅读，避免冗长
5. 数量严格：必须输出 %d 句翻译，不多不少
6. 一一对应：即使相邻字幕内容重复，也必须保留并分别翻译，不得合并、去重或省略
7. 索引严格：输出中 index 必须从 1 连续到 %d

输入格式：JSON 对象；previous_context 和 next_context 只供参考，target_subtitles 才需要翻译
输出格式：只返回 JSON：{"translations":[{"index":1,"text":"译文"}]}

	注意：只返回合法 JSON，不要使用 Markdown 代码块，不要添加解释。`,
		getLangName(t.config.SourceLang),
		len(texts),
		contextInfo,
		getLangName(t.config.TargetLang),
		len(texts),
		len(texts))

	targets := make([]map[string]interface{}, len(texts))
	for i, text := range texts {
		targets[i] = map[string]interface{}{"index": i + 1, "text": text}
	}
	inputPayload := map[string]interface{}{
		"previous_context": prevContext,
		"target_subtitles": targets,
		"next_context":     nextContext,
	}
	combinedJSON, err := json.Marshal(inputPayload)
	if err != nil {
		return nil, fmt.Errorf("构建翻译批次失败: %w", err)
	}

	// 调用 LLM
	response, err := t.callLLM(ctx, systemPrompt, string(combinedJSON))
	if err != nil {
		return nil, err
	}

	return parseTranslations(response, len(texts))
}

func parseTranslations(response string, expected int) ([]string, error) {
	var structured struct {
		Translations []struct {
			Index int    `json:"index"`
			Text  string `json:"text"`
		} `json:"translations"`
	}
	if err := json.Unmarshal([]byte(extractJSON(response)), &structured); err == nil && len(structured.Translations) > 0 {
		if len(structured.Translations) != expected {
			return nil, fmt.Errorf("翻译数量不匹配: 当前批次包含 %d 条待翻译字幕，实际返回 %d 条", expected, len(structured.Translations))
		}
		translated := make([]string, expected)
		for position, item := range structured.Translations {
			if item.Index != position+1 {
				return nil, fmt.Errorf("翻译索引不连续: 位置 %d 返回 index=%d", position+1, item.Index)
			}
			if strings.TrimSpace(item.Text) == "" {
				return nil, fmt.Errorf("第 %d 条翻译为空", item.Index)
			}
			translated[position] = strings.TrimSpace(item.Text)
		}
		return translated, nil
	}

	// 兼容 ytb2bili-main 原实现使用的分隔符响应。
	translated := strings.Split(response, sentenceBreak)
	for i := range translated {
		translated[i] = strings.TrimSpace(translated[i])
	}
	if len(translated) != expected {
		return nil, fmt.Errorf("翻译数量不匹配: 当前批次包含 %d 条待翻译字幕，实际返回 %d 条", expected, len(translated))
	}
	return translated, nil
}

func (t *Translator) shouldTranslate(ctx context.Context, texts []string) (bool, string, error) {
	if sameLanguage(t.config.SourceLang, t.config.TargetLang) {
		return false, t.config.SourceLang, nil
	}

	samples := make([]string, 0, 8)
	for _, text := range texts {
		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			continue
		}
		samples = append(samples, trimmed)
		if len(samples) >= 8 {
			break
		}
	}
	if len(samples) == 0 {
		return false, "", nil
	}

	systemPrompt := fmt.Sprintf(`你是字幕翻译前的语言判定器。请判断给定字幕样本是否需要翻译成%s。

规则：
1. 如果字幕主体已经是目标语言，needs_translation=false。
2. 如果字幕主体不是目标语言，needs_translation=true。
3. 混合语言时，以主体语言为准。
4. 只输出 JSON，不要输出解释文字。

输出格式：{"needs_translation":true,"detected_language":"en","reason":"主体为英文"}`,
		getLangName(t.config.TargetLang))

	response, err := t.callLLM(ctx, systemPrompt, strings.Join(samples, "\n"+sentenceBreak+"\n"))
	if err != nil {
		return false, "", fmt.Errorf("翻译语言判定请求失败: %w", err)
	}

	var decision struct {
		NeedsTranslation bool   `json:"needs_translation"`
		DetectedLanguage string `json:"detected_language"`
		Reason           string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(extractJSON(response)), &decision); err != nil {
		return false, "", fmt.Errorf("解析翻译语言判定失败: %w", err)
	}
	fmt.Printf("  语言判定: detected=%s, translate=%t, reason=%s\n",
		decision.DetectedLanguage, decision.NeedsTranslation, decision.Reason)
	return decision.NeedsTranslation, decision.DetectedLanguage, nil
}

func extractJSON(response string) string {
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}")
	if start >= 0 && end >= start {
		return response[start : end+1]
	}
	return response
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
			// Some generated SRT files contain an extra blank line between the
			// time code and its text. It is still the same cue, not a separator.
			if stage == 2 && len(textLines) == 0 {
				continue
			}
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

// MergeShortEntries merges very short adjacent SRT entries into sentence-level
// chunks for better translation and TTS quality. YouTube word-level timing can
// produce hundreds of ~10ms cues that should be grouped into natural sentences.
func MergeShortEntries(entries []SRTEntry) []SRTEntry {
	if len(entries) == 0 {
		return entries
	}

	type merged struct {
		startTimeCode string
		endTimeCode   string
		texts         []string
		startSec      float64
		endSec        float64
	}

	firstStart, firstEnd := splitTimeCode(entries[0].TimeCode)
	var groups []merged
	current := merged{startTimeCode: firstStart, endTimeCode: firstEnd, texts: []string{entries[0].Text}}
	current.startSec = parseStartTime(entries[0].TimeCode)
	current.endSec = parseEndTime(entries[0].TimeCode)

	for i := 1; i < len(entries); i++ {
		entry := entries[i]
		start := parseStartTime(entry.TimeCode)
		end := parseEndTime(entry.TimeCode)
		duration := end - start
		accDuration := end - current.startSec
		lastText := current.texts[len(current.texts)-1]
		hasEndPunct := strings.HasSuffix(strings.TrimSpace(lastText), ".") ||
			strings.HasSuffix(strings.TrimSpace(lastText), "!") ||
			strings.HasSuffix(strings.TrimSpace(lastText), "?") ||
			strings.HasSuffix(strings.TrimSpace(lastText), "\"")

		// Merge if: very short cue, no punctuation yet, or accumulated duration < 2s
		shouldMerge := duration < 1.0 || (!hasEndPunct && accDuration < 8.0) || accDuration < 2.0

		if shouldMerge {
			current.texts = append(current.texts, entry.Text)
			current.endTimeCode = strings.Split(entry.TimeCode, " --> ")[1]
			current.endSec = end
		} else {
			groups = append(groups, current)
			entryStart, entryEnd := splitTimeCode(entry.TimeCode)
			current = merged{
				startTimeCode: entryStart,
				endTimeCode:   entryEnd,
				texts:         []string{entry.Text},
				startSec:      start,
				endSec:        end,
			}
		}
	}
	groups = append(groups, current)

	// Build result
	result := make([]SRTEntry, 0, len(groups))
	for i, g := range groups {
		result = append(result, SRTEntry{
			Index:    i + 1,
			TimeCode: g.startTimeCode + " --> " + g.endTimeCode,
			Text:     strings.Join(g.texts, " "),
		})
	}
	return result
}

func splitTimeCode(tc string) (string, string) {
	parts := strings.Split(tc, " --> ")
	if len(parts) != 2 {
		return strings.TrimSpace(tc), strings.TrimSpace(tc)
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
}

func parseStartTime(tc string) float64 {
	parts := strings.Split(tc, " --> ")
	if len(parts) != 2 {
		return 0
	}
	return ParseSRTTime(strings.TrimSpace(parts[0]))
}

func parseEndTime(tc string) float64 {
	parts := strings.Split(tc, " --> ")
	if len(parts) != 2 {
		return 0
	}
	return ParseSRTTime(strings.TrimSpace(parts[1]))
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
		"zh-Hans": "中文",
		"zh-CN":   "中文",
		"zh":      "中文",
		"ja":      "日文",
		"ko":      "韩文",
		"es":      "西班牙文",
		"fr":      "法文",
		"de":      "德文",
		"ru":      "俄文",
		"ar":      "阿拉伯文",
		"pt":      "葡萄牙文",
		"it":      "意大利文",
		"auto":    "自动检测",
	}
	if name, ok := names[code]; ok {
		return name
	}
	return code
}

func sameLanguage(source, target string) bool {
	canonical := func(value string) string {
		value = strings.ToLower(strings.TrimSpace(value))
		if parts := strings.FieldsFunc(value, func(r rune) bool { return r == '-' || r == '_' }); len(parts) > 0 {
			return parts[0]
		}
		return value
	}
	source, target = canonical(source), canonical(target)
	return source != "" && source == target
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
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
		BatchSize:   25,
		MaxWorkers:  3,
		RetryCount:  2,
		ContextSize: 2,
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
	base = trimSubtitleProcessingSuffix(base)
	return base + suffix + ".srt"
}

func trimSubtitleProcessingSuffix(base string) string {
	if strings.EqualFold(filepath.Ext(base), ".cleaned") {
		base = strings.TrimSuffix(base, filepath.Ext(base))
	}
	language := strings.TrimPrefix(filepath.Ext(base), ".")
	if isLanguageTag(language) {
		base = strings.TrimSuffix(base, filepath.Ext(base))
	}
	return base
}

func isLanguageTag(value string) bool {
	_, ok := map[string]struct{}{
		"ar": {}, "de": {}, "en": {}, "es": {}, "fr": {}, "it": {},
		"ja": {}, "ko": {}, "pt": {}, "ru": {}, "zh": {}, "zh-cn": {},
		"zh-hans": {}, "zh-hant": {},
	}[strings.ToLower(strings.TrimSpace(value))]
	return ok
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
