package cli

// // 自检: check（配置有效性 / LLM / TTS / 代理连通性）

import (
	"bufio"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/zolagz/ytb2bili-go/internal/config"
	"github.com/zolagz/ytb2bili-go/internal/pipeline"
	"github.com/zolagz/ytb2bili-go/internal/resource"
	"github.com/zolagz/ytb2bili-go/internal/translator"
	"github.com/zolagz/ytb2bili-go/internal/tts"
)

func newCheckCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check",
		Short: "检查配置有效性与外部服务可用性（LLM / TTS / 代理）",
		Long: `逐项自检配置与关键外部依赖，定位配置问题与宕机服务：
  - 配置：解析、关键字段完整性（API key 掩码显示、路径存在性等）
  - LLM/DeepSeek：用实际 key 调用一次翻译样例，验证可用性（含 fallbacks）
  - TTS：provider=index → 探测本地 IndexTTS2 /health；provider=tencent → 实际合成一小段语音
  - 代理：校验 youtube_proxy 格式，并真实穿透代理连接 www.youtube.com:443

退出码：全部通过 = 0；任一关键项异常 = 1（未配置的可选项不计失败）。
支持 --json 机器可读输出（stdout 仅 JSON，日志转 stderr）。`,
		RunE: func(cmd *cobra.Command, args []string) error {
			asJSON, _ := cmd.Flags().GetBool("json")
			jsonMode = asJSON
			defer func() { jsonMode = false }()

			cfg := loadConfig()
			fails, warns := 0, 0

			outf("🔎 配置与服务自检\n")
			outf("%s\n", strings.Repeat("=", 40))
			outf("配置来源: %s\n", orDefault(configPath, "<默认配置>"))

			// 状态变量（用于 --json 汇总）
			var (
				cfgRep = configCheckRep{OK: true, Path: orDefault(configPath, "<默认>"), DataDir: cfg.DataDir}
				llmRep = llmCheckRep{OK: true}
				ttsRep = ttsCheckRep{}
				prxRep = proxyCheckRep{Configured: false, OK: true, Target: proxyProbeTarget}
			)
			reporter := func(ok bool, name, detail string) {
				icon := "✅"
				if !ok {
					icon = "❌"
					fails++
				}
				outf("%s %s\n", icon, name)
				if detail != "" {
					outf("       %s\n", detail)
				}
			}

			// ── 1. 配置有效性 ──────────────────────────────────────────────
			outf("\n── 配置 ──\n")
			ok := true
			var cfgIssues []string

			// 目录与关键文件
			if fi, err := os.Stat(cfg.DataDir); err == nil && fi.IsDir() {
				outf("✅ data_dir: %s\n", cfg.DataDir)
			} else {
				outf("⚠ data_dir 不存在: %s（首次运行会自动创建）\n", cfg.DataDir)
				warns++
			}
			cookiesPath := cfg.EffectiveCookiesPath()
			if fi, err := os.Stat(cookiesPath); err == nil && !fi.IsDir() {
				outf("✅ YouTube cookies: %s\n", cookiesPath)
			} else {
				outf("⚠ YouTube cookies 不存在: %s（下载受频率限制时可导入）\n", cookiesPath)
				warns++
			}

			// LLM key
			dsKey, dsBase, dsModel := effectiveLLMConfig(cfg)
			cfgRep.APIKeyConfigured = dsKey != ""
			if dsKey == "" {
				cfgIssues = append(cfgIssues, "未配置 LLM API key（config.yaml 的 llm_api_key / translation.deepseek.api_key，或环境变量 DEEPSEEK_API_KEY）")
				ok = false
				fails++
				outf("❌ 缺少 LLM API key（llm_api_key / translation.deepseek.api_key / DEEPSEEK_API_KEY 均未配置）\n")
			} else {
				outf("✅ LLM key: %s（base=%s, model=%s）\n", maskSecret(dsKey), dsBase, dsModel)
				if dsKey != strings.TrimSpace(dsKey) {
					cfgIssues = append(cfgIssues, "API key 含首尾空白，可能导致认证失败（建议去掉）")
					outf("⚠ API key 含首尾空白，建议去除\n")
					warns++
				}
			}

			// 翻译服务编排（仅提示性校验）
			primary, fallbacks := "deepseek", ""
			if cfg.Translation != nil {
				if cfg.Translation.Primary != "" {
					primary = cfg.Translation.Primary
				}
				if len(cfg.Translation.Fallbacks) > 0 {
					fallbacks = " → " + strings.Join(cfg.Translation.Fallbacks, " → ")
				}
			}
			outf("✅ 翻译编排: primary=%s%s\n", primary, fallbacks)

			// 数据统计目录/技能等只读提示
			if script := resource.SkillScript(filepath.Join("audio-video-sync", "scripts", "synthesize_srt.py")); fileExists(script) {
				outf("✅ 技能资源: %s\n", filepath.Dir(filepath.Dir(filepath.Dir(script))))
			} else {
				outf("⚠ 技能资源缺失: %s\n", script)
				warns++
			}

			cfgRep.OK = ok
			cfgRep.Issues = cfgIssues

			// ── 2. LLM / DeepSeek 可用性 ───────────────────────────────────
			outf("\n── LLM / DeepSeek ──\n")
			llmRep.BaseURL, llmRep.Model = dsBase, dsModel
			llmRep.KeyMasked = maskSecret(dsKey)
			if dsKey == "" {
				llmRep.OK = false
				outf("⏭ 跳过联网测试（未配置 key，见上方 ❌）\n")
			} else {
				results := translator.TestProviders(cfg)
				if len(results) == 0 {
					llmRep.OK = false
					llmRep.Error = "没有可测试的翻译服务商"
					outf("❌ 没有可测试的翻译服务商\n")
					fails++
				}
				for _, r := range results {
					p := providerCheckRep{Name: r.Name, OK: r.OK, LatencyMS: r.Latency.Milliseconds()}
					detail := ""
					if r.OK {
						p.Note = r.Note
						detail = r.Note
					} else {
						p.Error = r.Error
						detail = r.Error
					}
					llmRep.Providers = append(llmRep.Providers, p)
					reporter(r.OK, fmt.Sprintf("%-10s %8s", r.Name, r.Latency.Round(time.Millisecond)), detail)
					if !r.OK {
						llmRep.OK = false
					}
				}
			}

			// ── 3. 语音合成 (TTS) ─────────────────────────────────────────
			outf("\n── 语音合成 (TTS) ──\n")
			provider := pipeline.SelectTTSProvider(cfg)
			ttsRep.Provider = provider
			latency, detail, tErr := probeTTS(cfg, provider)
			if tErr != nil {
				ttsRep.OK = false
				ttsRep.Error = tErr.Error()
				reporter(false, fmt.Sprintf("TTS (provider=%s)", provider), tErr.Error())
			} else {
				ttsRep.OK = true
				ttsRep.LatencyMS = latency.Milliseconds()
				ttsRep.Detail = detail
				reporter(true, fmt.Sprintf("TTS (provider=%s)  %s", provider, latency.Round(time.Millisecond)), detail)
			}

			// ── 4. 代理可用性 ─────────────────────────────────────────────
			outf("\n── 代理 (YouTube 下载专用) ──\n")
			proxyRaw := strings.TrimSpace(cfg.YouTubeProxy)
			if proxyRaw == "" {
				outf("⚠ 未配置 youtube_proxy（跳过）。\n")
				outf("      直连被 YouTube 风控时，配置 socks5/http 代理 + 完整登录态 cookies 即可（仅影响下载，B站/翻译不走代理）\n")
				warns++
			} else {
				prxRep.Configured = true
				prxRep.URL = redactProxyURL(proxyRaw)
				prxRep.Scheme = proxyScheme(proxyRaw)
				lat, err := probeProxy(proxyRaw, proxyProbeTarget)
				prxRep.LatencyMS = lat.Milliseconds()
				if err != nil {
					prxRep.OK = false
					prxRep.Error = err.Error()
					reporter(false, fmt.Sprintf("代理 %s → %s", redactProxyURL(proxyRaw), proxyProbeTarget), err.Error())
				} else {
					prxRep.OK = true
					reporter(true, fmt.Sprintf("代理 %s → %s  %s", redactProxyURL(proxyRaw), proxyProbeTarget, lat.Round(time.Millisecond)),
						"可正常穿透代理访问 YouTube（代理与凭据有效）")
				}
			}

			// ── 汇总 ──────────────────────────────────────────────────────
			outf("\n")
			outf("%s\n", strings.Repeat("=", 40))
			switch {
			case fails == 0 && warns == 0:
				outf("🎉 全部检查通过\n")
			case fails == 0:
				outf("✅ 关键项全部通过（%d 条警告，见上方 ⚠）\n", warns)
			default:
				outf("❌ 检查未通过：%d 项异常、%d 条警告（见上方 ⚠/❌）\n", fails, warns)
			}

			rep := checkRep{
				OK:       fails == 0,
				Config:   cfgRep,
				LLM:      llmRep,
				TTS:      ttsRep,
				Proxy:    prxRep,
				Warnings: warns,
			}
			if asJSON {
				if err := emitJSON(rep); err != nil {
					return err
				}
			}
			if fails > 0 {
				return fmt.Errorf("检查未通过：%d 项异常，请按上方 ❌ 提示修复（修复后重跑 ytb check）", fails)
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "以 JSON 输出结果（stdout 仅含 JSON，日志转 stderr）")
	return cmd
}

