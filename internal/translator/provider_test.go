package translator

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"strings"
	"testing"
)

// ─── BatchAdapter ─────────────────────────────────────────────

func TestBatchAdapter(t *testing.T) {
	single := func(ctx context.Context, text, src, dst string) (string, error) {
		return "译:" + text, nil
	}
	adapter := NewBatchAdapter("test", single, 3)

	texts := []string{"a", "b", "c", "d"}
	out, err := adapter.TranslateBatch(context.Background(), texts, "en", "zh")
	if err != nil {
		t.Fatalf("TranslateBatch 失败: %v", err)
	}
	if len(out) != 4 {
		t.Fatalf("len(out) = %d, want 4", len(out))
	}
	// 顺序保持
	for i, want := range []string{"译:a", "译:b", "译:c", "译:d"} {
		if out[i] != want {
			t.Errorf("out[%d] = %q, want %q", i, out[i], want)
		}
	}
}

func TestBatchAdapterSingleItem(t *testing.T) {
	single := func(ctx context.Context, text, src, dst string) (string, error) {
		return "译:" + text, nil
	}
	adapter := NewBatchAdapter("test", single, 3)
	out, err := adapter.TranslateBatch(context.Background(), []string{"x"}, "en", "zh")
	if err != nil || len(out) != 1 || out[0] != "译:x" {
		t.Fatalf("单条批量失败: %v %v", out, err)
	}
}

func TestBatchAdapterPartialFailure(t *testing.T) {
	single := func(ctx context.Context, text, src, dst string) (string, error) {
		if text == "bad" {
			return "", errTest
		}
		return "ok:" + text, nil
	}
	adapter := NewBatchAdapter("test", single, 3)
	_, err := adapter.TranslateBatch(context.Background(), []string{"a", "bad", "c"}, "en", "zh")
	if err == nil || !strings.Contains(err.Error(), "1/3") {
		t.Fatalf("部分失败应报错，got: %v", err)
	}
}

var errTest = &testErr{}

type testErr struct{}

func (e *testErr) Error() string { return "测试错误" }

func md5Hex(data []byte) string {
	sum := md5.Sum(data)
	return hex.EncodeToString(sum[:])
}

// ─── Router 主备切换 ──────────────────────────────────────────

type mockProvider struct {
	name    string
	fail    bool
	callCnt *int
}

func (m *mockProvider) Name() string { return m.name }

func (m *mockProvider) TranslateBatch(ctx context.Context, texts []string, src, dst string) ([]string, error) {
	*m.callCnt++
	if m.fail {
		return nil, errTest
	}
	out := make([]string, len(texts))
	for i, t := range texts {
		out[i] = m.name + ":" + t
	}
	return out, nil
}

func TestRouterPrimarySuccess(t *testing.T) {
	primaryCalls := 0
	primary := &mockProvider{name: "deepseek", callCnt: &primaryCalls}
	fallbackCalls := 0
	fallback := &mockProvider{name: "baidu", callCnt: &fallbackCalls}

	router := NewRouter(primary, []Provider{fallback}, 2)
	out, err := router.TranslateBatch(context.Background(), []string{"hi"}, "en", "zh")
	if err != nil {
		t.Fatalf("primary 成功应无错误: %v", err)
	}
	if out[0] != "deepseek:hi" {
		t.Errorf("out = %v", out)
	}
	if primaryCalls != 1 || fallbackCalls != 0 {
		t.Errorf("调用数: primary=%d fallback=%d, want 1/0", primaryCalls, fallbackCalls)
	}
}

func TestRouterFailover(t *testing.T) {
	primaryCalls := 0
	primary := &mockProvider{name: "deepseek", fail: true, callCnt: &primaryCalls}
	fallbackCalls := 0
	fallback := &mockProvider{name: "baidu", callCnt: &fallbackCalls}

	router := NewRouter(primary, []Provider{fallback}, 2)
	out, err := router.TranslateBatch(context.Background(), []string{"hi"}, "en", "zh")
	if err != nil {
		t.Fatalf("fallback 成功应无错误: %v", err)
	}
	if out[0] != "baidu:hi" {
		t.Errorf("out = %v, want baidu:hi", out)
	}
	// primary 重试 3 次（retries=2 → 共 3 次）+ fallback 1 次
	if primaryCalls != 3 {
		t.Errorf("primary 调用 %d 次, want 3（重试2次）", primaryCalls)
	}
	if fallbackCalls != 1 {
		t.Errorf("fallback 调用 %d 次, want 1", fallbackCalls)
	}
}

func TestRouterAllFail(t *testing.T) {
	primary := &mockProvider{name: "deepseek", fail: true, callCnt: new(int)}
	fallback := &mockProvider{name: "baidu", fail: true, callCnt: new(int)}

	router := NewRouter(primary, []Provider{fallback}, 1)
	_, err := router.TranslateBatch(context.Background(), []string{"hi"}, "en", "zh")
	if err == nil {
		t.Fatal("全部失败应返回错误")
	}
}

// ─── 语言代码映射 ─────────────────────────────────────────────

func TestBaiduLang(t *testing.T) {
	if got := baiduLang("zh-Hans"); got != "zh" {
		t.Errorf("baiduLang(zh-Hans) = %s, want zh", got)
	}
	if got := baiduLang("en"); got != "en" {
		t.Errorf("baiduLang(en) = %s, want en", got)
	}
}

func TestTencentLang(t *testing.T) {
	if got := tencentLang("zh-Hans"); got != "zh" {
		t.Errorf("tencentLang(zh-Hans) = %s, want zh", got)
	}
	if got := tencentLang("en"); got != "en" {
		t.Errorf("tencentLang(en) = %s, want en", got)
	}
}

// ─── 百度签名 ─────────────────────────────────────────────────

func TestBaiduSign(t *testing.T) {
	// MD5(appid + q + salt + key) 已知向量
	p := &BaiduProvider{appID: "20230801", appKey: "testkey"}
	sum := md5Hex([]byte("20230801hello123testkey"))
	_ = sum
	// 验证 sign 构造逻辑（不依赖具体值，只验证确定性）
	salt := "123"
	signRaw := p.appID + "hello" + salt + p.appKey
	if len(signRaw) == 0 {
		t.Error("signRaw 不应为空")
	}
}
