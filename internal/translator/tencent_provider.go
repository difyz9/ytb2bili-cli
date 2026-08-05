package translator

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// TencentConfig 腾讯云翻译配置。
type TencentConfig struct {
	SecretID  string
	SecretKey string
	Region    string
	Endpoint  string
}

// TencentProvider 腾讯云机器翻译（TMT）TextTranslate。
// 文档: https://cloud.tencent.com/document/product/551/15619
// 认证: TC3-HMAC-SHA256 签名（手写实现，不依赖 SDK 版本裁剪）
type TencentProvider struct {
	secretID  string
	secretKey string
	region    string
	endpoint  string
	client    *http.Client
}

// NewTencentProvider 创建腾讯翻译 Provider。
func NewTencentProvider(cfg TencentConfig) *TencentProvider {
	if cfg.Region == "" {
		cfg.Region = "ap-guangzhou"
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = "tmt.tencentcloudapi.com"
	}
	return &TencentProvider{
		secretID:  cfg.SecretID,
		secretKey: cfg.SecretKey,
		region:    cfg.Region,
		endpoint:  cfg.Endpoint,
		client:    &http.Client{Timeout: 15 * time.Second},
	}
}

func (t *TencentProvider) Name() string { return "tencent" }

// tencentLang 腾讯语言代码（zh-Hans → zh）。
func tencentLang(lang string) string {
	switch lang {
	case "zh-Hans", "zh-CN", "zh":
		return "zh"
	case "en":
		return "en"
	default:
		return lang
	}
}

// TranslateBatch 批量翻译（BatchAdapter 负责并发与重组）。
func (t *TencentProvider) TranslateBatch(ctx context.Context, texts []string, sourceLang, targetLang string) ([]string, error) {
	adapter := NewBatchAdapter(t.Name(), t.single, 5)
	return adapter.TranslateBatch(ctx, texts, sourceLang, targetLang)
}

func (t *TencentProvider) single(ctx context.Context, text, sourceLang, targetLang string) (string, error) {
	// TextTranslate 请求参数
	payload := map[string]interface{}{
		"SourceText": text,
		"Source":     tencentLang(sourceLang),
		"Target":     tencentLang(targetLang),
		"ProjectId":  0,
	}
	body, err := t.signedRequest(ctx, "TextTranslate", payload)
	if err != nil {
		return "", err
	}

	var result struct {
		Response struct {
			TargetText string `json:"TargetText"`
			Error      *struct {
				Code    string `json:"Code"`
				Message string `json:"Message"`
			} `json:"Error"`
			RequestId string `json:"RequestId"`
		} `json:"Response"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("解析腾讯翻译响应失败: %s", truncateStr(string(body), 200))
	}
	if result.Response.Error != nil {
		return "", fmt.Errorf("腾讯翻译错误 %s: %s", result.Response.Error.Code, result.Response.Error.Message)
	}
	if result.Response.TargetText == "" {
		return "", fmt.Errorf("腾讯翻译返回空结果: %s", truncateStr(string(body), 200))
	}
	return result.Response.TargetText, nil
}

// signedRequest 腾讯云 TC3-HMAC-SHA256 签名请求。
// 参考: https://cloud.tencent.com/document/product/551/30347（通用签名 v3）
func (t *TencentProvider) signedRequest(ctx context.Context, action string, payload map[string]interface{}) ([]byte, error) {
	bodyBytes, _ := json.Marshal(payload)

	// 1. CanonicalRequest
	timestamp := time.Now().Unix()
	date := time.Unix(timestamp, 0).UTC().Format("2006-01-02")
	service := "tmt"
	host := t.endpoint

	canonicalHeaders := "content-type:application/json; charset=utf-8\nhost:" + host + "\nx-tc-action:" + strings.ToLower(action) + "\n"
	signedHeaders := "content-type;host;x-tc-action"
	hashedPayload := sha256Hex(bodyBytes)
	canonicalRequest := strings.Join([]string{
		"POST",
		"/",
		"",
		canonicalHeaders,
		signedHeaders,
		hashedPayload,
	}, "\n")

	// 2. StringToSign
	credentialScope := strings.Join([]string{date, service, "tc3_request"}, "/")
	stringToSign := strings.Join([]string{
		"TC3-HMAC-SHA256",
		fmt.Sprintf("%d", timestamp),
		credentialScope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")

	// 3. Signature
	secretDate := hmacSHA256([]byte("TC3"+t.secretKey), date)
	secretService := hmacSHA256(secretDate, service)
	secretSigning := hmacSHA256(secretService, "tc3_request")
	signature := hex.EncodeToString(hmacSHA256(secretSigning, stringToSign))

	// 4. Authorization
	authorization := fmt.Sprintf("TC3-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		t.secretID, credentialScope, signedHeaders, signature)

	req, err := http.NewRequestWithContext(ctx, "POST", "https://"+host+"/", strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("构建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Host", host)
	req.Header.Set("X-TC-Action", action)
	req.Header.Set("X-TC-Timestamp", fmt.Sprintf("%d", timestamp))
	req.Header.Set("X-TC-Version", "2018-03-21")
	req.Header.Set("X-TC-Region", t.region)
	req.Header.Set("Authorization", authorization)

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("腾讯翻译 HTTP %d: %s", resp.StatusCode, truncateStr(string(body), 300))
	}
	return body, nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}