// ─── 检查实现辅助 ──────────────────────────────────────────────────────────

// proxyProbeTarget 代理连通性的探测目标（YouTube 下载场景真实目标）。
const proxyProbeTarget = "www.youtube.com:443"

// effectiveLLMConfig 返回实际生效的 LLM 配置（translation.deepseek 段优先，
// 否则回退顶层 llm_* 配置）。key/base/model 均返回非空有效值（可能为空串）。
func effectiveLLMConfig(cfg *config.Config) (key, baseURL, model string) {
	key, baseURL, model = cfg.LLMAPIKey, cfg.LLMBaseURL, cfg.LLMModel
	if cfg.Translation != nil && cfg.Translation.DeepSeek != nil {
		if cfg.Translation.DeepSeek.APIKey != "" {
			key = cfg.Translation.DeepSeek.APIKey
		}
		if cfg.Translation.DeepSeek.BaseURL != "" {
			baseURL = cfg.Translation.DeepSeek.BaseURL
		}
		if cfg.Translation.DeepSeek.Model != "" {
			model = cfg.Translation.DeepSeek.Model
		}
	}
	return key, baseURL, model
}

// maskSecret 掩码显示密钥（保留头尾 4 位）。
func maskSecret(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "<空>"
	}
	rs := []rune(s)
	if len(rs) <= 8 {
		return "****"
	}
	return string(rs[:4]) + "****" + string(rs[len(rs)-4:])
}

func fileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}

// ─── TTS 探针 ──────────────────────────────────────────────────────────────

// probeTTS 按实际生效的 provider 探测语音合成可用性。
// index → 本地 IndexTTS2 /health（含运行环境就绪性）；tencent → 实际合成。
func probeTTS(cfg *config.Config, provider string) (time.Duration, string, error) {
	switch provider {
	case "tencent":
		lat, err := tts.Probe(cfg)
		return lat, "", err
	case "index", "index-tts":
		idxCfg := config.DefaultIndexTTSConfig()
		if cfg.TTS != nil && cfg.TTS.Index != nil {
			idxCfg = cfg.TTS.Index
		}
		apiURL := strings.TrimSpace(idxCfg.APIURL)
		if apiURL == "" {
			apiURL = "http://localhost:18765"
		}

		// 运行环境就绪性：python 脚本 + .venv（缺 .venv 时会回退 PATH 的 python3）
		script := resource.SkillScript(filepath.Join("audio-video-sync", "scripts", "synthesize_srt.py"))
		if !fileExists(script) {
			return 0, "", fmt.Errorf("配音脚本缺失: %s（技能目录未找到）", script)
		}
		venvPython := filepath.Join(resource.ProjectRoot(), ".venv", "bin", "python3")
		if !fileExists(venvPython) {
			return 0, "", fmt.Errorf(".venv 未创建（ytb init --venv 一键安装配音依赖），当前会回退到 PATH 的 python3，可能缺 pysrt/pydub")
		}

		start := time.Now()
		client := &http.Client{Timeout: 8 * time.Second}
		resp, err := client.Get(apiURL + "/health")
		latency := time.Since(start)
		if err != nil {
			return latency, "", fmt.Errorf("无法连接 IndexTTS2 服务 %s: %v（确认已启动: systemctl --user start index-tts）", apiURL, err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		if err != nil {
			return latency, "", fmt.Errorf("读取 /health 响应失败: %v", err)
		}
		var h struct {
			Status      string `json:"status"`
			ModelLoaded any    `json:"model_loaded"`
		}
		if err := json.Unmarshal(body, &h); err != nil {
			return latency, "", fmt.Errorf("解析 /health 响应失败: %v (body: %s)", err, truncate(string(body), 120))
		}
		if h.Status != "ok" {
			return latency, "", fmt.Errorf("IndexTTS2 状态异常: status=%q（body: %s）", h.Status, truncate(string(body), 120))
		}
		// model_loaded 可能为 true/false 或字符串；只有显式 false 视为未就绪
		if b, isBool := h.ModelLoaded.(bool); isBool && !b {
			return latency, "", fmt.Errorf("IndexTTS2 服务已启动但模型未加载（model_loaded=false），无法合成")
		}
		detail := "服务就绪 " + strings.TrimSpace(string(body))
		return latency, detail, nil
	default:
		return 0, "", fmt.Errorf("未知 TTS provider: %q（应为 tencent / index）", provider)
	}
}

// ─── 代理探针（纯 Go，无外部依赖）──────────────────────────────────────────

// probeProxy 校验代理格式并真实穿透代理连接 target（host:port）。
// 支持 http/https（CONNECT）与 socks5/socks5h（含用户名密码认证）。
func probeProxy(raw, target string) (time.Duration, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return 0, fmt.Errorf("代理 URL 解析失败: %v", err)
	}
	scheme := strings.ToLower(u.Scheme)
	switch scheme {
	case "socks5", "socks5h", "http", "https":
	default:
		return 0, fmt.Errorf("不支持的代理协议 %q（支持 socks5/socks5h/http/https）", u.Scheme)
	}
	host := u.Host
	if u.Port() == "" {
		return 0, fmt.Errorf("代理地址需包含端口: %s", u.Host)
	}
	if _, _, err := net.SplitHostPort(host); err != nil {
		host = net.JoinHostPort(u.Hostname(), u.Port())
	}
	if target == "" {
		target = proxyProbeTarget
	}

	start := time.Now()
	d := net.Dialer{Timeout: 10 * time.Second}
	conn, err := d.Dial("tcp", host)
	if err != nil {
		return time.Since(start), fmt.Errorf("无法连接代理 %s: %v", host, err)
	}
	defer conn.Close()
	// 整体握手限时，避免“可连接但不应答”的代理把自检挂死
	conn.SetDeadline(time.Now().Add(15 * time.Second))

	if scheme == "https" {
		tconn := tls.Client(conn, &tls.Config{ServerName: u.Hostname(), MinVersion: tls.VersionTLS12})
		if err := tconn.Handshake(); err != nil {
			return time.Since(start), fmt.Errorf("代理 TLS 握手失败: %v", err)
		}
		conn = tconn
	}

	user, pass := "", ""
	if u.User != nil {
		user = u.User.Username()
		pass, _ = u.User.Password()
	}
	switch scheme {
	case "socks5", "socks5h":
		err = socks5Connect(conn, user, pass, target)
	default:
		err = httpProxyConnect(conn, u, target)
	}
	if err != nil {
		return time.Since(start), err
	}
	return time.Since(start), nil
}

// redactProxyURL 显示代理地址时隐藏密码（手工拼接避免 URL 转义掩码字符）。
func redactProxyURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if u.User == nil || u.Host == "" {
		return raw
	}
	if _, hasPwd := u.User.Password(); hasPwd {
		return u.Scheme + "://" + u.User.Username() + ":****@" + u.Host
	}
	return raw
}

