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
}

// TencentCloudConfig 腾讯云 API 凭证
type TencentCloudConfig struct {
	SecretID  string `yaml:"secret_id"`
	SecretKey string `yaml:"secret_key"`
	Region    string `yaml:"region"`
}

// TTSConfig 语音合成参数
type TTSConfig struct {
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
