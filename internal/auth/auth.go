package auth

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

// Default B站 API credentials (moved from hardcoded to package-level variables
// that can be overridden via env vars or config. Users can pass custom values
// through the exported setter or environment variables.
var (
	appKeyTV = "4409e2ce8ffd12b8"
	appSecTV = "59b43e04ad6965f34319062b478f83dd"
	appKey   = "783bbb7264451d82"
	appSec   = "2653583c8873dea268ab9386918b1d65"
)

func init() {
	appKeyTV = os.Getenv("BILI_APP_KEY_TV")
	if appKeyTV == "" {
		appKeyTV = "4409e2ce8ffd12b8"
	}
	appSecTV = os.Getenv("BILI_APP_SEC_TV")
	if appSecTV == "" {
		appSecTV = "59b43e04ad6965f34319062b478f83dd"
	}
	appKey = os.Getenv("BILI_APP_KEY")
	if appKey == "" {
		appKey = "783bbb7264451d82"
	}
	appSec = os.Getenv("BILI_APP_SEC")
	if appSec == "" {
		appSec = "2653583c8873dea268ab9386918b1d65"
	}
}

type LoginInfo struct {
	Cookies  map[string]string `json:"cookies"`
	BiliJCT  string            `json:"bili_jct"`
	TokenInfo TokenInfo         `json:"token_info"`
}

type TokenInfo struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	Mid          int64  `json:"mid"`
	Uname        string `json:"uname"`
}

type QRCodeData struct {
	URL      string `json:"url"`
	AuthCode string `json:"auth_code"`
}

func sign(params map[string]string, sec string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var pairs []string
	for _, k := range keys {
		pairs = append(pairs, k+"="+params[k])
	}
	raw := strings.Join(pairs, "&") + sec
	h := md5.Sum([]byte(raw))
	return hex.EncodeToString(h[:])
}

func httpClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}

func apiGet(urlStr string) (map[string]interface{}, error) {
	resp, err := httpClient().Get(urlStr)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("apiGet: reading body failed: %w", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("apiGet: json decode failed: %w", err)
	}
	return result, nil
}

func apiPost(urlStr string, data url.Values) (map[string]interface{}, error) {
	resp, err := httpClient().PostForm(urlStr, data)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("apiPost: reading body failed: %w", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("apiPost: json decode failed: %w", err)
	}
	return result, nil
}

func GetQRCode() (*QRCodeData, error) {
	ts := fmt.Sprintf("%d", time.Now().Unix())
	params := map[string]string{
		"appkey":  appKeyTV,
		"local_id": "0",
		"ts":      ts,
	}
	params["sign"] = sign(params, appSecTV)

	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}

	result, err := apiPost("https://passport.bilibili.com/x/passport-tv-login/qrcode/auth_code", form)
	if err != nil {
		return nil, fmt.Errorf("请求二维码失败: %w", err)
	}
	code, _ := result["code"].(float64)
	if code != 0 {
		msg, _ := result["message"].(string)
		return nil, fmt.Errorf("获取二维码失败: %s", msg)
	}
	data, _ := result["data"].(map[string]interface{})
	return &QRCodeData{
		URL:      data["url"].(string),
		AuthCode: data["auth_code"].(string),
	}, nil
}

func PollQRCode(authCode string, timeout time.Duration) (*LoginInfo, error) {
	return PollQRCodeContext(context.Background(), authCode, timeout)
}