// proxyScheme 返回代理协议（小写，含非法时返回原样小写）。
func proxyScheme(raw string) string {
	if u, err := url.Parse(raw); err == nil {
		return strings.ToLower(u.Scheme)
	}
	return strings.ToLower(strings.SplitN(raw, "://", 2)[0])
}

// socks5Connect 执行 SOCKS5 握手并 CONNECT 到目标（RFC 1928/1929，含域名直连）。
func socks5Connect(conn net.Conn, user, pass, target string) error {
	// 握手：声明支持 no-auth 与 user/pass
	methods := []byte{0x05, 0x01, 0x00}
	if user != "" {
		methods = []byte{0x05, 0x02, 0x00, 0x02}
	}
	if _, err := conn.Write(methods); err != nil {
		return fmt.Errorf("SOCKS5 握手写入失败: %v", err)
	}
	resp := make([]byte, 2)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return fmt.Errorf("SOCKS5 握手读取失败: %v", err)
	}
	if resp[0] != 0x05 {
		return fmt.Errorf("SOCKS5 响应版本错误: %d", resp[0])
	}
	switch resp[1] {
	case 0x00:
		// 无需认证
	case 0x02:
		if _, err := conn.Write(append(append([]byte{0x01, byte(len(user))}, []byte(user)...), byte(len(pass)))); err != nil {
			return fmt.Errorf("SOCKS5 认证写入失败: %v", err)
		}
		if _, err := conn.Write([]byte(pass)); err != nil {
			return fmt.Errorf("SOCKS5 认证写入失败: %v", err)
		}
		auth := make([]byte, 2)
		if _, err := io.ReadFull(conn, auth); err != nil {
			return fmt.Errorf("SOCKS5 认证读取失败: %v", err)
		}
		if auth[1] != 0x00 {
			return fmt.Errorf("SOCKS5 用户名密码认证失败 (code=%d)，请检查代理账号密码", auth[1])
		}
	case 0xFF:
		return fmt.Errorf("代理不接受可用认证方式（no-auth/user-pass）")
	default:
		return fmt.Errorf("代理要求认证方法 0x%02x，当前不支持", resp[1])
	}

	// CONNECT 目标（域名直连，socks5h 语义；多数 socks5 代理同样接受）
	host, portStr, err := net.SplitHostPort(target)
	if err != nil {
		return fmt.Errorf("非法目标 %q: %v", target, err)
	}
	var port int
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
		return fmt.Errorf("非法目标端口 %q: %v", portStr, err)
	}
	req := []byte{0x05, 0x01, 0x00, 0x03, byte(len(host))}
	req = append(req, []byte(host)...)
	req = append(req, byte(port>>8), byte(port))
	if _, err := conn.Write(req); err != nil {
		return fmt.Errorf("SOCKS5 CONNECT 写入失败: %v", err)
	}
	head := make([]byte, 4)
	if _, err := io.ReadFull(conn, head); err != nil {
		return fmt.Errorf("SOCKS5 CONNECT 响应读取失败: %v", err)
	}
	if head[0] != 0x05 {
		return fmt.Errorf("SOCKS5 CONNECT 响应版本错误: %d", head[0])
	}
	if head[1] != 0x00 {
		return fmt.Errorf("SOCKS5 连接目标失败: %s", socks5Status(head[1]))
	}
	var addrLen int
	switch head[3] {
	case 0x01:
		addrLen = 4
	case 0x04:
		addrLen = 16
	case 0x03:
		b := make([]byte, 1)
		if _, err := io.ReadFull(conn, b); err != nil {
			return fmt.Errorf("SOCKS5 读取域名长度失败: %v", err)
		}
		addrLen = int(b[0])
	default:
		return fmt.Errorf("SOCKS5 未知地址类型 0x%02x", head[3])
	}
	if _, err := io.ReadFull(conn, make([]byte, addrLen+2)); err != nil {
		return fmt.Errorf("SOCKS5 读取响应地址失败: %v", err)
	}
	return nil
}

