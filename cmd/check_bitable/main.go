package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	larkbitable "github.com/larksuite/oapi-sdk-go/v3/service/bitable/v1"

	"github.com/larksuite/oapi-sdk-go/v3"
)

func main() {
	appID := os.Getenv("FEISHU_APP_ID")
	appSecret := os.Getenv("FEISHU_APP_SECRET")
	appToken := os.Getenv("BITABLE_APP_TOKEN")
	tableID := os.Getenv("BITABLE_TABLE_ID")

	if appID == "" || appSecret == "" || appToken == "" || tableID == "" {
		fmt.Fprintln(os.Stderr, "缺少配置：请设置 FEISHU_APP_ID、FEISHU_APP_SECRET、BITABLE_APP_TOKEN 和 BITABLE_TABLE_ID")
		os.Exit(2)
	}

	fmt.Println("🔍 查询飞书多维表格中的任务...")
	fmt.Println(strings.Repeat("=", 60))

	// 创建飞书客户端
	client := lark.NewClient(appID, appSecret)

	// 查询所有记录（不过滤状态）
	reqBuilder := larkbitable.NewListAppTableRecordReqBuilder().
		AppToken(appToken).
		TableId(tableID).
		PageSize(100)

	req := reqBuilder.Build()
	resp, err := client.Bitable.AppTableRecord.List(context.Background(), req)
	if err != nil {
		fmt.Printf("❌ 查询失败: %v\n", err)
		os.Exit(1)
	}

	if !resp.Success() {
		fmt.Printf("❌ 查询失败 [code=%d]: %s\n", resp.Code, resp.Msg)
		os.Exit(1)
	}

	fmt.Printf("📊 共找到 %d 条记录\n\n", len(resp.Data.Items))

	for i, item := range resp.Data.Items {
		fmt.Printf("【记录 %d】%s\n", i+1, *item.RecordId)
		fmt.Println(strings.Repeat("-", 40))

		// 提取字段
		url := getStringField(item.Fields, "链接")
		title := getStringField(item.Fields, "标题")
		channel := getStringField(item.Fields, "频道")
		videoID := getStringField(item.Fields, "视频ID")
		cookies := getStringField(item.Fields, "Cookies")
		status := getStringField(item.Fields, "状态")
		errorMsg := getStringField(item.Fields, "错误信息")
		bvid := getStringField(item.Fields, "BVID")

		fmt.Printf("  链接: %s\n", url)
		fmt.Printf("  标题: %s\n", title)
		fmt.Printf("  频道: %s\n", channel)
		fmt.Printf("  视频ID: %s\n", videoID)
		fmt.Printf("  状态: %s\n", status)
		if errorMsg != "" {
			fmt.Printf("  错误: %s\n", errorMsg)
		}
		if bvid != "" {
			fmt.Printf("  BVID: %s\n", bvid)
		}

		// 分析 cookies
		fmt.Printf("  Cookies 长度: %d 字符\n", len(cookies))
		if cookies != "" {
			analyzeCookies(cookies)
		}

		fmt.Println()
	}
}

func getStringField(fields map[string]interface{}, key string) string {
	if v, ok := fields[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func analyzeCookies(cookiesJSON string) {
	// 简单分析 cookies 格式
	if strings.HasPrefix(cookiesJSON, "[") {
		fmt.Printf("  Cookies 格式: JSON 数组\n")

		// 统计关键 cookies
		keyCookies := []string{"LOGIN_INFO", "__Secure-1PSID", "__Secure-3PSID", "SAPISID", "SSID", "HSID", "SID"}
		found := 0
		for _, key := range keyCookies {
			if strings.Contains(cookiesJSON, key) {
				found++
			}
		}
		fmt.Printf("  关键 Cookies: %d/%d\n", found, len(keyCookies))

		// 检查是否有过期时间
		if strings.Contains(cookiesJSON, "\"expire\":0") || strings.Contains(cookiesJSON, "\"expire\": 0") {
			fmt.Printf("  ⚠️ 警告: 有 cookies 的过期时间为 0 (session cookies)\n")
		} else {
			fmt.Printf("  ✅ Cookies 有过期时间\n")
		}
	} else if strings.Contains(cookiesJSON, "Netscape") {
		fmt.Printf("  Cookies 格式: Netscape\n")
	} else {
		fmt.Printf("  Cookies 格式: 未知\n")
	}

	// 显示部分 cookies 内容（不显示完整值）
	fmt.Printf("  Cookies 预览: %s...\n", cookiesJSON[:min(100, len(cookiesJSON))])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
