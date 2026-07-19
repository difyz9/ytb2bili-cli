// Command llm-batch-translator translates an SRT file in context-aware LLM batches.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/zolagz/ytb2bili-go/internal/config"
	"github.com/zolagz/ytb2bili-go/internal/translator"
)

const (
	defaultBatchSize   = 25
	defaultMaxWorkers  = 3
	defaultRetryCount  = 2
	defaultContextSize = 2
)

func main() {
	var options cliOptions
	flag.StringVar(&options.input, "input", "", "输入 SRT 文件路径（必填）")
	flag.StringVar(&options.output, "output", "", "输出 SRT 文件路径（默认：<名称>.<目标语言>.srt）")
	flag.StringVar(&options.configPath, "config", "config.yaml", "项目 YAML 配置文件")
	flag.StringVar(&options.sourceLang, "source-lang", "en", "源语言代码")
	flag.StringVar(&options.targetLang, "target-lang", "", "目标语言代码（默认读取配置）")
	flag.StringVar(&options.baseURL, "base-url", "", "OpenAI 兼容 API 地址（默认读取配置或 LLM_BASE_URL）")
	flag.StringVar(&options.model, "model", "", "翻译模型（默认读取配置或 LLM_MODEL）")
	flag.IntVar(&options.batchSize, "batch-size", defaultBatchSize, "每批待翻译字幕数")
	flag.IntVar(&options.maxWorkers, "workers", defaultMaxWorkers, "最大并发批次数")
	flag.IntVar(&options.retryCount, "retries", defaultRetryCount, "每批失败后的重试次数")
	flag.IntVar(&options.contextSize, "context-size", defaultContextSize, "每批前后各携带的上下文字幕数")
	flag.Parse()

	if err := options.validate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		flag.Usage()
		os.Exit(2)
	}

	appConfig, err := loadConfig(options.configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取配置失败: %v\n", err)
		os.Exit(2)
	}
	options.applyConfig(appConfig)
	if strings.TrimSpace(appConfig.LLMAPIKey) == "" {
		fmt.Fprintln(os.Stderr, "缺少 LLM API Key：请设置 DEEPSEEK_API_KEY 或在配置文件中填写 llm_api_key")
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	engine := translator.New(translator.Config{
		APIKey:      appConfig.LLMAPIKey,
		BaseURL:     options.baseURL,
		Model:       options.model,
		SourceLang:  options.sourceLang,
		TargetLang:  options.targetLang,
		BatchSize:   options.batchSize,
		MaxWorkers:  options.maxWorkers,
		RetryCount:  options.retryCount,
		ContextSize: options.contextSize,
	})

	fmt.Printf("批量翻译配置: batch=%d, workers=%d, retries=%d, context=%d, %s -> %s, model=%s\n",
		options.batchSize, options.maxWorkers, options.retryCount, options.contextSize,
		options.sourceLang, options.targetLang, options.model)
	if err := engine.TranslateSRTFile(ctx, options.input, options.output); err != nil {
		fmt.Fprintf(os.Stderr, "批量翻译失败: %v\n", err)
		os.Exit(1)
	}
}

type cliOptions struct {
	input, output, configPath string
	sourceLang, targetLang    string
	baseURL, model            string
	batchSize, maxWorkers     int
	retryCount, contextSize   int
}

func (o *cliOptions) validate() error {
	if strings.TrimSpace(o.input) == "" {
		return fmt.Errorf("--input 为必填参数")
	}
	if o.batchSize <= 0 {
		return fmt.Errorf("--batch-size 必须大于 0")
	}
	if o.maxWorkers <= 0 {
		return fmt.Errorf("--workers 必须大于 0")
	}
	if o.retryCount < 0 {
		return fmt.Errorf("--retries 不能小于 0")
	}
	if o.contextSize < 0 {
		return fmt.Errorf("--context-size 不能小于 0")
	}
	return nil
}

func (o *cliOptions) applyConfig(appConfig *config.Config) {
	if strings.TrimSpace(o.targetLang) == "" {
		o.targetLang = appConfig.EffectiveTranslationTargetLang()
	}
	if strings.TrimSpace(o.baseURL) == "" {
		o.baseURL = appConfig.LLMBaseURL
	}
	if strings.TrimSpace(o.model) == "" {
		o.model = appConfig.LLMModel
	}
	if strings.TrimSpace(o.output) == "" {
		o.output = translator.TranslatedSRTPath(o.input, o.targetLang)
	}
}

func loadConfig(path string) (*config.Config, error) {
	appConfig, err := config.LoadYAML(path)
	if err == nil {
		return appConfig, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	appConfig = config.Default()
	appConfig.Init()
	return appConfig, nil
}