func socks5Status(code byte) string {
	switch code {
	case 0x01:
		return "general failure"
	case 0x02:
		return "connection not allowed"
	case 0x03:
		return "network unreachable"
	case 0x04:
		return "host unreachable"
	case 0x05:
		return "connection refused"
	case 0x06:
		return "TTL expired"
	case 0x07:
		return "command not supported"
	case 0x08:
		return "address type not supported"
	}
	return fmt.Sprintf("未知错误码 0x%02x", code)
}

// httpProxyConnect 通过 HTTP(S) 代理发 CONNECT 隧道（Proxy-Authorization 可选）。
func httpProxyConnect(conn net.Conn, u *url.URL, target string) error {
	req := "CONNECT " + target + " HTTP/1.1\r\nHost: " + target + "\r\n"
	if u.User != nil {
		pw, _ := u.User.Password()
		cred := base64.StdEncoding.EncodeToString([]byte(u.User.Username() + ":" + pw))
		req += "Proxy-Authorization: Basic " + cred + "\r\n"
	}
	req += "\r\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		return fmt.Errorf("代理 CONNECT 写入失败: %v", err)
	}
	br := bufio.NewReader(conn)
	statusLine, err := br.ReadString('\n')
	if err != nil {
		return fmt.Errorf("代理 CONNECT 响应读取失败: %v", err)
	}
	// 读完剩余响应头
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return fmt.Errorf("代理 CONNECT 响应头读取失败: %v", err)
		}
		if line == "\r\n" || line == "\n" {
			break
		}
	}
	if !strings.Contains(statusLine, " 200 ") {
		return fmt.Errorf("代理 CONNECT 被拒绝: %s", strings.TrimSpace(statusLine))
	}
	return nil
}