// PollQRCodeContext 轮询二维码扫描结果，支持 context 取消
func PollQRCodeContext(ctx context.Context, authCode string, timeout time.Duration) (*LoginInfo, error) {
	deadline := time.Now().Add(timeout)
	lastTick := time.Now()
	start := time.Now()

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("扫码轮询被取消: %w", ctx.Err())
		default:
		}

		// 每 5 秒输出一次进度，让用户知道轮询还在进行
		if time.Since(lastTick) >= 5*time.Second {
			elapsed := int(time.Since(start).Seconds())
			fmt.Fprintf(os.Stderr, "   ⏳ 等待扫码... %ds/%ds\n", elapsed, int(timeout.Seconds()))
			lastTick = time.Now()
		}

		time.Sleep(time.Second)

		ts := fmt.Sprintf("%d", time.Now().Unix())
		params := map[string]string{
			"appkey":    appKeyTV,
			"auth_code": authCode,
			"local_id":  "0",
			"ts":        ts,
		}
		params["sign"] = sign(params, appSecTV)

		form := url.Values{}
		for k, v := range params {
			form.Set(k, v)
		}

		result, err := apiPost("https://passport.bilibili.com/x/passport-tv-login/qrcode/poll", form)
		if err != nil {
			continue
		}
		code, _ := result["code"].(float64)

		switch int(code) {
		case 0:
			fmt.Fprintf(os.Stderr, "   ✅ 扫码已确认，正在获取登录信息...\n")
			return extractLoginInfo(result)
		case 86038:
			return nil, fmt.Errorf("二维码已过期")
		case -3:
			return nil, fmt.Errorf("API错误")
		}
	}
	return nil, fmt.Errorf("扫码超时（%ds）", int(timeout.Seconds()))
}

func extractLoginInfo(result map[string]interface{}) (*LoginInfo, error) {
	data, _ := result["data"].(map[string]interface{})
	if data == nil {
		return nil, fmt.Errorf("登录响应缺少 data")
	}
	cookieInfo, _ := data["cookie_info"].(map[string]interface{})
	if cookieInfo == nil {
		return nil, fmt.Errorf("登录响应缺少 cookie_info")
	}
	cookies := make(map[string]string)
	biliJCT := ""
	if raw, ok := cookieInfo["cookies"].([]interface{}); ok {
		for _, c := range raw {
			if cm, ok := c.(map[string]interface{}); ok {
				name, _ := cm["name"].(string)
				rawVal, _ := cm["value"].(string)
				// URL 解码 cookie 值（B站 API 返回的是 URL 编码的）
				val, err := url.QueryUnescape(rawVal)
				if err != nil {
					val = rawVal
				}
				cookies[name] = val
				if name == "bili_jct" {
					biliJCT = val
				}
			}
		}
	}

	tokenInfo := TokenInfo{}
	if ti, ok := data["token_info"].(map[string]interface{}); ok {
		tokenInfo.AccessToken, _ = ti["access_token"].(string)
		tokenInfo.RefreshToken, _ = ti["refresh_token"].(string)
		if mid, ok := ti["mid"].(float64); ok {
			tokenInfo.Mid = int64(mid)
		}
		tokenInfo.Uname, _ = ti["uname"].(string)
	}

	return &LoginInfo{
		Cookies:  cookies,
		BiliJCT:  biliJCT,
		TokenInfo: tokenInfo,
	}, nil
}

func ValidateLogin(cred *LoginInfo) (bool, error) {
	req, err := http.NewRequest("GET", "https://api.bilibili.com/x/web-interface/nav", nil)
	if err != nil {
		return false, fmt.Errorf("ValidateLogin: create request failed: %w", err)
	}
	for name, val := range cred.Cookies {
		req.AddCookie(&http.Cookie{Name: name, Value: val})
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Referer", "https://www.bilibili.com/")

	resp, err := httpClient().Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("ValidateLogin: reading body failed: %w", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return false, fmt.Errorf("ValidateLogin: json decode failed: %w", err)
	}
	code, _ := result["code"].(float64)
	return code == 0, nil
}

func GetUserInfo(cred *LoginInfo) (map[string]interface{}, error) {
	req, err := http.NewRequest("GET", "https://api.bilibili.com/x/space/myinfo", nil)
	if err != nil {
		return nil, fmt.Errorf("GetUserInfo: create request failed: %w", err)
	}
	for name, val := range cred.Cookies {
		req.AddCookie(&http.Cookie{Name: name, Value: val})
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Referer", "https://www.bilibili.com/")

	resp, err := httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("GetUserInfo: reading body failed: %w", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("GetUserInfo: json decode failed: %w", err)
	}
	code, _ := result["code"].(float64)
	if code != 0 {
		return nil, fmt.Errorf("获取用户信息失败")
	}
	data, _ := result["data"].(map[string]interface{})
	return data, nil
}
