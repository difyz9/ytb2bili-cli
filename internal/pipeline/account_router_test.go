package pipeline

import (
	"testing"

	"github.com/zolagz/ytb2bili-go/internal/config"
)

func TestResolveAccount(t *testing.T) {
	cfg := &config.Config{
		Accounts: []config.AccountConfig{
			{Name: "AI号", TypeRule: []string{"AI", "人工智能", "ChatGPT"}},
			{Name: "副业号", TypeRule: []string{"副业", "赚钱"}},
			{Name: "默认号", IsDefault: true},
		},
	}
	r := &accountRouter{config: cfg}

	cases := []struct {
		title string
		tags  []string
		want  string
	}{
		{"ChatGPT 新功能详解", nil, "AI号"},
		{"人工智能入门教程", []string{"AI"}, "AI号"},
		{"副业赚钱的5种方法", nil, "副业号"},
		{"随便一个视频", []string{"vlog"}, "默认号"},        // 无匹配 → 默认
		{"The Best AI Side Hustles", nil, "AI号"},        // 大小写不敏感 + 英文匹配
		{"如何掌握新版ChatGPT Work", nil, "AI号"},          // 中文匹配
	}
	for _, c := range cases {
		got := r.resolve(c.title, c.tags)
		if got != c.want {
			t.Errorf("resolve(%q) = %q, want %q", c.title, got, c.want)
		}
	}
}

func TestResolveAccountNoConfig(t *testing.T) {
	r := &accountRouter{config: &config.Config{}}
	if got := r.resolve("anything", nil); got != "" {
		t.Errorf("无配置时应返回空, got %q", got)
	}
	r2 := &accountRouter{config: nil}
	if got := r2.resolve("x", nil); got != "" {
		t.Errorf("nil 配置时应返回空, got %q", got)
	}
}

func TestMatchRule(t *testing.T) {
	if !matchRule("how to use chatgpt", []string{"ChatGPT"}) {
		t.Error("应匹配忽略大小写的 ChatGPT")
	}
	if matchRule("how to cook", []string{"AI"}) {
		t.Error("不应误匹配")
	}
	if matchRule("", []string{"   "}) {
		t.Error("空规则应被跳过（返回 false 而非 panic）")
	}
}
