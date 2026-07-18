package download

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// YouTubeCookie 从 Chrome 扩展接收的 cookie 结构
type YouTubeCookie struct {
	Name           string  `json:"name"`
	Value          string  `json:"value"`
	Domain         string  `json:"domain,omitempty"`
	Path           string  `json:"path,omitempty"`
	Secure         bool    `json:"secure,omitempty"`
	HttpOnly       bool    `json:"httpOnly,omitempty"`
	SameSite       string  `json:"sameSite,omitempty"`
	ExpirationDate float64 `json:"expirationDate,omitempty"`
	StoreID        string  `json:"storeId,omitempty"`
}

// 默认加密密钥，需与 extension/utils/config.ts 中的 COOKIES_ENCRYPT_KEY 保持一致
// 可通过 COOKIES_ENCRYPT_KEY 环境变量覆盖
const defaultCookiesEncryptKey = "59e7052041ce4bd6aff82f6a0bca9cde"

func getEncryptKey() string {
	if key := os.Getenv("COOKIES_ENCRYPT_KEY"); key != "" {
		return key
	}
	return defaultCookiesEncryptKey
}

// deriveKey 从密钥字符串派生出 AES-256 密钥（与 extension 的 crypto.ts 保持一致）
func deriveKey(keyStr string) []byte {
	if len(keyStr) >= 32 {
		return []byte(keyStr[:32])
	}
	// 补零到 32 字节
	padded := make([]byte, 32)
	copy(padded, keyStr)
	return padded
}

// DecryptCookiesMeta 解密从 Chrome 扩展接收的 meta 加密 cookies
// meta: AES-GCM 加密的 base64 字符串（前 12 字节为 IV，后续为密文）
// 返回解密后的 cookie JSON 字符串
func DecryptCookiesMeta(meta string) (string, error) {
	if meta == "" {
		return "", fmt.Errorf("empty meta")
	}

	// Base64 解码
	combined, err := base64.StdEncoding.DecodeString(meta)
	if err != nil {
		return "", fmt.Errorf("base64 decode failed: %w", err)
	}

	if len(combined) < 13 { // 至少需要 12 字节 IV + 1 字节数据
		return "", fmt.Errorf("encrypted data too short (%d bytes)", len(combined))
	}

	// 前 12 字节为 IV
	iv := combined[:12]
	ciphertext := combined[12:]

	// 派生密钥
	key := deriveKey(getEncryptKey())

	// AES-GCM 解密
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("aes cipher failed: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("gcm failed: %w", err)
	}

	plaintext, err := gcm.Open(nil, iv, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("aes-gcm decrypt failed: %w", err)
	}

	return string(plaintext), nil
}

// ParseCookiesJSON 解析 cookies JSON 字符串为 YouTubeCookie 数组
func ParseCookiesJSON(jsonStr string) ([]YouTubeCookie, error) {
	if jsonStr == "" {
		return nil, fmt.Errorf("empty cookies json")
	}

	// 尝试先解析为数组
	var cookies []YouTubeCookie
	if err := json.Unmarshal([]byte(jsonStr), &cookies); err == nil {
		return cookies, nil
	}

	// 如果是单个对象（兼容旧格式）
	var single YouTubeCookie
	if err := json.Unmarshal([]byte(jsonStr), &single); err == nil {
		return []YouTubeCookie{single}, nil
	}

	return nil, fmt.Errorf("failed to parse cookies json")
}

// CookiesToNetscape 将 cookies 数组转换为 Netscape 格式字符串
// 这是 yt-dlp 需要的 cookie 文件格式
func CookiesToNetscape(cookies []YouTubeCookie) string {
	var lines []string
	lines = append(lines, "# Netscape HTTP Cookie File")
	lines = append(lines, "# https://curl.haxx.se/rfc/cookie_spec.html")
	lines = append(lines, "# This is a generated file! Do not edit.")
	lines = append(lines, "")

	for _, c := range cookies {
		domain := c.Domain
		if domain == "" {
			domain = ".youtube.com"
		}
		includeSubDomain := "FALSE"
		if strings.HasPrefix(domain, ".") {
			includeSubDomain = "TRUE"
		}
		path := c.Path
		if path == "" {
			path = "/"
		}
		secureFlag := "FALSE"
		if c.Secure {
			secureFlag = "TRUE"
		}
		expiry := "0"
		if c.ExpirationDate > 0 {
			expiry = fmt.Sprintf("%.0f", c.ExpirationDate)
		}
		name := c.Name
		value := c.Value

		line := fmt.Sprintf("%s\t%s\t%s\t%s\t%s\t%s\t%s",
			domain, includeSubDomain, path, secureFlag, expiry, name, value)
		lines = append(lines, line)
	}
	lines = append(lines, "")

	return strings.Join(lines, "\n")
}

// SaveCookiesFromMeta 完整流程：解密 meta → 解析 JSON → 转为 Netscape 格式 → 保存到文件
// 返回保存的文件路径
func SaveCookiesFromMeta(meta string, outputDir string) (string, error) {
	if meta == "" {
		return "", fmt.Errorf("no cookies meta provided")
	}

	// 1. 解密
	jsonStr, err := DecryptCookiesMeta(meta)
	if err != nil {
		return "", fmt.Errorf("decrypt failed: %w", err)
	}

	// 2. 解析 JSON
	cookies, err := ParseCookiesJSON(jsonStr)
	if err != nil {
		return "", fmt.Errorf("parse failed: %w", err)
	}

	if len(cookies) == 0 {
		return "", fmt.Errorf("no cookies found")
	}

	// 3. 转为 Netscape 格式
	netscape := CookiesToNetscape(cookies)

	// 4. 保存到文件（统一保存到 data/cookies/ 目录）
	cookiesDir := filepath.Join(outputDir, "cookies")
	os.MkdirAll(cookiesDir, 0755)
	cookiesFile := filepath.Join(cookiesDir, "youtube_cookies_from_meta.txt")
	if err := os.WriteFile(cookiesFile, []byte(netscape), 0600); err != nil {
		return "", fmt.Errorf("write cookies file failed: %w", err)
	}

	return cookiesFile, nil
}
