package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	"github.com/larksuite/oapi-sdk-go/v3/channel"
	"github.com/larksuite/oapi-sdk-go/v3/channel/types"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

// FeishuBot 飞书机器人
type FeishuBot struct {
	appID       string
	appSecret   string
	client      *lark.Client
	wsClient    *larkws.Client
	channel     types.Channel
	messageChan chan *FeishuMessage
}

// FeishuMessage 飞书消息
type FeishuMessage struct {
	ChatID    string
	MessageID string
	UserID    string
	Content   string
	ChatType  string
}

// NewFeishuBot 创建飞书机器人
func NewFeishuBot(appID, appSecret string) *FeishuBot {
	client := lark.NewClient(appID, appSecret, lark.WithLogLevel(larkcore.LogLevelInfo))
	wsClient := larkws.NewClient(appID, appSecret, larkws.WithLogLevel(larkcore.LogLevelInfo))
	ch := channel.NewChannel(client, wsClient)

	bot := &FeishuBot{
		appID:       appID,
		appSecret:   appSecret,
		client:      client,
		wsClient:    wsClient,
		channel:     ch,
		messageChan: make(chan *FeishuMessage, 100),
	}

	// 注册消息处理
	ch.OnMessage(func(ctx context.Context, msg *types.NormalizedMessage) error {
		bot.messageChan <- &FeishuMessage{
			ChatID:    msg.ChatID,
			MessageID: msg.MessageID,
			UserID:    msg.UserID,
			Content:   msg.Content,
			ChatType:  msg.ChatType,
		}
		return nil
	})

	return bot
}

// Start 启动飞书机器人
func (b *FeishuBot) Start(ctx context.Context) error {
	b.channel.OnReady(func() {
		fmt.Println("✅ 飞书机器人已就绪")
	})

	b.channel.OnError(func(err error) {
		fmt.Printf("❌ 飞书机器人错误: %v\n", err)
	})

	return b.channel.Start(ctx)
}

// Stop 停止飞书机器人
func (b *FeishuBot) Stop(ctx context.Context) error {
	return b.channel.Stop(ctx)
}

// SendMessage 发送文本消息
func (b *FeishuBot) SendMessage(ctx context.Context, chatID string, text string) error {
	_, err := b.channel.Send(ctx, &types.SendInput{
		ChatID: chatID,
		Text:   text,
	})
	return err
}

// SendMarkdown 发送 Markdown 消息
func (b *FeishuBot) SendMarkdown(ctx context.Context, chatID string, markdown string) error {
	_, err := b.channel.Send(ctx, &types.SendInput{
		ChatID:   chatID,
		Markdown: markdown,
	})
	return err
}

// SendCard 发送卡片消息
func (b *FeishuBot) SendCard(ctx context.Context, chatID string, card map[string]interface{}) error {
	cardJSON, _ := json.Marshal(card)
	_, err := b.channel.Send(ctx, &types.SendInput{
		ChatID: chatID,
		Card:   string(cardJSON),
	})
	return err
}

// ReplyMessage 回复消息
func (b *FeishuBot) ReplyMessage(ctx context.Context, msg *FeishuMessage, text string) error {
	_, err := b.channel.Send(ctx, &types.SendInput{
		ChatID:         msg.ChatID,
		Text:           text,
		ReplyMessageID: msg.MessageID,
	})
	return err
}

// ReplyMarkdown 回复 Markdown
func (b *FeishuBot) ReplyMarkdown(ctx context.Context, msg *FeishuMessage, markdown string) error {
	_, err := b.channel.Send(ctx, &types.SendInput{
		ChatID:         msg.ChatID,
		Markdown:       markdown,
		ReplyMessageID: msg.MessageID,
	})
	return err
}

// ReplyCard 回复卡片
func (b *FeishuBot) ReplyCard(ctx context.Context, msg *FeishuMessage, card map[string]interface{}) error {
	cardJSON, _ := json.Marshal(card)
	_, err := b.channel.Send(ctx, &types.SendInput{
		ChatID:         msg.ChatID,
		Card:           string(cardJSON),
		ReplyMessageID: msg.MessageID,
	})
	return err
}

// GetMessageChan 获取消息通道
func (b *FeishuBot) GetMessageChan() <-chan *FeishuMessage {
	return b.messageChan
}

