package cli

import (
	"bufio"
	"net"
	"strings"
	"testing"

	"github.com/zolagz/ytb2bili-go/internal/config"
)

func TestMaskSecret(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", "<空>"},
		{"abc", "****"},
		{"sk-1234567890abcdef", "sk-1****cdef"},
		{"  sk-1234567890abcdef  ", "sk-1****cdef"}, // 首尾空白不显示
	}
	for _, c := range cases {
		if got := maskSecret(c.in); got != c.want {
			t.Errorf("maskSecret(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestRedactProxyURL(t *testing.T) {
	raw := "socks5://user:secret123@1.2.3.4:62813"
	got := redactProxyURL(raw)
	if strings.Contains(got, "secret123") {
		t.Errorf("redactProxyURL 泄露密码: %s", got)
	}
	if !strings.Contains(got, "user:****") {
		t.Errorf("redactProxyURL 应保留用户名并掩码密码: %s", got)
	}
	if !strings.Contains(got, "1.2.3.4:62813") {
		t.Errorf("redactProxyURL 应保留 host:port: %s", got)
	}

	plain := "http://1.2.3.4:8080"
	if redactProxyURL(plain) != plain {
		t.Errorf("无凭据代理不应被改写: %s", plain)
	}
}

func TestProxyScheme(t *testing.T) {
	cases := []struct{ in, want string }{
		{"socks5h://u:p@h:1", "socks5h"},
		{"http://h:8080", "http"},
		{"HTTPS://h:8080", "https"},
		{"socks5h://h", "socks5h"},
	}
	for _, c := range cases {
		if got := proxyScheme(c.in); got != c.want {
			t.Errorf("proxyScheme(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestEffectiveLLMConfigFallback 验证 translation.deepseek 段优先于顶层 llm_*。
func TestEffectiveLLMConfigFallback(t *testing.T) {
	cfg := config.Default()
	cfg.Translation = &config.TranslationConfig{}
	cfg.LLMAPIKey = "top-level-key"
	cfg.LLMBaseURL = "https://top.example"
	cfg.LLMModel = "top-model"
	cfg.Translation.DeepSeek = &config.DeepSeekCfg{
		APIKey: "ds-key", BaseURL: "https://ds.example", Model: "ds-model",
	}

	key, base, model := effectiveLLMConfig(cfg)
	if key != "ds-key" || base != "https://ds.example" || model != "ds-model" {
		t.Errorf("deepseek 段未生效: key=%q base=%q model=%q", key, base, model)
	}

	cfg.Translation.DeepSeek = nil
	key, base, model = effectiveLLMConfig(cfg)
	if key != "top-level-key" || base != "https://top.example" || model != "top-model" {
		t.Errorf("应回退顶层配置: key=%q base=%q model=%q", key, base, model)
	}
}

// TestSocks5HandshakeWithMockServer 用本地 mock SOCKS5 代理验证握手（user/pass 认证 + 域名 CONNECT）。
func TestSocks5HandshakeWithMockServer(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	const (
		wantUser = "alice"
		wantPass = "wonderland"
	)

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				br := bufio.NewReader(c)

				// 握手：读取方法表，选择 0x02（user/pass）
				head := make([]byte, 2)
				if _, err := br.Read(head); err != nil || head[0] != 0x05 {
					return
				}
				extra := make([]byte, head[1])
				if _, err := br.Read(extra); err != nil {
					return
				}
				c.Write([]byte{0x05, 0x02})

				// RFC1929 认证
				auth := make([]byte, 2)
				if _, err := br.Read(auth); err != nil {
					return
				}
				user := make([]byte, auth[1])
				if _, err := br.Read(user); err != nil {
					return
				}
				plen := make([]byte, 1)
				if _, err := br.Read(plen); err != nil {
					return
				}
				pass := make([]byte, plen[0])
				if _, err := br.Read(pass); err != nil {
					return
				}
				if string(user) != wantUser || string(pass) != wantPass {
					c.Write([]byte{0x01, 0x01}) // 认证失败
					return
				}
				c.Write([]byte{0x01, 0x00})

				// CONNECT 域名请求：VER CMD RSV ATYP HLEN
				req := make([]byte, 5)
				if _, err := br.Read(req); err != nil {
					return
				}
				host := make([]byte, req[4])
				if _, err := br.Read(host); err != nil {
					return
				}
				port := make([]byte, 2)
				if _, err := br.Read(port); err != nil {
					return
				}
				if req[0] != 0x05 || req[1] != 0x01 || req[3] != 0x03 || string(host) != "www.youtube.com" {
					return
				}
				// 成功：VER REP RSV ATYP(IPv4) + addr + port
				c.Write([]byte{0x05, 0x00, 0x00, 0x01, 0x7f, 0x00, 0x00, 0x01, 0x01, 0xbb})
			}(conn)
		}
	}()

	addr := ln.Addr().String()
	raw := "socks5://" + wantUser + ":" + wantPass + "@" + addr
	if _, err := probeProxy(raw, "www.youtube.com:443"); err != nil {
		t.Fatalf("probeProxy 应成功: %v", err)
	}

	rawBad := "socks5://" + wantUser + ":wrong@" + addr
	if _, err := probeProxy(rawBad, "www.youtube.com:443"); err == nil {
		t.Fatal("错误密码应导致 SOCKS5 认证失败")
	}
}

// TestHTTPProxyConnect 用本地 mock HTTP 代理验证 CONNECT 隧道。
func TestHTTPProxyConnect(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		br := bufio.NewReader(conn)
		line, _ := br.ReadString('\n')
		if !strings.HasPrefix(line, "CONNECT www.youtube.com:443") {
			conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
			return
		}
		for {
			h, err := br.ReadString('\n')
			if err != nil || h == "\r\n" || h == "\n" {
				break
			}
		}
		conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	}()

	raw := "http://user:pass@" + ln.Addr().String()
	if _, err := probeProxy(raw, "www.youtube.com:443"); err != nil {
		t.Fatalf("probeProxy(http CONNECT) 应成功: %v", err)
	}
}

// TestProxyURLValidation 校验协议/端口错误能返回明确错误。
func TestProxyURLValidation(t *testing.T) {
	if _, err := probeProxy("ftp://1.2.3.4:21", "www.youtube.com:443"); err == nil || !strings.Contains(err.Error(), "不支持的代理协议") {
		t.Errorf("ftp 应报不支持协议: %v", err)
	}
	if _, err := probeProxy("socks5://1.2.3.4", "www.youtube.com:443"); err == nil || !strings.Contains(err.Error(), "端口") {
		t.Errorf("缺端口应报错: %v", err)
	}
}
