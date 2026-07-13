package command

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/urfave/cli/v2"
	"github.com/zolagz/ytb2bili-go/internal/config"
	"github.com/zolagz/ytb2bili-go/internal/feishu"
	"github.com/zolagz/ytb2bili-go/internal/server"
)

// bitableCommand 多维表格任务命令
func bitableCommand(cfg *config.Config) *cli.Command {
	return &cli.Command{
		Name:  "bitable",
		Usage: "从飞书多维表格读取任务并执行",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "feishu-app-id", Usage: "飞书 App ID"},
			&cli.StringFlag{Name: "feishu-app-secret", Usage: "飞书 App Secret"},
			&cli.StringFlag{Name: "app-token", Usage: "多维表格 App Token"},
			&cli.StringFlag{Name: "table-id", Usage: "多维表格 Table ID"},
			&cli.DurationFlag{Name: "interval", Value: 30 * time.Second, Usage: "轮询间隔"},
		},
		Action: func(c *cli.Context) error {
			// 获取配置
			feishuAppID := c.String("feishu-app-id")
			if feishuAppID == "" {
				feishuAppID = cfg.FeishuAppID
			}
			if feishuAppID == "" {
				feishuAppID = os.Getenv("FEISHU_APP_ID")
			}

			feishuAppSecret := c.String("feishu-app-secret")
			if feishuAppSecret == "" {
				feishuAppSecret = cfg.FeishuAppSecret
			}
			if feishuAppSecret == "" {
				feishuAppSecret = os.Getenv("FEISHU_APP_SECRET")
			}

			appToken := c.String("app-token")
			if appToken == "" {
				appToken = cfg.BitableAppToken
			}
			if appToken == "" {
				appToken = os.Getenv("BITABLE_APP_TOKEN")
			}

			tableID := c.String("table-id")
			if tableID == "" {
				tableID = cfg.BitableTableID
			}
			if tableID == "" {
				tableID = os.Getenv("BITABLE_TABLE_ID")
			}

			interval := c.Duration("interval")

			// 验证配置
			if feishuAppID == "" || feishuAppSecret == "" {
				return fmt.Errorf("请提供飞书 App ID 和 App Secret")
			}
			if appToken == "" || tableID == "" {
				return fmt.Errorf("请提供多维表格 App Token 和 Table ID")
			}

			fmt.Println("🚀 启动多维表格任务处理器...")
			fmt.Println(strings.Repeat("=", 50))
			fmt.Printf("   飞书 App ID: %s\n", feishuAppID)
			fmt.Printf("   多维表格 Token: %s\n", appToken)
			fmt.Printf("   表格 ID: %s\n", tableID)
			fmt.Printf("   轮询间隔: %v\n", interval)
			fmt.Println(strings.Repeat("=", 50))

			// 创建飞书客户端
			client := feishu.NewMultiTableClient(feishuAppID, feishuAppSecret)

			// 创建多维表格配置
			bitableConfig := feishu.BitableConfig{
				AppToken: appToken,
				TableID:  tableID,
			}

			// 创建任务处理器
			processor := server.NewBitableProcessor(cfg, client, bitableConfig, interval)

			// 启动处理器
			processor.Start(context.Background())

			return nil
		},
	}
}
