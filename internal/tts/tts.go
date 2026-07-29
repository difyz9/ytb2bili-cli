// Package tts 腾讯云语音合成
//
// 读取 SRT 字幕文件，逐条调用腾讯云 TTS API 生成音频文件，
// 按字幕序号命名（1.mp3, 2.mp3, ...），供 audio-sync 步骤使用。
package tts

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	tts "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/tts/v20190823"

	appcfg "github.com/zolagz/ytb2bili-go/internal/config"
)

// Config 腾讯云 TTS 配置
type Config struct {
	SecretID  string // TENCENTCLOUD_SECRET_ID
	SecretKey string // TENCENTCLOUD_SECRET_KEY
	Region    string // 地域，默认 ap-guangzhou
	Voice     int64  // 音色，默认 0（亲和女声）
	Volume    float64 // 音量，0-15，默认 2
	Speed     float64 // 语速，0-2，默认 1
	ModelType int64  // 模型类型，默认 1（普通）
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		Region:    "ap-guangzhou",
		Voice:     0,
		Volume:    2,
		Speed:     1,
		ModelType: 1,
	}
}

// FromAppConfig 从 config.Config 创建 TTS 配置
func FromAppConfig(appCfg interface{}) Config {
	c := DefaultConfig()
	switch v := appCfg.(type) {
	case *appcfg.Config:
		if v.TencentCloud != nil {
			if v.TencentCloud.SecretID != "" {
				c.SecretID = v.TencentCloud.SecretID
			}
			if v.TencentCloud.SecretKey != "" {
				c.SecretKey = v.TencentCloud.SecretKey
			}
			if v.TencentCloud.Region != "" {
				c.Region = v.TencentCloud.Region
			}
		}
		if v.TTS != nil {
			c.Voice = v.TTS.VoiceType
			c.Volume = v.TTS.Volume
			c.Speed = v.TTS.Speed
		}
	}
	if c.SecretID == "" {
		c.SecretID = os.Getenv("TENCENTCLOUD_SECRET_ID")
	}
	if c.SecretKey == "" {
		c.SecretKey = os.Getenv("TENCENTCLOUD_SECRET_KEY")
	}
	return c
}

// FillFromEnv 从环境变量填充配置
func (c *Config) FillFromEnv() {
	if c.SecretID == "" {
		c.SecretID = os.Getenv("TENCENTCLOUD_SECRET_ID")
	}
	if c.SecretKey == "" {
		c.SecretKey = os.Getenv("TENCENTCLOUD_SECRET_KEY")
	}
}

// Validate 检查配置是否有效
func (c *Config) Validate() error {
	if c.SecretID == "" {
		return fmt.Errorf("TENCENTCLOUD_SECRET_ID 未设置")
	}
	if c.SecretKey == "" {
		return fmt.Errorf("TENCENTCLOUD_SECRET_KEY 未设置")
	}
	return nil
}

// Result 单条字幕的合成结果
type Result struct {
	Index int    // 字幕序号
	Text  string // 原始文本
	Path  string // 音频文件路径
	Err   error  // 错误信息
}