// CreateTaskCard 创建任务卡片
func CreateTaskCard(taskID, title, status, bvid string) map[string]interface{} {
	statusEmoji := map[string]string{
		"pending":    "⏳",
		"processing": "🔄",
		"completed":  "✅",
		"failed":     "❌",
	}

	elements := []interface{}{
		map[string]interface{}{
			"tag": "div",
			"text": map[string]interface{}{
				"tag":     "lark_md",
				"content": fmt.Sprintf("**任务ID:** %s", taskID),
			},
		},
		map[string]interface{}{
			"tag": "div",
			"text": map[string]interface{}{
				"tag":     "lark_md",
				"content": fmt.Sprintf("**视频标题:** %s", title),
			},
		},
		map[string]interface{}{
			"tag": "div",
			"text": map[string]interface{}{
				"tag":     "lark_md",
				"content": fmt.Sprintf("**状态:** %s %s", statusEmoji[status], status),
			},
		},
	}

	if bvid != "" {
		elements = append(elements, map[string]interface{}{
			"tag": "div",
			"text": map[string]interface{}{
				"tag":     "lark_md",
				"content": fmt.Sprintf("**B站链接:** https://www.bilibili.com/video/%s", bvid),
			},
		})
	}

	return map[string]interface{}{
		"config": map[string]interface{}{
			"wide_screen_mode": true,
		},
		"header": map[string]interface{}{
			"title": map[string]interface{}{
				"tag":     "plain_text",
				"content": "YouTube 视频处理",
			},
			"template": "blue",
		},
		"elements": elements,
	}
}

// CreateHistoryCard 创建历史记录卡片
func CreateHistoryCard(videos []map[string]string) map[string]interface{} {
	var elements []interface{}

	for i, v := range videos {
		elements = append(elements, map[string]interface{}{
			"tag": "div",
			"text": map[string]interface{}{
				"tag":     "lark_md",
				"content": fmt.Sprintf("%d. **%s**\n   YouTube: %s\n   B站: %s", i+1, v["title"], v["youtube_url"], v["bilibili_url"]),
			},
		})
	}

	if len(videos) == 0 {
		elements = append(elements, map[string]interface{}{
			"tag": "div",
			"text": map[string]interface{}{
				"tag":     "lark_md",
				"content": "暂无提交记录",
			},
		})
	}

	return map[string]interface{}{
		"config": map[string]interface{}{
			"wide_screen_mode": true,
		},
		"header": map[string]interface{}{
			"title": map[string]interface{}{
				"tag":     "plain_text",
				"content": "📋 提交历史",
			},
			"template": "green",
		},
		"elements": elements,
	}
}

// CreateHelpCard 创建帮助卡片
func CreateHelpCard() map[string]interface{} {
	return map[string]interface{}{
		"config": map[string]interface{}{
			"wide_screen_mode": true,
		},
		"header": map[string]interface{}{
			"title": map[string]interface{}{
				"tag":     "plain_text",
				"content": "📖 帮助",
			},
			"template": "purple",
		},
		"elements": []interface{}{
			map[string]interface{}{
				"tag": "div",
				"text": map[string]interface{}{
					"tag":     "lark_md",
					"content": "发送 YouTube 视频链接即可自动处理",
				},
			},
			map[string]interface{}{
				"tag": "div",
				"text": map[string]interface{}{
					"tag":     "lark_md",
					"content": "**命令:**\n- `help` - 显示帮助\n- `history` - 查看历史\n- `status` - 查看状态",
				},
			},
		},
	}
}

// CreateStatusCard 创建状态卡片
func CreateStatusCard(taskCount, historyCount int) map[string]interface{} {
	return map[string]interface{}{
		"config": map[string]interface{}{
			"wide_screen_mode": true,
		},
		"header": map[string]interface{}{
			"title": map[string]interface{}{
				"tag":     "plain_text",
				"content": "📊 系统状态",
			},
			"template": "blue",
		},
		"elements": []interface{}{
			map[string]interface{}{
				"tag": "div",
				"text": map[string]interface{}{
					"tag":     "lark_md",
					"content": fmt.Sprintf("**待处理任务:** %d\n**已完成任务:** %d", taskCount, historyCount),
				},
			},
		},
	}
}

// ParseYouTubeURL 解析 YouTube URL
func ParseYouTubeURL(text string) string {
	text = strings.TrimSpace(text)

	// youtube.com/watch?v=xxx
	if strings.Contains(text, "youtube.com/watch?v=") {
		parts := strings.Split(text, "v=")
		if len(parts) > 1 {
			id := strings.Split(parts[1], "&")[0]
			if len(id) == 11 {
				return "https://www.youtube.com/watch?v=" + id
			}
		}
	}

	// youtu.be/xxx
	if strings.Contains(text, "youtu.be/") {
		parts := strings.Split(text, "youtu.be/")
		if len(parts) > 1 {
			id := strings.Split(parts[1], "?")[0]
			if len(id) == 11 {
				return "https://www.youtube.com/watch?v=" + id
			}
		}
	}

	// youtube.com/shorts/xxx
	if strings.Contains(text, "youtube.com/shorts/") {
		parts := strings.Split(text, "shorts/")
		if len(parts) > 1 {
			id := strings.Split(parts[1], "?")[0]
			if len(id) == 11 {
				return "https://www.youtube.com/watch?v=" + id
			}
		}
	}

	return ""
}
