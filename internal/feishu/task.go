package feishu

import (
	"context"
	"fmt"
	"time"

	larkbitable "github.com/larksuite/oapi-sdk-go/v3/service/bitable/v1"
)

// VideoTaskRecord 视频任务记录
type VideoTaskRecord struct {
	RecordID    string                 `json:"record_id"`
	URL         string                 `json:"url"`
	Title       string                 `json:"title"`
	Channel     string                 `json:"channel"`
	VideoID     string                 `json:"video_id"`
	Cookies     string                 `json:"cookies"`
	Status      string                 `json:"status"` // pending, downloading, transcribing, translating, completed, failed
	Error       string                 `json:"error,omitempty"`
	BVID        string                 `json:"bvid,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	Fields      map[string]interface{} `json:"-"`
}

// BitableConfig 多维表格配置
type BitableConfig struct {
	AppToken string `json:"app_token"`
	TableID  string `json:"table_id"`
}

// GetPendingTasks 获取待处理的任务
func (c *MultiTableClient) GetPendingTasks(config BitableConfig, pageSize int) ([]*VideoTaskRecord, error) {
	var allTasks []*VideoTaskRecord
	pageToken := ""

	for {
		reqBuilder := larkbitable.NewListAppTableRecordReqBuilder().
			AppToken(config.AppToken).
			TableId(config.TableID).
			PageSize(pageSize)

		if pageToken != "" {
			reqBuilder = reqBuilder.PageToken(pageToken)
		}

		// 添加过滤条件：status = "pending"
		filter := fmt.Sprintf("CurrentValue.[状态] = \"pending\"")
		reqBuilder = reqBuilder.Filter(filter)

		req := reqBuilder.Build()

		resp, err := c.client.Bitable.AppTableRecord.List(context.Background(), req)
		if err != nil {
			return nil, fmt.Errorf("查询任务失败: %v", err)
		}

		if !resp.Success() {
			return nil, fmt.Errorf("查询任务失败 [code=%d]: %s", resp.Code, resp.Msg)
		}

		// 转换记录格式
		for _, item := range resp.Data.Items {
			task := &VideoTaskRecord{
				RecordID: *item.RecordId,
				Fields:   item.Fields,
			}

			// 从 Fields 中提取数据
			if url, ok := item.Fields["链接"].(string); ok {
				task.URL = url
			}
			if title, ok := item.Fields["标题"].(string); ok {
				task.Title = title
			}
			if channel, ok := item.Fields["频道"].(string); ok {
				task.Channel = channel
			}
			if videoID, ok := item.Fields["视频ID"].(string); ok {
				task.VideoID = videoID
			}
			if cookies, ok := item.Fields["Cookies"].(string); ok {
				task.Cookies = cookies
			}
			if status, ok := item.Fields["状态"].(string); ok {
				task.Status = status
			}

			allTasks = append(allTasks, task)
		}

		// 检查是否还有更多数据
		if resp.Data.PageToken == nil || !*resp.Data.HasMore {
			break
		}
		pageToken = *resp.Data.PageToken
	}

	return allTasks, nil
}

// UpdateTaskStatus 更新任务状态
func (c *MultiTableClient) UpdateTaskStatus(config BitableConfig, recordID, status, error, bvid string) error {
	fields := map[string]interface{}{
		"状态":     status,
		"更新时间": time.Now().UnixMilli(), // 使用毫秒时间戳
	}

	if error != "" {
		fields["错误信息"] = error
	}

	if bvid != "" {
		fields["BVID"] = bvid
	}

	req := larkbitable.NewUpdateAppTableRecordReqBuilder().
		AppToken(config.AppToken).
		TableId(config.TableID).
		RecordId(recordID).
		AppTableRecord(larkbitable.NewAppTableRecordBuilder().
			Fields(fields).
			Build()).
		Build()

	resp, err := c.client.Bitable.AppTableRecord.Update(context.Background(), req)
	if err != nil {
		return fmt.Errorf("更新任务状态失败: %v", err)
	}

	if !resp.Success() {
		return fmt.Errorf("更新任务状态失败 [code=%d]: %s", resp.Code, resp.Msg)
	}

	return nil
}

// CreateTask 创建视频任务
func (c *MultiTableClient) CreateTask(config BitableConfig, task *VideoTaskRecord) (string, error) {
	now := time.Now().UnixMilli()
	fields := map[string]interface{}{
		"链接":     task.URL,
		"标题":     task.Title,
		"频道":     task.Channel,
		"视频ID":   task.VideoID,
		"Cookies": task.Cookies,
		"状态":     "pending",
		"创建时间": now,
		"更新时间": now,
	}

	recordID, err := c.CreateRecord(config.AppToken, config.TableID, fields)
	if err != nil {
		return "", fmt.Errorf("创建任务失败: %v", err)
	}

	return recordID, nil
}
