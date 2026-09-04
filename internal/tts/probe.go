package tts

// 服务可用性探针：ytb check 等服务自检使用，只读、不落盘。

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"
	tts "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/tts/v20190823"

	appcfg "github.com/zolagz/ytb2bili-go/internal/config"
)

// Probe 测试腾讯云 TTS 可用性：用当前配置合成一段极短文本并校验返回音频非空。
// 返回耗时；nil 表示凭据有效、接口可用（会实际调用一次 TTS，极小成本）。
func Probe(appCfg *appcfg.Config) (time.Duration, error) {
	cfg := FromAppConfig(appCfg)
	if err := cfg.Validate(); err != nil {
		return 0, err
	}
	client, err := newClient(cfg)
	if err != nil {
		return 0, fmt.Errorf("创建 TTS 客户端失败: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req := tts.NewTextToVoiceRequest()
	req.Text = common.StringPtr("语音合成可用性测试")
	req.VoiceType = common.Int64Ptr(cfg.Voice)
	req.Volume = common.Float64Ptr(cfg.Volume)
	req.Speed = common.Float64Ptr(cfg.Speed)
	req.ModelType = common.Int64Ptr(cfg.ModelType)
	req.SessionId = common.StringPtr(fmt.Sprintf("probe_%d", os.Getpid()))

	start := time.Now()
	resp, err := client.TextToVoiceWithContext(ctx, req)
	latency := time.Since(start)
	if err != nil {
		if sdkErr, ok := err.(*errors.TencentCloudSDKError); ok {
			return latency, fmt.Errorf("API 错误: %s (code=%s)", sdkErr.Message, sdkErr.Code)
		}
		return latency, fmt.Errorf("请求失败: %w", err)
	}
	if resp.Response.Audio == nil || *resp.Response.Audio == "" {
		return latency, fmt.Errorf("返回音频为空（凭据/音色配置可能无效）")
	}
	return latency, nil
}
