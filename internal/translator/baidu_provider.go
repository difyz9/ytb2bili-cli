package translator

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// BaiduConfig 百度翻译配置。
type BaiduConfig struct {
	AppID     string
	AppKey    string
	Endpoint  string
	QPS       int
}

// BaiduProvider 百度翻译（通用翻译 API）。
// 文档: https://api.fanyi.baidu.com/doc/21
// 签名: MD5(appid + q + salt + key)，q 需 UTF-8。
type BaiduProvider struct {
	appID    string
	appKey   string
	endpoint string
	qps      int
	client   *http.Client
}

// NewBaiduProvider 创建百度翻译 Provider。
func NewBaiduProvider(cfg BaiduConfig) *BaiduProvider {
	if cfg.Endpoint == "" {
		cfg.Endpoint = "https://api.fanyi.baidu.com/api/trans/vip/translate"
	}
	if cfg.QPS <= 0 {
		cfg.QPS = 5
	}
	return &BaiduProvider{
		appID:    cfg.AppID,
		appKey:   cfg.AppKey,
		endpoint: cfg.Endpoint,
		qps:      cfg.QPS,
		client:   &http.Client{Timeout: 15 * time.Second},
	}
}

func (b *BaiduProvider) Name() string { return "baidu" }

// TranslateBatch 批量翻译（BatchAdapter 负责并发与重组）。
func (b *BaiduProvider) TranslateBatch(ctx context.Context, texts []string, sourceLang, targetLang string) ([]string, error) {
	adapter := NewBatchAdapter(b.Name(), b.single, b.qps)
	return adapter.TranslateBatch(ctx, texts, sourceLang, targetLang)
}

// baiduLang 百度语言代码（zh-Hans → zh，zh 保持）。
func baiduLang(lang string) string {
	switch lang {
	case "zh-Hans", "zh-CN", "zh":
		return "zh"
	case "en":
		return "en"
	default:
		return lang
	}
}

func (b *BaiduProvider) single(ctx context.Context, text, sourceLang, targetLang string) (string, error) {
	salt := fmt.Sprintf("%d", time.Now().UnixNano())
	signRaw := b.appID + text + salt + b.appKey
	sum := md5.Sum([]byte(signRaw))
	sign := hex.EncodeToString(sum[:])

	params := url.Values{}
	params.Set("q", text)
	params.Set("from", baiduLang(sourceLang))
	params.Set("to", baiduLang(targetLang))
	params.Set("appid", b.appID)
	params.Set("salt", salt)
	params.Set("sign", sign)

	req, err := http.NewRequestWithContext(ctx, "GET", b.endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return "", fmt.Errorf("构建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := b.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		From        string `json:"from"`
		To          string `json:"to"`
		TransResult []struct {
			Src string `json:"src"`
			Dst string `json:"dst"`
		} `json:"trans_result"`
		ErrorCode string `json:"error_code"`
		ErrorMsg  string `json:"error_msg"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("解析响应失败: %s", truncateStr(string(body), 200))
	}
	if result.ErrorCode != "" && result.ErrorCode != "0" {
		return "", fmt.Errorf("百度翻译错误 %s: %s", result.ErrorCode, result.ErrorMsg)
	}
	if len(result.TransResult) == 0 {
		return "", fmt.Errorf("无翻译结果: %s", truncateStr(string(body), 200))
	}
	return result.TransResult[0].Dst, nil
}
