package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client interface {
	Complete(context.Context, string) (string, error)
}

type OpenAIClient struct {
	APIKey, BaseURL, Model string
	HTTPClient             *http.Client
}

func (c *OpenAIClient) Complete(ctx context.Context, prompt string) (string, error) {
	if c.BaseURL == "" || c.Model == "" {
		return "", fmt.Errorf("llm: base URL and model are required")
	}
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	payload, err := json.Marshal(map[string]any{
		"model": c.Model, "messages": []map[string]string{{"role": "user", "content": prompt}},
		"temperature": 0.7, "max_tokens": 2048,
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+"/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
	}
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
		return "", fmt.Errorf("解析 LLM 响应失败: %s", truncate(body, 200))
	}
	if result.Error != nil {
		return "", fmt.Errorf("LLM 错误: %s", result.Error.Message)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("LLM HTTP %d: %s", resp.StatusCode, truncate(body, 200))
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("LLM 返回空结果")
	}
	return result.Choices[0].Message.Content, nil
}

func truncate(value []byte, max int) string {
	if len(value) > max {
		value = value[:max]
	}
	return string(value)
}
