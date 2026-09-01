package config

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/zolagz/ytb2bili-go/internal/queue"
)

// envPlaceholderPattern 匹配 ${VAR} 形式的环境变量占位符（仅这种形式，避免误伤 $ 符号）。
var envPlaceholderPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// expandEnvPlaceholders 展开配置值中的 ${ENV_VAR} 占位符。
// 未定义的环境变量替换为空字符串（调用方会 fallback 到默认值）。
// 例如: api_key: "${DEEPSEEK_API_KEY}" → api_key: "sk-xxx..."（或空）
func expandEnvPlaceholders(data []byte) []byte {
	return envPlaceholderPattern.ReplaceAllFunc(data, func(m []byte) []byte {
		name := envPlaceholderPattern.FindSubmatch(m)[1]
		if val := os.Getenv(string(name)); val != "" {
			return []byte(val)
		}
		return []byte("")
	})
}

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
	SkillsDir   string `yaml:"skills_dir"`   // 技能资源目录（默认自动探测项目根下 skills/，可指定绝对/相对路径）
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

	// 多翻译服务配置（Phase 1）
	Translation *TranslationConfig `yaml:"translation"`

	// YouTube OAuth 授权配置
	YouTubeOAuth *YouTubeOAuthConfig `yaml:"youtube_oauth"`

	// 转录后端配置
	Transcriber *TranscriberConfig `yaml:"transcriber"`

	// 多 B站账号配置：按稿件类型路由投稿账号
	Accounts []AccountConfig `yaml:"accounts"`

	// 自主搜索调度配置（auto / daemon 共用，单一来源）
	Search *SearchConfig `yaml:"search"`

	// daemon 守护进程调度配置
	Daemon *DaemonConfig `yaml:"daemon"`
}

// AccountConfig 单个 B站账号的路由规则
type AccountConfig struct {
	// Name 账号标识（对应 data/cookies/accounts/<name>.json 凭证文件）
	Name string `yaml:"name"`
	// TypeRule 稿件类型关键词列表：标题/标签命中任意关键词则投稿到该账号
	// 例如: ["AI", "人工智能", "教程"]。空则作为默认账号（兜底）。
	TypeRule []string `yaml:"type_rule,omitempty"`
	// IsDefault 是否为默认账号（未匹配任何规则时使用）。多个为 true 时取第一个。
	IsDefault bool `yaml:"is_default,omitempty"`
}

// SearchConfig 自主搜索调度参数（auto / daemon 共用单一来源，替代脚本内硬编码）
type SearchConfig struct {
	// Keywords 搜索关键词列表（daemon 轮换使用；auto 无参数时也读取）
	Keywords []string `yaml:"keywords,omitempty"`
	// Scorer 评分策略: popular / fresh / balanced / nowcast
	Scorer string `yaml:"scorer,omitempty"`
	// UploadDate 上传日期过滤: last_hour / today / this_week / this_month / this_year
	UploadDate string `yaml:"upload_date,omitempty"`
	// MaxDuration 最大视频时长（分钟），0=不限
	MaxDuration int `yaml:"max_duration,omitempty"`
	// MaxVideos 每批最多处理视频数
	MaxVideos int `yaml:"max_videos,omitempty"`
	// MinViews 最低播放量过滤
	MinViews int `yaml:"min_views,omitempty"`
	// Duration 时长档位过滤: short(<4m) / medium(4-20m) / long(>20m)
	Duration string `yaml:"duration,omitempty"`
	// SkipTranslate 跳过翻译
	SkipTranslate bool `yaml:"skip_translate,omitempty"`
}

