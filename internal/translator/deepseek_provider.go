package translator

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DeepSeekConfig LLM 翻译配置。
type DeepSeekConfig struct {
	APIKey      string
	BaseURL     string
	Model       string
	BatchSize   int
	ContextSize int
}

// DeepSeekProvider 基于 LLM 的批量翻译（JSON 结构化输出）。
type DeepSeekProvider struct {
	apiKey  string
	baseURL string
	model   string
	batch   int
	context int
	client  *http.Client
}

// NewDeepSeekProvider 创建 DeepSeek Provider。
func NewDeepSeekProvider(cfg DeepSeekConfig) *DeepSeekProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.deepseek.com"
	}
	if cfg.Model == "" {
		cfg.Model = "deepseek-v4-flash"
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 15
	}
	if cfg.ContextSize < 0 {
		cfg.ContextSize = 2
	}
	return &DeepSeekProvider{
		apiKey:  cfg.APIKey,
		baseURL: cfg.BaseURL,
		model:   cfg.Model,
		batch:   cfg.BatchSize,
		context: cfg.ContextSize,
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

func (d *DeepSeekProvider) Name() string { return "deepseek" }

// TranslateBatch LLM 批量翻译（JSON 输出，严格校验数量/索引）。
func (d *DeepSeekProvider) TranslateBatch(ctx context.Context, texts []string, sourceLang, targetLang string) ([]string, error) {
	if len(texts) == 0 {
		return []string{}, nil
	}

	systemPrompt := fmt.Sprintf(`你是一个专业的视频字幕翻译专家。我将给你一段连续的%s字幕，其中包含 %d 句需要翻译的内容。

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
		getLangName(sourceLang),
		len(texts),
		getLangName(targetLang),
		len(texts),
		len(texts))

	targets := make([]map[string]interface{}, len(texts))
	for i, text := range texts {
		targets[i] = map[string]interface{}{"index": i + 1, "text": text}
	}
	inputPayload := map[string]interface{}{
		"previous_context": []string{},
		"target_subtitles": targets,
		"next_context":     []string{},
	}
	combinedJSON, err := json.Marshal(inputPayload)
	if err != nil {
		return nil, fmt.Errorf("构建翻译批次失败: %w", err)
	}

	response, err := d.callLLM(ctx, systemPrompt, string(combinedJSON))
	if err != nil {
		return nil, err
	}

	return parseTranslations(response, len(texts))
}

func (d *DeepSeekProvider) callLLM(ctx context.Context, systemPrompt, userContent string) (string, error) {
	messages := []map[string]string{
		{"role": "system", "content": systemPrompt},
		{"role": "user", "content": userContent},
	}

	payload := map[string]interface{}{
		"model":       d.model,
		"messages":    messages,
		"temperature": 0.3,
		"max_tokens":  8192,
	}
	// 字幕翻译不需要链式思考：deepseek-v4-flash 等推理模型会把 completion 预算烧在内部
	// reasoning 上，触发 finish=length 后 content 为空/截断 → “期望 N 条实际 1 条”。
	// 对 DeepSeek 官方端点显式关闭 thinking；其他 OpenAI 兼容端点不传该参数（避免被拒）。
	if targetsDeepSeek(d.baseURL) {
		payload["thinking"] = map[string]interface{}{"type": "disabled"}
	}
	payloadBytes, _ := json.Marshal(payload)

	req, _ := http.NewRequestWithContext(ctx, "POST", d.baseURL+"/chat/completions", strings.NewReader(string(payloadBytes)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+d.apiKey)

	resp, err := d.client.Do(req)
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
		return "", fmt.Errorf("解析 LLM 响应失败: %s", truncateStr(string(body), 200))
	}

	if result.Error != nil {
		return "", fmt.Errorf("LLM 错误: %s", result.Error.Message)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("LLM 返回空 choices: %s", truncateStr(string(body), 200))
	}

	return result.Choices[0].Message.Content, nil
}

// targetsDeepSeek 判断 baseURL 是否指向 DeepSeek 官方 API（api.deepseek.com）。
// DeepSeek 的推理模型支持用 thinking:{"type":"disabled"} 关闭链式思考（避免把
// completion 预算烧在 reasoning 上导致 content 为空）；其他 OpenAI 兼容端点
// （vllm/ollama/自定义网关等）不保证接受该参数，仅 DeepSeek 才附带。
func targetsDeepSeek(baseURL string) bool {
	h := strings.ToLower(strings.TrimSpace(baseURL))
	return strings.Contains(h, "deepseek.com")
}
