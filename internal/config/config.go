package config

import (
	"os"

	"github.com/BurntSushi/toml"
)

// Config 配置
type Config struct {
	DataDir        string `toml:"data_dir"`
	LLMAPIKey      string `toml:"llm_api_key"`
	LLMBaseURL     string `toml:"llm_base_url"`
	LLMModel       string `toml:"llm_model"`
	BiliTid        int    `toml:"bili_tid"`
	YouTubeCookies string `toml:"youtube_cookies"`
	
	// 飞书多维表格配置
	FeishuAppID     string `toml:"feishu_app_id"`
	FeishuAppSecret string `toml:"feishu_app_secret"`
	BitableAppToken string `toml:"bitable_app_token"`
	BitableTableID  string `toml:"bitable_table_id"`
}

func Default() *Config {
	return &Config{
		DataDir:    "./data",
		LLMBaseURL: "https://api.deepseek.com",
		LLMModel:   "deepseek-v4-flash",
		BiliTid:    122,
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
	if cookies := os.Getenv("YOUTUBE_COOKIES"); cookies != "" {
		c.YouTubeCookies = cookies
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

func Load(path string) (*Config, error) {
	cfg := Default()
	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, err
	}
	cfg.Init()
	return cfg, nil
}
