// Package ytoauth 提供 YouTube OAuth 授权登录与订阅频道获取。
//
// 使用 Google OAuth2 设备码流程（Device Flow），适合 CLI 场景：
//  1. login: 获取设备码 + 用户码，打印授权 URL，用户在浏览器授权
//  2. 轮询 token 端点，授权完成后获取 access_token / refresh_token
//  3. sync: 用 token 调用 YouTube Data API v3 subscriptions.list
//     拉取用户关注的频道列表，写入本地订阅存储
package ytoauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// ─── 常量 ────────────────────────────────────────────────────────────────

const (
	// deviceCodeEndpoint Google OAuth2 设备码端点
	deviceCodeEndpoint = "https://oauth2.googleapis.com/device/code"
	// tokenEndpoint Google OAuth2 token 端点
	tokenEndpoint = "https://oauth2.googleapis.com/token"
	// youtubeReadonlyScope 只读访问 YouTube 数据（订阅列表）
	youtubeReadonlyScope = "https://www.googleapis.com/auth/youtube.readonly"
)

// ─── 配置 ────────────────────────────────────────────────────────────────

// Config OAuth 客户端配置
type Config struct {
	// ClientID Google OAuth 客户端 ID（必需）
	ClientID string
	// ClientSecret Google OAuth 客户端密钥（必需）
	ClientSecret string
	// Scopes 授权的权限范围（默认 youtube.readonly）
	Scopes []string
}

// DefaultScopes 返回默认权限范围
func DefaultScopes() []string { return []string{youtubeReadonlyScope} }

// Validate 检查配置是否有效
func (c Config) Validate() error {
	if strings.TrimSpace(c.ClientID) == "" {
		return errors.New("yt-oauth: client_id 未配置（config.yaml → youtube_oauth.client_id）")
	}
	if strings.TrimSpace(c.ClientSecret) == "" {
		return errors.New("yt-oauth: client_secret 未配置（config.yaml → youtube_oauth.client_secret）")
	}
	return nil
}

// ─── Token 存储 ──────────────────────────────────────────────────────────

// TokenStore 将 OAuth token 持久化到本地文件
type TokenStore struct {
	path string
}

// NewTokenStore 创建 token 存储（data/yt_oauth_token.json）
func NewTokenStore(dataDir string) *TokenStore {
	return &TokenStore{path: filepath.Join(dataDir, "yt_oauth_token.json")}
}

// Path 返回 token 文件路径
func (s *TokenStore) Path() string { return s.path }

// Save 保存 token 到本地文件
func (s *TokenStore) Save(tok *oauth2.Token) error {
	if tok == nil {
		return errors.New("yt-oauth: 无法保存空 token")
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}
	data, err := json.MarshalIndent(tok, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 token 失败: %w", err)
	}
	return os.WriteFile(s.path, data, 0600)
}

// Load 从本地文件加载 token
func (s *TokenStore) Load() (*oauth2.Token, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.New("yt-oauth: 未登录（token 不存在），请先运行: ytb yt-oauth login")
		}
		return nil, err
	}
	var tok oauth2.Token
	if err := json.Unmarshal(data, &tok); err != nil {
		return nil, fmt.Errorf("解析 token 失败: %w", err)
	}
	return &tok, nil
}