// DaemonConfig ytb daemon 守护进程调度配置
type DaemonConfig struct {
	// IntervalSec 批间休息秒数（默认 60）
	IntervalSec int `yaml:"interval_sec,omitempty"`
	// MaxBatches 最大批次数（0=无限，默认 0）
	MaxBatches int `yaml:"max_batches,omitempty"`
	// ConsumePerBatch 每批消费排队任务上限（默认 2）
	ConsumePerBatch int `yaml:"consume_per_batch,omitempty"`
	// MaxRetries 失败任务自动重试上限（默认 3，达上限后停止并告警）
	MaxRetries int `yaml:"max_retries,omitempty"`
	// StepTimeoutSec 单步骤超时秒数（下载 1800 / TTS 3600 等），超时 kill 重试
	StepTimeoutSec map[string]int `yaml:"step_timeout_sec,omitempty"`
	// TaskTimeoutSec 单任务总超时秒数（0=不限制，默认 0）
	TaskTimeoutSec int `yaml:"task_timeout_sec,omitempty"`
	// HeartbeatFile 心跳文件路径（相对 data_dir，默认 daemon/heartbeat.json）
	HeartbeatFile string `yaml:"heartbeat_file,omitempty"`
	// AlertWebhook 飞书自定义机器人 webhook（失败/心跳告警用，空=不告警）
	AlertWebhook string `yaml:"alert_webhook,omitempty"`
	// AlertFailed 失败任务达上限后是否飞书告警（默认 true）
	AlertFailed *bool `yaml:"alert_failed,omitempty"`
}

// EffectiveConsumePerBatch 返回每批消费上限（默认 2）
func (d *DaemonConfig) EffectiveConsumePerBatch() int {
	if d == nil || d.ConsumePerBatch <= 0 {
		return 2
	}
	return d.ConsumePerBatch
}

// EffectiveInterval 返回批间休息秒数（默认 60）
func (d *DaemonConfig) EffectiveInterval() int {
	if d == nil || d.IntervalSec <= 0 {
		return 60
	}
	return d.IntervalSec
}

// EffectiveMaxRetries 返回失败重试上限（默认 3）
func (d *DaemonConfig) EffectiveMaxRetries() int {
	if d == nil || d.MaxRetries <= 0 {
		return queue.DefaultMaxRetries
	}
	return d.MaxRetries
}

// EffectiveHeartbeatFile 返回心跳文件路径（相对 data_dir，默认 daemon/heartbeat.json）
func (d *DaemonConfig) EffectiveHeartbeatFile() string {
	if d == nil || strings.TrimSpace(d.HeartbeatFile) == "" {
		return filepath.Join("daemon", "heartbeat.json")
	}
	return d.HeartbeatFile
}

// StepTimeout 返回某步骤超时（秒），未配置返回 0（不限制）
func (d *DaemonConfig) StepTimeout(step string) int {
	if d == nil || d.StepTimeoutSec == nil {
		return 0
	}
	return d.StepTimeoutSec[step]
}

