package translator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OllamaConfig 本地 Ollama 配置。
type OllamaConfig struct {
	BaseURL string
	Model   string
	Batch   int
}

// OllamaProvider 本地 Ollama LLM 翻译（零成本、无网络依赖）。
// 端点: POST {BaseURL}/api/chat（本地默认 http://localhost:11434）
// 说明: translation-tool 的 ollama.go 误用云端 API，此处实现正确的本地 /api/chat 调用。
type OllamaProvider struct {
	baseURL string
	model   string
	batch   int
	client  *http.Client
}

// NewOllamaProvider 创建 Ollama Provider。
func NewOllamaProvider(cfg OllamaConfig) *OllamaProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://localhost:11434"
	}
	if cfg.Model == "" {
		cfg.Model = "qwen2.5:7b"
	}
	if cfg.Batch <= 0 {
		cfg.Batch = 10
	}
	return &OllamaProvider{
		baseURL: cfg.BaseURL,
		model:   cfg.Model,
		batch:   cfg.Batch,
		client:  &http.Client{Timeout: 300 * time.Second},
	}
}

func (o *OllamaProvider) Name() string { return "ollama" }

// TranslateBatch 批量翻译（JSON 结构化输出，与 DeepSeek 相同的严格校验）。
// 本地模型能力有限：batch 控制在 10 条内，避免长 JSON 截断。
func (o *OllamaProvider) TranslateBatch(ctx context.Context, texts []string, sourceLang, targetLang string) ([]string, error) {
	if len(texts) == 0 {
		return []string{}, nil
	}
	// 本地模型单次处理能力有限，分批内部串行
	const chunkSize = 10
	var all []string
	for start := 0; start < len(texts); start += chunkSize {
		end := min(start+chunkSize, len(texts))
		chunk := texts[start:end]
		out, err := o.translateChunk(ctx, chunk, sourceLang, targetLang)
		if err != nil {
			return nil, fmt.Errorf("第 %d-%d 条: %w", start+1, end, err)
		}
		all = append(all, out...)
	}
	return all, nil
}

func (o *OllamaProvider) translateChunk(ctx context.Context, texts []string, sourceLang, targetLang string) ([]string, error) {
	systemPrompt := fmt.Sprintf(`你是一个专业的视频字幕翻译专家。我将给你一段连续的%s字幕，其中包含 %d 句需要翻译的内容。

翻译要求：
1. 自然流畅：使用口语化表达，符合%s字幕习惯
2. 准确传神：忠实原文含义，保持语气和情感
3. 简洁明了：字幕需要快速阅读，避免冗长
4. 数量严格：必须输出 %d 句翻译，不多不少
5. 一一对应：即使相邻字幕内容重复，也必须保留并分别翻译，不得合并、去重或省略
6. 索引严格：输出中 index 必须从 1 连续到 %d

输入格式：JSON 对象；target_subtitles 才需要翻译
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
	payloadIn := map[string]interface{}{
		"target_subtitles": targets,
	}
	userContent, _ := json.Marshal(payloadIn)

	// Ollama /api/chat
	reqBody := map[string]interface{}{
		"model":    o.model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": string(userContent)},
		},
		"stream": false,
		"options": map[string]interface{}{
			"temperature": 0.2,
			"num_predict": 4096,
		},
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, "POST", o.baseURL+"/api/chat", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("构建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Ollama 请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Ollama HTTP %d: %s", resp.StatusCode, truncateStr(string(body), 300))
	}

	var result struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析 Ollama 响应失败: %s", truncateStr(string(body), 200))
	}
	if result.Error != "" {
		return nil, fmt.Errorf("Ollama 错误: %s", result.Error)
	}
	if result.Message.Content == "" {
		return nil, fmt.Errorf("Ollama 返回空内容: %s", truncateStr(string(body), 200))
	}

	return parseTranslations(result.Message.Content, len(texts))
}