// Clear 删除 token 文件（登出）
func (s *TokenStore) Clear() error {
	if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Exists 报告是否已登录
func (s *TokenStore) Exists() bool {
	_, err := os.Stat(s.path)
	return err == nil
}

// ─── 设备码流程 ──────────────────────────────────────────────────────────

// DeviceCodeResponse 设备码端点响应
type DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURL string `json:"verification_url"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// StartDeviceFlow 发起设备码授权流程，返回设备码信息
func StartDeviceFlow(ctx context.Context, cfg Config) (*DeviceCodeResponse, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	scopes := cfg.Scopes
	if len(scopes) == 0 {
		scopes = DefaultScopes()
	}

	form := url.Values{}
	form.Set("client_id", cfg.ClientID)
	form.Set("scope", strings.Join(scopes, " "))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, deviceCodeEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求设备码失败: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result DeviceCodeResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析设备码响应失败: %w", err)
	}
	if result.DeviceCode == "" {
		// Google 设备码端点只允许 "TV and Limited Input devices" 类型客户端，
		// 其他类型会返回 error 响应——把真实原因浮出来而不是笼统报"响应为空"。
		var apiErr struct {
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
		}
		if json.Unmarshal(body, &apiErr) == nil && apiErr.Error != "" {
			if apiErr.ErrorDescription != "" {
				return nil, fmt.Errorf("设备码请求被拒绝 [%s]: %s", apiErr.Error, apiErr.ErrorDescription)
			}
			return nil, fmt.Errorf("设备码请求被拒绝: %s", apiErr.Error)
		}
		return nil, errors.New("设备码响应为空（请检查 client_id 是否正确）")
	}
	return &result, nil
}

// PollToken 轮询 token 端点直到用户完成授权。
// 返回 token，或用户取消/超时的错误。
func PollToken(ctx context.Context, cfg Config, device *DeviceCodeResponse) (*oauth2.Token, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	interval := device.Interval
	if interval <= 0 {
		interval = 5
	}
	expiresAt := time.Now().Add(time.Duration(device.ExpiresIn) * time.Second)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		if time.Now().After(expiresAt) {
			return nil, errors.New("设备码已过期，请重新运行 login")
		}

		form := url.Values{}
		form.Set("client_id", cfg.ClientID)
		form.Set("client_secret", cfg.ClientSecret)
		form.Set("device_code", device.DeviceCode)
		form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(form.Encode()))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("轮询 token 失败: %w", err)
		}

		var body map[string]any
		json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			tok := &oauth2.Token{
				AccessToken:  stringVal(body["access_token"]),
				RefreshToken: stringVal(body["refresh_token"]),
				TokenType:    stringVal(body["token_type"]),
			}
			if exp, ok := body["expires_in"].(float64); ok {
				tok.Expiry = time.Now().Add(time.Duration(exp) * time.Second)
			}
			return tok, nil
		}

		// 处理授权进行中的状态
		errorCode := stringVal(body["error"])
		switch errorCode {
		case "authorization_pending":
			// 用户还没授权，继续等待
		case "slow_down":
			interval += 5
		case "access_denied":
			return nil, errors.New("用户拒绝了授权")
		case "expired_token":
			return nil, errors.New("设备码已过期，请重新运行 login")
		default:
			return nil, fmt.Errorf("授权失败: %s (%s)", errorCode, stringVal(body["error_description"]))
		}

		time.Sleep(time.Duration(interval) * time.Second)
	}
}

// stringVal 安全读取 map 中的字符串值
func stringVal(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// ─── Token 刷新与客户端 ──────────────────────────────────────────────────

// RefreshToken 使用 refresh_token 刷新访问令牌
func RefreshToken(ctx context.Context, cfg Config, tok *oauth2.Token) (*oauth2.Token, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if tok == nil || tok.RefreshToken == "" {
		return nil, errors.New("yt-oauth: 无 refresh_token，请重新登录")
	}

	form := url.Values{}
	form.Set("client_id", cfg.ClientID)
	form.Set("client_secret", cfg.ClientSecret)
	form.Set("refresh_token", tok.RefreshToken)
	form.Set("grant_type", "refresh_token")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("刷新 token 失败: %w", err)
	}
	defer resp.Body.Close()

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("刷新 token 失败: %s", stringVal(body["error_description"]))
	}

	newTok := &oauth2.Token{
		AccessToken:  stringVal(body["access_token"]),
		RefreshToken: tok.RefreshToken, // 保留原 refresh_token
		TokenType:    stringVal(body["token_type"]),
	}
	if exp, ok := body["expires_in"].(float64); ok {
		newTok.Expiry = time.Now().Add(time.Duration(exp) * time.Second)
	}
	return newTok, nil
}

// ValidToken 检查 token 是否有效，过期时自动刷新
func ValidToken(ctx context.Context, cfg Config, store *TokenStore) (*oauth2.Token, error) {
	tok, err := store.Load()
	if err != nil {
		return nil, err
	}
	if tok.Valid() {
		return tok, nil
	}
	// 过期，尝试刷新
	newTok, err := RefreshToken(ctx, cfg, tok)
	if err != nil {
		return nil, fmt.Errorf("token 已过期且刷新失败: %w", err)
	}
	if err := store.Save(newTok); err != nil {
		return nil, err
	}
	return newTok, nil
}
