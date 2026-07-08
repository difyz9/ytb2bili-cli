package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func main() {
	// 模拟从 Chrome 扩展获取的 cookies（包含多个域）
	cookiesJSON := `[
		{
			"domain": ".youtube.com",
			"expirationDate": 1807076018.955204,
			"hostOnly": false,
			"httpOnly": true,
			"name": "__Secure-1PSID",
			"path": "/",
			"sameSite": "no_restriction",
			"secure": true,
			"session": false,
			"storeId": "0",
			"value": "g.a000-wj3A0xf6UHL78eyDmKtaJMIb273WQ8mpFYYJWIkcQvUPTia9o7SB-HfTsv4_I37nmNBTwACgYKAbwSARISFQHGX2Mi8jnEmHlHeVy8BVxXu8HKrhoVAUF8yKo8L4PtusIUil76XdDHhoPg0076"
		},
		{
			"domain": ".youtube.com",
			"expirationDate": 1807076018.955204,
			"hostOnly": false,
			"httpOnly": true,
			"name": "__Secure-3PSID",
			"path": "/",
			"sameSite": "no_restriction",
			"secure": true,
			"session": false,
			"storeId": "0",
			"value": "g.a000-wj3A0xf6UHL78eyDmKtaJMIb273WQ8mpFYYJWIkcQvUPTiat0TcPhy5MrZPKgXl-thfSgACgYKAV0SARISFQHGX2MigtAMIe1BDFDUW7H55JUphhoVAUF8yKqlFLI6Ot4box0nQS4FLtX20076"
		},
		{
			"domain": ".google.com",
			"expirationDate": 1807076018.955204,
			"hostOnly": false,
			"httpOnly": true,
			"name": "SID",
			"path": "/",
			"sameSite": "lax",
			"secure": true,
			"session": false,
			"storeId": "0",
			"value": "AYUTj0_5Kf9V8Q9v7Z6bL4dF2gH1jK3lM5nO7pQ9rS1tU3vW5xY7zA9bC1dE3fG5h"
		},
		{
			"domain": ".google.com",
			"expirationDate": 1807076018.955204,
			"hostOnly": false,
			"httpOnly": true,
			"name": "HSID",
			"path": "/",
			"sameSite": "lax",
			"secure": false,
			"session": false,
			"storeId": "0",
			"value": "AYUTj0_5Kf9V8Q9v7Z6bL4dF2gH1jK3lM5nO7pQ9rS1tU3vW5xY7zA9bC1dE3fG5h"
		},
		{
			"domain": ".google.com",
			"expirationDate": 1807076018.955204,
			"hostOnly": false,
			"httpOnly": true,
			"name": "SSID",
			"path": "/",
			"sameSite": "lax",
			"secure": true,
			"session": false,
			"storeId": "0",
			"value": "AYUTj0_5Kf9V8Q9v7Z6bL4dF2gH1jK3lM5nO7pQ9rS1tU3vW5xY7zA9bC1dE3fG5h"
		},
		{
			"domain": ".google.com",
			"expirationDate": 1807076018.955204,
			"hostOnly": false,
			"httpOnly": true,
			"name": "LOGIN_INFO",
			"path": "/",
			"sameSite": "lax",
			"secure": true,
			"session": false,
			"storeId": "0",
			"value": "AFmmF2swRQIhALC5Z8e9f7d6c5b4a3a2a1a0a9a8a7a6a5a4a3a2a1a0a9a8a7a6"
		},
		{
			"domain": ".youtube.com",
			"expirationDate": 1807076018.955204,
			"hostOnly": false,
			"httpOnly": true,
			"name": "SAPISID",
			"path": "/",
			"sameSite": "none",
			"secure": true,
			"session": false,
			"storeId": "0",
			"value": "nVIWPFzNH2j2lBKe/AlSziICxQeVtYay1b"
		},
		{
			"domain": ".youtube.com",
			"expirationDate": 1807076018.955204,
			"hostOnly": false,
			"httpOnly": false,
			"name": "SSID",
			"path": "/",
			"sameSite": "none",
			"secure": true,
			"session": false,
			"storeId": "0",
			"value": "AKVhgiZC7x8__zO6V"
		}
	]`

	// 解析 cookies
	var cookies []struct {
		Name            string  `json:"name"`
		Value           string  `json:"value"`
		Domain          string  `json:"domain"`
		Path            string  `json:"path"`
		Expire          int64   `json:"expire"`
		ExpirationDate  float64 `json:"expirationDate"`
		HttpOnly        bool    `json:"httpOnly"`
		Secure          bool    `json:"secure"`
		SameSite        string  `json:"sameSite"`
		Session         bool    `json:"session"`
		StoreID         string  `json:"storeId"`
		HostOnly        bool    `json:"hostOnly"`
	}

	if err := json.Unmarshal([]byte(cookiesJSON), &cookies); err != nil {
		fmt.Printf("❌ 解析 cookies 失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("📊 解析到 %d 个 cookies\n\n", len(cookies))

	// 生成 Netscape 格式
	var sb strings.Builder
	sb.WriteString("# Netscape HTTP Cookie File\n")
	sb.WriteString("# https://curl.haxx.se/rfc/cookie_spec.html\n")
	sb.WriteString("# This is a generated file! Do not edit.\n\n")

	validCount := 0
	for _, c := range cookies {
		// 确定过期时间
		var expires int64
		if c.ExpirationDate > 0 {
			expires = int64(c.ExpirationDate)
		} else {
			expires = c.Expire
		}

		// 跳过 session cookies
		if expires == 0 {
			fmt.Printf("⏭️ 跳过 session cookie: %s\n", c.Name)
			continue
		}

		secure := "FALSE"
		if c.Secure {
			secure = "TRUE"
		}

		sb.WriteString(fmt.Sprintf("%s\tTRUE\t%s\t%s\t%d\t%s\t%s\n",
			c.Domain,
			c.Path,
			secure,
			expires,
			c.Name,
			c.Value,
		))
		validCount++
	}

	// 保存到文件
	outputPath := "/tmp/test_enhanced_cookies.txt"
	if err := os.WriteFile(outputPath, []byte(sb.String()), 0644); err != nil {
		fmt.Printf("❌ 保存文件失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n✅ 生成了 %d 个有效 cookies\n", validCount)
	fmt.Printf("📁 文件已保存到: %s\n\n", outputPath)

	// 显示生成的文件内容
	fmt.Println("📄 文件内容:")
	fmt.Println(strings.Repeat("=", 60))
	content, _ := os.ReadFile(outputPath)
	fmt.Println(string(content))
}
