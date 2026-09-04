package queue

import (
	"strings"
	"testing"
)

// ─── P：失败分类启发式（用真实日志里的错误样本） ────────────────────────────

func TestClassifyErrorSamples(t *testing.T) {
	cases := []struct{ msg, want string }{
		{"ERROR: [youtube] abc: Sign in to confirm you're not a bot. Use --cookies", ClassAuth},
		{"cookies ./x 缺少 SID/SSID", ClassAuth},
		{"腾讯翻译错误 AuthFailure.SecretIdNotFound: The SecretId is not found", ClassAuth},
		{"POST https://... dial tcp 1.2.3.4:443: i/o timeout", ClassNetwork},
		{"分片 1 上传失败: status 504", ClassNetwork},
		{"读取响应时连接被重置: EOF", ClassNetwork},
		{"上传失败: 投稿失败: 网络异常", ClassNetwork},
		{"翻译失败 (重试 3 次): 全部翻译服务失败 (primary=deepseek)", ClassExternal},
		{"ffmpeg 未安装，请先安装", ClassResource},
		{"转写失败: whisper 模型不存在: models/ggml-base.bin", ClassResource},
		{"写 queue.json 失败: permission denied", ClassLocal},
		{"something totally unexpected happened", ClassUnknown},
	}
	for _, c := range cases {
		if got := ClassifyError(c.msg); got != c.want {
			t.Errorf("ClassifyError(%q) = %q，期望 %q", c.msg, got, c.want)
		}
	}
}

// ─── P：状态转移自动落审计事件 ──────────────────────────────────────────────

func TestAuditEventsOnTransitions(t *testing.T) {
	dir := t.TempDir()
	q := New(dir)

	if _, err := q.AddWithRetries("v1", "https://youtu.be/v1", "V", "c", "auto", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Next("daemon:w1"); err != nil {
		t.Fatal(err)
	}
	if err := q.Fail("v1", "Sign in to confirm you're not a bot"); err != nil {
		t.Fatal(err)
	}

	events, err := q.audit.Read()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("期望 3 条事件(queued/claimed/failed)，实际 %d: %+v", len(events), events)
	}
	if events[0].Type != "queued" || events[1].Type != "claimed" || events[2].Type != "failed" {
		t.Fatalf("事件序列错误: %+v", events)
	}
	if events[2].ErrorClass != ClassAuth {
		t.Fatalf("failed 事件应带 auth 分类，实际 %+v", events[2])
	}
	if events[2].RetryCount != 1 || events[2].MaxRetries != 1 {
		t.Fatalf("重试快照错误: %+v", events[2])
	}

	// 完成路径：带 bvid + 耗时
	if _, err := q.Add("v2", "https://youtu.be/v2", "V2", "c", "manual"); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Next("daemon:w1"); err != nil {
		t.Fatal(err)
	}
	if err := q.Complete("v2", "BVabc"); err != nil {
		t.Fatal(err)
	}
	sum, err := q.audit.SummarizeFailures(0)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Queued != 2 || sum.Completed != 1 || sum.Total != 1 {
		t.Fatalf("统计错误: %+v", sum)
	}
	if sum.ByClass[ClassAuth] != 1 {
		t.Fatalf("分类计数错误: %+v", sum.ByClass)
	}
	if !strings.Contains(sum.String(), "auth") {
		t.Fatalf("String 输出缺少分类: %s", sum.String())
	}
}
