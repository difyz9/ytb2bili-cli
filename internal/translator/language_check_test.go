package translator

import (
	"os"
	"path/filepath"
	"testing"
)

// ─── looksLikeTargetLanguage ─────────────────────────────────

func TestLooksLikeTargetLanguage(t *testing.T) {
	enLines := []string{
		"I started a weekly AI newsletter and I have zero employees.",
		"No writers, no editors, no social media person and no operations.",
		"Just me and a team of AI agents that handle researching.",
	}
	zhLines := []string{
		"我用AI智能体开了一人公司：0员工运营2.7万订阅newsletter全流程",
		"本视频将为你提供一份完整指南，教你如何在自有硬件上微调大语言模型。",
		"在这里，我会教你如何挑选合适的LLM，根据你自己的数据进行微调。",
	}
	mixedLines := []string{
		"我用AI智能体开了一人公司",
		"I started a weekly AI newsletter",
		"在这里，我会教你如何挑选合适的LLM",
	}

	cases := []struct {
		name   string
		texts  []string
		target string
		want   bool
	}{
		{"英文原文+zh目标 → 不通过", enLines, "zh-Hans", false},
		{"中文+zh目标 → 通过", zhLines, "zh-Hans", true},
		{"混合+zh目标 → 通过", mixedLines, "zh-Hans", true},
		{"英文原文+en目标 → 通过（不校验）", enLines, "en", true},
		{"空输入 → 不通过", []string{}, "zh-Hans", false},
		{"zh-Hant 同族", zhLines, "zh-Hant", true},
		{"cn 别名", zhLines, "cn", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := looksLikeTargetLanguage(c.texts, c.target); got != c.want {
				t.Errorf("looksLikeTargetLanguage(%v, %q) = %v, want %v", c.texts, c.target, got, c.want)
			}
		})
	}
}

// ─── ValidateSRTFile ─────────────────────────────────────────

func TestValidateSRTFile(t *testing.T) {
	dir := t.TempDir()

	// 英文"假翻译"文件（历史 bug 场景）
	enSRT := "1\n00:00:00,000 --> 00:00:04,760\nI started a weekly AI newsletter and I have zero employees.\n\n" +
		"2\n00:00:04,760 --> 00:00:09,360\nNo writers, no editors, no social media person.\n"
	enPath := filepath.Join(dir, "video.zh-Hans.srt")
	if err := os.WriteFile(enPath, []byte(enSRT), 0644); err != nil {
		t.Fatal(err)
	}

	// 正常中文翻译文件
	zhSRT := "1\n00:00:00,000 --> 00:00:04,760\n我用AI智能体开了一人公司：0员工运营2.7万订阅newsletter全流程\n\n" +
		"2\n00:00:04,760 --> 00:00:09,360\n在这里，我会教你如何挑选合适的LLM。\n"
	zhPath := filepath.Join(dir, "video2.zh-Hans.srt")
	if err := os.WriteFile(zhPath, []byte(zhSRT), 0644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		path   string
		target string
		want   bool
	}{
		{"英文假翻译 → 拒绝", enPath, "zh-Hans", false},
		{"正常中文 → 通过", zhPath, "zh-Hans", true},
		{"文件不存在 → 拒绝", filepath.Join(dir, "nope.srt"), "zh-Hans", false},
		{"en 目标不校验 → 通过", enPath, "en", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ValidateSRTFile(c.path, c.target); got != c.want {
				t.Errorf("ValidateSRTFile(%q, %q) = %v, want %v", c.path, c.target, got, c.want)
			}
		})
	}
}