// ShouldAlertFailed 是否对最终失败任务发飞书告警（默认 true）
func (d *DaemonConfig) ShouldAlertFailed() bool {
	if d == nil || d.AlertFailed == nil {
		return true
	}
	return *d.AlertFailed
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

// TranslationConfig 多翻译服务配置（Phase 1）
// primary 主服务，fallbacks 降级顺序。deepseek 为默认（兼容现有 llm_* 配置）。
type TranslationConfig struct {
	Primary   string   `yaml:"primary"`             // 主服务: deepseek / baidu / tencent / ollama
	Fallbacks []string `yaml:"fallbacks"`           // 降级顺序（空=不降级）
	Retries   int      `yaml:"retries"`             // 主服务重试次数（默认 2）
	// DeepSeek LLM 翻译
	DeepSeek *DeepSeekCfg `yaml:"deepseek"`
	// 百度翻译
	Baidu *BaiduCfg `yaml:"baidu"`
	// 腾讯云翻译（默认复用 tencent_cloud 凭证）
	Tencent *TencentCfg `yaml:"tencent"`
	// 本地 Ollama 翻译（零成本兜底）
	Ollama *OllamaCfg `yaml:"ollama"`
}

// OllamaCfg 本地 Ollama 配置
type OllamaCfg struct {
	BaseURL string `yaml:"base_url"`
	Model   string `yaml:"model"`
	Batch   int    `yaml:"batch_size"`
}

// DeepSeekCfg DeepSeek LLM 翻译配置
type DeepSeekCfg struct {
	APIKey      string `yaml:"api_key"`
	BaseURL     string `yaml:"base_url"`
	Model       string `yaml:"model"`
	BatchSize   int    `yaml:"batch_size"`
	ContextSize int    `yaml:"context_size"`
}

// BaiduCfg 百度翻译配置
type BaiduCfg struct {
	AppID    string `yaml:"app_id"`
	AppKey   string `yaml:"app_key"`
	QPS      int    `yaml:"qps"`
}

// TencentCfg 腾讯翻译配置（为空时复用 tencent_cloud 凭证）
type TencentCfg struct {
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
	// EmotionAlpha 情感强度 0-1（默认 0.2）
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
		EmotionAlpha: 0.2,
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
		Search: &SearchConfig{
			Scorer:      "nowcast",
			UploadDate:  "this_month",
			MaxDuration: 40,
			MaxVideos:   5,
		},
		Daemon: &DaemonConfig{
			IntervalSec:     60,
			ConsumePerBatch: 2,
			MaxRetries:      queue.DefaultMaxRetries,
			StepTimeoutSec: map[string]int{
				"download": 1800, // 30 分钟
				"tts":      3600, // 60 分钟
			},
			HeartbeatFile: filepath.Join("daemon", "heartbeat.json"),
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

	// 多翻译服务配置（环境变量覆盖，优先级: 环境变量 > config.yaml）
	if c.Translation == nil {
		c.Translation = &TranslationConfig{}
	}
	if primary := os.Getenv("TRANSLATION_PRIMARY"); primary != "" {
		c.Translation.Primary = primary
	}
	if fallbacks := os.Getenv("TRANSLATION_FALLBACKS"); fallbacks != "" {
		c.Translation.Fallbacks = nil
		for _, name := range strings.Split(fallbacks, ",") {
			if name = strings.TrimSpace(name); name != "" {
				c.Translation.Fallbacks = append(c.Translation.Fallbacks, name)
			}
		}
	}
	if retries := os.Getenv("TRANSLATION_RETRIES"); retries != "" {
		if n, err := strconv.Atoi(retries); err == nil {
			c.Translation.Retries = n
		}
	}
	if c.Translation.DeepSeek == nil {
		c.Translation.DeepSeek = &DeepSeekCfg{}
	}
	if c.Translation.Ollama == nil {
		c.Translation.Ollama = &OllamaCfg{}
	}
	if base := os.Getenv("OLLAMA_BASE_URL"); base != "" {
		c.Translation.Ollama.BaseURL = base
	}
	if model := os.Getenv("OLLAMA_MODEL"); model != "" {
		c.Translation.Ollama.Model = model
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

	// 自主搜索调度配置（daemon / auto 共用）
	if c.Search == nil {
		c.Search = &SearchConfig{}
	}
	if strings.TrimSpace(c.Search.Scorer) == "" {
		c.Search.Scorer = "nowcast"
	}
	if c.Search.MaxVideos <= 0 {
		c.Search.MaxVideos = 5
	}

	// daemon 守护进程配置
	if c.Daemon == nil {
		c.Daemon = &DaemonConfig{}
	}
	if c.Daemon.StepTimeoutSec == nil {
		c.Daemon.StepTimeoutSec = map[string]int{"download": 1800, "tts": 3600}
	}
	if strings.TrimSpace(c.Daemon.HeartbeatFile) == "" {
		c.Daemon.HeartbeatFile = filepath.Join("daemon", "heartbeat.json")
	}
	if c.Daemon.AlertWebhook == "" {
		c.Daemon.AlertWebhook = os.Getenv("YTB2BILI_ALERT_WEBHOOK")
	}
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
	// 展开 ${ENV_VAR} 占位符（未定义变量→空，调用方 fallback）
	data = expandEnvPlaceholders(data)
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	cfg.Init()
	return cfg, nil
}
