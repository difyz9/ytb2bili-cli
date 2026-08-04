package config

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ExpandHome 将路径开头的 ~/ 展开为用户主目录（~ 单独出现时也处理）。
// 便于在配置里写 ~/.biliup/models/ggml-base.bin 这类复用系统模型的路径。
func ExpandHome(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
		return path
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// Config 配置
type Config struct {
	DataDir    string `yaml:"data_dir"`
	DownloadDir string `yaml:"download_dir"` // 视频下载根目录（默认 <data_dir>/downloads）
	LLMAPIKey  string `yaml:"llm_api_key"`
	LLMBaseURL            string   `yaml:"llm_base_url"`
	LLMModel              string   `yaml:"llm_model"`
	TranslationTargetLang string   `yaml:"translation_target_lang"`
	BiliTid               int      `yaml:"bili_tid"`
	YouTubeCookies        string   `yaml:"youtube_cookies"`
	ServerToken           string   `yaml:"server_token"`
	AllowedOrigins        []string `yaml:"allowed_origins"`
	ChromeDebugPort       int      `yaml:"chrome_debug_port"` // Chrome 远程调试起始端口（0=默认 9222，被占用自动 +1 找空闲）
	MinDurationSec        int      `yaml:"min_duration_sec"`  // 入队时低于该秒数视为 Short 跳过（0=不限制）

	// 飞书多维表格配置
	FeishuAppID     string `yaml:"feishu_app_id"`
	FeishuAppSecret string `yaml:"feishu_app_secret"`
	BitableAppToken string `yaml:"bitable_app_token"`
	BitableTableID  string `yaml:"bitable_table_id"`

	// 腾讯云 TTS 配置
	TencentCloud *TencentCloudConfig `yaml:"tencent_cloud"`
	TTS          *TTSConfig          `yaml:"tts"`
	Concurrent   *ConcurrentConfig   `yaml:"concurrent"`

	// YouTube OAuth 授权配置
	YouTubeOAuth *YouTubeOAuthConfig `yaml:"youtube_oauth"`

	// 转录后端配置
	Transcriber *TranscriberConfig `yaml:"transcriber"`
}

// YouTubeOAuthConfig Google OAuth 客户端凭证
type YouTubeOAuthConfig struct {
	// ClientID Google OAuth 客户端 ID（Google Cloud Console 创建）
	ClientID string `yaml:"client_id"`
	// ClientSecret Google OAuth 客户端密钥
	ClientSecret string `yaml:"client_secret"`
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
	// Index 本地 IndexTTS 服务配置（provider=index 时生效）
	Index *IndexTTSConfig `yaml:"index"`
}

// IndexTTSConfig 本地 IndexTTS2 HTTP 服务参数
type IndexTTSConfig struct {
	// APIURL IndexTTS2 服务地址（默认 http://localhost:18765）
	APIURL string `yaml:"api_url"`
	// Emotion 情感预设：default/happy/angry/sad/excited 等（默认 default）
	Emotion string `yaml:"emotion"`
	// EmotionAlpha 情感强度 0-1（默认 0.6）
	EmotionAlpha float64 `yaml:"emotion_alpha"`
	// RefAudio 参考音频路径（服务端可见路径，用于语音克隆音色）
	RefAudio string `yaml:"ref_audio"`
	// UseEmoText 是否使用文本情感描述（默认 false）
	UseEmoText bool `yaml:"use_emo_text"`
	// EmoText 文本情感描述（use_emo_text=true 时生效）
	EmoText string `yaml:"emo_text"`
	// Concurrency 并发合成数（默认 1）
	Concurrency int `yaml:"concurrency"`
	// Retries 失败重试次数（默认 3）
	Retries int `yaml:"retries"`
	// Timeout 单条请求超时秒数（默认 180）
	Timeout float64 `yaml:"timeout"`
	// ServerOutputDir 服务端输出目录（synthesize_srt.py 中转用，一般无需修改）
	ServerOutputDir string `yaml:"server_output_dir"`
}

// DefaultIndexTTSConfig 返回 IndexTTS 默认配置
func DefaultIndexTTSConfig() *IndexTTSConfig {
	return &IndexTTSConfig{
		APIURL:       "http://localhost:18765",
		Emotion:      "default",
		EmotionAlpha: 0.6,
		Concurrency:  1,
		Retries:      3,
		Timeout:      180,
	}
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
		MinDurationSec:        240, // 入队时低于此秒数的视频跳过（Short/短视频）
		TTS: &TTSConfig{
			VoiceType:       101008,
			Volume:          5,
			Speed:           1.0,
			PrimaryLanguage: 1,
			SampleRate:      16000,
			Codec:           "mp3",
			Index:           DefaultIndexTTSConfig(),
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
	if dir := os.Getenv("YTB2BILI_DOWNLOAD_DIR"); dir != "" {
		c.DownloadDir = dir
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
	c.Transcriber.Whisper.Model = ExpandHome(c.Transcriber.Whisper.Model)
}

// EffectiveDownloadDir 返回视频下载根目录：显式配置 download_dir 优先，
// 否则回退到 <data_dir>/downloads。
// DefaultCookiesFile 统一的 YouTube cookies 文件名（写入与读取都用它）。
const DefaultCookiesFile = "youtube_cookies_from_meta.txt"

// EffectiveCookiesPath 返回 YouTube cookies 文件路径：
// 显式配置 youtube_cookies 优先（支持 ~/ 展开），否则 <data_dir>/cookies/youtube_cookies_from_meta.txt。
// EffectiveChromeDebugPort 返回 Chrome 远程调试起始端口（默认 9222）。
func (c *Config) EffectiveChromeDebugPort() int {
	if c != nil && c.ChromeDebugPort > 0 {
		return c.ChromeDebugPort
	}
	return 9222
}

func (c *Config) EffectiveCookiesPath() string {
	if c != nil {
		if p := strings.TrimSpace(c.YouTubeCookies); p != "" {
			return ExpandHome(p)
		}
		if dir := strings.TrimSpace(c.DataDir); dir != "" {
			return filepath.Join(dir, "cookies", DefaultCookiesFile)
		}
	}
	return filepath.Join("data", "cookies", DefaultCookiesFile)
}

func (c *Config) EffectiveDownloadDir() string {
	if c != nil {
		if dir := strings.TrimSpace(c.DownloadDir); dir != "" {
			return ExpandHome(dir)
		}
		if dir := strings.TrimSpace(c.DataDir); dir != "" {
			return filepath.Join(dir, "downloads")
		}
	}
	return "data/downloads"
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