// SynthesizeSRT 读取 SRT 字幕文件，逐条调用腾讯云 TTS 合成音频
// outputDir: 输出目录，每个字幕生成 <n>.mp3 文件
// concurrency: 并发数（0 = 顺序合成）
// 返回每条字幕的合成结果
func SynthesizeSRT(ctx context.Context, srtPath, outputDir string, cfg Config, concurrency int) ([]Result, error) {
	cfg.FillFromEnv()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	// 读取 SRT
	entries, err := parseSRT(srtPath)
	if err != nil {
		return nil, fmt.Errorf("解析字幕失败: %w", err)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("字幕为空")
	}

	// 确保输出目录
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("创建输出目录失败: %w", err)
	}

	// 创建客户端
	client, err := newClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("创建 TTS 客户端失败: %w", err)
	}

	if concurrency <= 0 {
		concurrency = 1
	}

	// 使用 channel 控制并发
	sem := make(chan struct{}, concurrency)
	var mu sync.Mutex
	var results []Result
	var wg sync.WaitGroup

	for _, entry := range entries {
		if ctx.Err() != nil {
			break
		}

		// 过滤过短文本
		text := strings.TrimSpace(entry.Text)
		if text == "" {
			results = append(results, Result{Index: entry.Index, Text: text, Err: fmt.Errorf("空文本，跳过")})
			continue
		}

		sem <- struct{}{}
		wg.Add(1)

		go func(idx int, txt string) {
			defer func() {
				<-sem
				wg.Done()
			}()

			outputPath := filepath.Join(outputDir, fmt.Sprintf("%d.mp3", idx))
			fmt.Printf("  🎤 [%d] TTS: %s\n", idx, truncate(txt, 40))

			err := synthesizeOne(ctx, client, txt, cfg, outputPath)
			mu.Lock()
			results = append(results, Result{Index: idx, Text: txt, Path: outputPath, Err: err})
			mu.Unlock()
			if err != nil {
				fmt.Printf("  ❌ [%d] 合成失败: %v\n", idx, err)
			} else {
				fmt.Printf("  ✅ [%d] %s\n", idx, outputPath)
			}
		}(entry.Index, text)
	}

	wg.Wait()

	return results, nil
}

// synthesizeOne 调用腾讯云 TTS API 合成单条文本并保存为 MP3
func synthesizeOne(ctx context.Context, client *tts.Client, text string, cfg Config, outputPath string) error {
	request := tts.NewTextToVoiceRequest()
	request.Text = common.StringPtr(text)
	request.Volume = common.Float64Ptr(cfg.Volume)
	request.Speed = common.Float64Ptr(cfg.Speed)
	request.ModelType = common.Int64Ptr(cfg.ModelType)
	request.VoiceType = common.Int64Ptr(cfg.Voice)
	request.SessionId = common.StringPtr(fmt.Sprintf("srt_%d", os.Getpid()))

	response, err := client.TextToVoice(request)
	if err != nil {
		if sdkErr, ok := err.(*errors.TencentCloudSDKError); ok {
			return fmt.Errorf("API 错误: %s", sdkErr.Message)
		}
		return fmt.Errorf("请求失败: %w", err)
	}

	if response.Response.Audio == nil {
		return fmt.Errorf("返回数据为空")
	}

	// Base64 解码音频数据
	audioData, err := base64.StdEncoding.DecodeString(*response.Response.Audio)
	if err != nil {
		return fmt.Errorf("音频数据解码失败: %w", err)
	}

	return os.WriteFile(outputPath, audioData, 0644)
}

// newClient 创建腾讯云 TTS 客户端
func newClient(cfg Config) (*tts.Client, error) {
	credential := common.NewCredential(cfg.SecretID, cfg.SecretKey)
	cpf := profile.NewClientProfile()
	cpf.HttpProfile.Endpoint = "tts.tencentcloudapi.com"
	return tts.NewClient(credential, cfg.Region, cpf)
}

// srtEntry SRT 条目
type srtEntry struct {
	Index    int
	TimeCode string
	Text     string
}

// parseSRT 解析 SRT 文件
func parseSRT(path string) ([]srtEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []srtEntry
	var current srtEntry
	var textLines []string
	stage := 0

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			if stage == 2 && len(textLines) > 0 {
				current.Text = strings.Join(textLines, "\n")
				entries = append(entries, current)
			}
			current = srtEntry{}
			textLines = nil
			stage = 0
			continue
		}

		switch stage {
		case 0:
			var idx int
			if _, err := fmt.Sscanf(line, "%d", &idx); err == nil {
				current.Index = idx
				stage = 1
			}
		case 1:
			if strings.Contains(line, "-->") {
				current.TimeCode = line
				stage = 2
			}
		case 2:
			textLines = append(textLines, line)
		}
	}

	// 最后一次 flush
	if stage == 2 && len(textLines) > 0 {
		current.Text = strings.Join(textLines, "\n")
		entries = append(entries, current)
	}

	return entries, scanner.Err()
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "..."
}