// ─── JSON 报告结构 ─────────────────────────────────────────────────────────

type checkRep struct {
	OK       bool           `json:"ok"`
	Warnings int            `json:"warnings"`
	Config   configCheckRep `json:"config"`
	LLM      llmCheckRep    `json:"llm"`
	TTS      ttsCheckRep    `json:"tts"`
	Proxy    proxyCheckRep  `json:"proxy"`
}

type configCheckRep struct {
	OK               bool     `json:"ok"`
	Path             string   `json:"path"`
	DataDir          string   `json:"data_dir"`
	APIKeyConfigured bool     `json:"api_key_configured"`
	Issues           []string `json:"issues,omitempty"`
}

type providerCheckRep struct {
	Name      string `json:"name"`
	OK        bool   `json:"ok"`
	LatencyMS int64  `json:"latency_ms,omitempty"`
	Note      string `json:"note,omitempty"`
	Error     string `json:"error,omitempty"`
}

type llmCheckRep struct {
	OK        bool               `json:"ok"`
	BaseURL   string             `json:"base_url"`
	Model     string             `json:"model"`
	KeyMasked string             `json:"key,omitempty"`
	Error     string             `json:"error,omitempty"`
	Providers []providerCheckRep `json:"providers"`
}

type ttsCheckRep struct {
	OK        bool   `json:"ok"`
	Provider  string `json:"provider"`
	LatencyMS int64  `json:"latency_ms,omitempty"`
	Detail    string `json:"detail,omitempty"`
	Error     string `json:"error,omitempty"`
}

type proxyCheckRep struct {
	Configured bool   `json:"configured"`
	OK         bool   `json:"ok"`
	URL        string `json:"url,omitempty"`
	Scheme     string `json:"scheme,omitempty"`
	Target     string `json:"target"`
	LatencyMS  int64  `json:"latency_ms,omitempty"`
	Error      string `json:"error,omitempty"`
}

// truncate 中文友好截断。
func truncate(s string, n int) string {
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return string(rs[:n]) + "..."
}
