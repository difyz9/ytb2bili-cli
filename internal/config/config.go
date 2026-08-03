package config

import (
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config 配置
type Config struct {
	DataDir               string   `yaml:"data_dir"`
	LLMAPIKey             string   `yaml:"llm_api_key"`
	LLMBaseURL            string   `yaml:"llm_base_url"`
	LLMModel              string   `yaml:"llm_model"`
	TranslationTargetLang string   `yaml:"translation_target_lang"`
	BiliTid               int      `yaml:"bili_tid"`
	YouTubeCookies        string   `yaml:"youtube_cookies"`
	ServerToken           string   `yaml:"server_token"`
	AllowedOrigins        []string `yaml:"allowed_origins"`

	// 飞书多维表格配置
	FeishuAppID     string `yaml:"feishu_app_id"`
	FeishuAppSecret string `yaml:"feishu_app_secret"`
	BitableAppToken string `yaml:"bitable_app_token"`
	BitableTableID  string `yaml:"bitable_table_id"`

	// 腾讯云 TTS 配置
	TencentCloud *TencentCloudConfig `yaml:"tencent_cloud"`
	TTS          *TTSConfig          `yaml:"tts"`
	Concurrent   *ConcurrentConfig   `yaml:"concurrent"`

	// 转录后端配置
	Transcriber *TranscriberConfig `yaml:"transcriber"`
}

// TranscriberConfig 转录后端配置
type TranscriberConfig struct {
	// Provider 指定 pipeline transcribe 步骤使用的转录器：
	//   bcut - Bcut ASR 云服务；whisper - 本地 whisper.cpp（whisper-cli）；空/未配置 - 默认 whisper
	Provider string         `yaml:"provider"`
	Whisper  *WhisperConfig `yaml:"whisper"`
}

// WhisperConfig whisper.cpp 本地转录参数
type WhisperConfig struct {
	// Binary whisper-cli 可执行文件路径（默认 "whisper-cli"，按 PATH 查找）
	Binary string `yaml:"binary"`
	// Model GGML 模型文件路径（默认 "models/ggml-base.bin"）
	Model string `yaml:"model"`
	// Threads 推理线程数（默认 4）
	Threads int `yaml:"threads"`
}

// TencentCloudConfig 腾讯云 API 凭证
type TencentCloudConfig struct {
	SecretID  string `yaml:"secret_id"`
	SecretKey string `yaml:"secret_key"`
	Region    string `yaml:"region"`
}

// TTSConfig 语音合成参数
type TTSConfig struct {
	// Provider 指定 pipeline tts 步骤使用的合成器：
	//   tencent - 腾讯云 TTS；index - 本地 IndexTTS；空/auto - 自动检测（有腾讯云凭据用 tencent，否则用 index）
	Provider        string  `yaml:"provider"`
	VoiceType       int64   `yaml:"voice_type"`
	Volume          float64 `yaml:"volume"`
	Speed           float64 `yaml:"speed"`
	PrimaryLanguage int     `yaml:"primary_language"`
	SampleRate      int64   `yaml:"sample_rate"`
	Codec           string  `yaml:"codec"`
}

// ConcurrentConfig 并发处理配置
type ConcurrentConfig struct {
	MaxWorkers int `yaml:"max_workers"`
	RateLimit  int `yaml:"rate_limit"`
	BatchSize  int `yaml:"batch_size"`
}

func Default() *Config {
	return &Config{
		DataDir:               "./data",
		LLMBaseURL:            "https://api.deepseek.com",
		LLMModel:              "deepseek-v4-flash",
		TranslationTargetLang: "zh-Hans",
		BiliTid:               122,
		TTS: &TTSConfig{
			VoiceType:       101008,
			Volume:          5,
			Speed:           1.0,
			PrimaryLanguage: 1,
			SampleRate:      16000,
			Codec:           "mp3",
		},
		Concurrent: &ConcurrentConfig{
			MaxWorkers: 5,
			RateLimit:  20,
			BatchSize:  10,
		},
		Transcriber: &TranscriberConfig{
			Provider: "whisper",
			Whisper: &WhisperConfig{
				Binary:  "whisper-cli",
				Model:   "models/ggml-base.bin",
				Threads: 4,
			},
		},
	}
}

func (c *Config) Init() {
	if c.LLMAPIKey == "" {
		c.LLMAPIKey = os.Getenv("DEEPSEEK_API_KEY")
	}
	if base := os.Getenv("LLM_BASE_URL"); base != "" {
		c.LLMBaseURL = base
	}
	if model := os.Getenv("LLM_MODEL"); model != "" {
		c.LLMModel = model
	}
	if target := strings.TrimSpace(os.Getenv("YTB2BILI_TRANSLATION_TARGET_LANG")); target != "" {
		c.TranslationTargetLang = target
	}
	if strings.TrimSpace(c.TranslationTargetLang) == "" {
		c.TranslationTargetLang = "zh-Hans"
	}
	if cookies := os.Getenv("YOUTUBE_COOKIES"); cookies != "" {
		c.YouTubeCookies = cookies
	}
	if c.ServerToken == "" {
		c.ServerToken = os.Getenv("YTB2BILI_SERVER_TOKEN")
	}
	if origins := os.Getenv("YTB2BILI_ALLOWED_ORIGINS"); origins != "" {
		c.AllowedOrigins = nil
		for _, origin := range strings.Split(origins, ",") {
			if origin = strings.TrimSpace(origin); origin != "" {
				c.AllowedOrigins = append(c.AllowedOrigins, origin)
			}
		}
	}

	// 飞书多维表格配置
	if c.FeishuAppID == "" {
		c.FeishuAppID = os.Getenv("FEISHU_APP_ID")
	}
	if c.FeishuAppSecret == "" {
		c.FeishuAppSecret = os.Getenv("FEISHU_APP_SECRET")
	}
	if c.BitableAppToken == "" {
		c.BitableAppToken = os.Getenv("BITABLE_APP_TOKEN")
	}
	if c.BitableTableID == "" {
		c.BitableTableID = os.Getenv("BITABLE_TABLE_ID")
	}

	// 腾讯云 TTS 配置
	if c.TencentCloud == nil {
		c.TencentCloud = &TencentCloudConfig{}
	}
	if c.TencentCloud.SecretID == "" {
		c.TencentCloud.SecretID = os.Getenv("TENCENTCLOUD_SECRET_ID")
	}
	if c.TencentCloud.SecretKey == "" {
		c.TencentCloud.SecretKey = os.Getenv("TENCENTCLOUD_SECRET_KEY")
	}
	if c.TencentCloud.Region == "" {
		c.TencentCloud.Region = os.Getenv("TENCENTCLOUD_REGION")
	}
	if c.TencentCloud.Region == "" {
		c.TencentCloud.Region = "ap-guangzhou"
	}

	// 转录后端配置
	if c.Transcriber == nil {
		c.Transcriber = &TranscriberConfig{Provider: "whisper"}
	}
	if c.Transcriber.Whisper == nil {
		c.Transcriber.Whisper = &WhisperConfig{Binary: "whisper-cli", Model: "models/ggml-base.bin", Threads: 4}
	}
	if c.Transcriber.Whisper.Binary == "" {
		c.Transcriber.Whisper.Binary = "whisper-cli"
	}
	if c.Transcriber.Whisper.Model == "" {
		c.Transcriber.Whisper.Model = "models/ggml-base.bin"
	}
	if c.Transcriber.Whisper.Threads <= 0 {
		c.Transcriber.Whisper.Threads = 4
	}
	if binary := os.Getenv("YTB2BILI_WHISPER_BINARY"); binary != "" {
		c.Transcriber.Whisper.Binary = binary
	}
	if model := os.Getenv("YTB2BILI_WHISPER_MODEL"); model != "" {
		c.Transcriber.Whisper.Model = model
	}
}

func (c *Config) EffectiveTranscriberProvider() string {
	if c != nil && c.Transcriber != nil {
		switch strings.ToLower(strings.TrimSpace(c.Transcriber.Provider)) {
		case "bcut":
			return "bcut"
		}
	}
	return "whisper"
}

func (c *Config) EffectiveTranslationTargetLang() string {
	if c != nil && strings.TrimSpace(c.TranslationTargetLang) != "" {
		return strings.TrimSpace(c.TranslationTargetLang)
	}
	return "zh-Hans"
}

func LoadYAML(path string) (*Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	cfg.Init()
	return cfg, nil
}
