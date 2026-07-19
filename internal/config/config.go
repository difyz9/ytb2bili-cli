package config

import (
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

// Config 配置
type Config struct {
	DataDir               string   `toml:"data_dir" yaml:"data_dir"`
	LLMAPIKey             string   `toml:"llm_api_key" yaml:"llm_api_key"`
	LLMBaseURL            string   `toml:"llm_base_url" yaml:"llm_base_url"`
	LLMModel              string   `toml:"llm_model" yaml:"llm_model"`
	TranslationTargetLang string   `toml:"translation_target_lang" yaml:"translation_target_lang"`
	BiliTid               int      `toml:"bili_tid" yaml:"bili_tid"`
	YouTubeCookies        string   `toml:"youtube_cookies" yaml:"youtube_cookies"`
	ServerToken           string   `toml:"server_token" yaml:"server_token"`
	AllowedOrigins        []string `toml:"allowed_origins" yaml:"allowed_origins"`

	// 飞书多维表格配置
	FeishuAppID     string `toml:"feishu_app_id" yaml:"feishu_app_id"`
	FeishuAppSecret string `toml:"feishu_app_secret" yaml:"feishu_app_secret"`
	BitableAppToken string `toml:"bitable_app_token" yaml:"bitable_app_token"`
	BitableTableID  string `toml:"bitable_table_id" yaml:"bitable_table_id"`
}

func Default() *Config {
	return &Config{
		DataDir:               "./data",
		LLMBaseURL:            "https://api.deepseek.com",
		LLMModel:              "deepseek-v4-flash",
		TranslationTargetLang: "zh-Hans",
		BiliTid:               122,
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

func Load(path string) (*Config, error) {
	cfg := Default()
	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, err
	}
	cfg.Init()
	return cfg, nil
}
