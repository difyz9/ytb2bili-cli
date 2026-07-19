package translator

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestDeduplicateRollingEntries(t *testing.T) {
	entries := []SRTEntry{
		{Index: 1, TimeCode: "00:00:00,000 --> 00:00:02,000", Text: "hello world"},
		{Index: 2, TimeCode: "00:00:02,000 --> 00:00:04,000", Text: "hello world\nnew phrase"},
		{Index: 3, TimeCode: "00:00:04,000 --> 00:00:06,000", Text: "new phrase"},
	}
	got := DeduplicateRollingEntries(entries)
	if len(got) != 2 || got[0].Text != "hello world" || got[1].Text != "new phrase" {
		t.Fatalf("unexpected rolling-caption cleanup: %#v", got)
	}
	if got[0].Index != 1 || got[1].Index != 2 {
		t.Fatalf("indices were not normalized: %#v", got)
	}
}

func TestTranslateTextsSkipsSameLanguage(t *testing.T) {
	translator := New(Config{SourceLang: "zh-CN", TargetLang: "zh-Hans"})
	input := []string{"第一句", "第二句"}
	result, err := translator.TranslateTexts(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !result.SkippedTranslation || result.DetectedLanguage != "zh-CN" {
		t.Fatalf("unexpected skip result: %#v", result)
	}
	if len(result.TranslatedTexts) != len(input) || result.TranslatedTexts[1] != input[1] {
		t.Fatalf("unexpected copied translations: %#v", result.TranslatedTexts)
	}
}

func TestRetryStopsWhenContextIsCancelled(t *testing.T) {
	translator := New(Config{
		APIKey: "unused", BaseURL: "http://127.0.0.1:1", RetryCount: 5,
		SourceLang: "en", TargetLang: "zh",
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	_, err := translator.translateGroupWithRetry(ctx, []string{"hello"}, nil, nil)
	if err == nil || time.Since(started) > time.Second {
		t.Fatalf("cancelled retry returned err=%v after %v", err, time.Since(started))
	}
}

func TestDeduplicateTranslations(t *testing.T) {
	entries := []SRTEntry{{Index: 4}, {Index: 5}, {Index: 6}}
	gotEntries, gotTexts := DeduplicateTranslations(entries, []string{"你好", " 你好 ", "下一句"})
	if len(gotEntries) != 2 || len(gotTexts) != 2 {
		t.Fatalf("got %d entries and %d texts", len(gotEntries), len(gotTexts))
	}
	if gotTexts[0] != "你好" || gotTexts[1] != "下一句" {
		t.Fatalf("unexpected texts: %#v", gotTexts)
	}
	if gotEntries[0].Index != 1 || gotEntries[1].Index != 2 {
		t.Fatalf("indices were not normalized: %#v", gotEntries)
	}
}

func TestTranslatedSRTPath(t *testing.T) {
	if got := TranslatedSRTPath("/tmp/video.en.srt", "zh-Hans"); got != "/tmp/video.zh-Hans.srt" {
		t.Fatalf("unexpected translated path: %s", got)
	}
	if got := TranslatedSRTPath("/tmp/video.zh-Hans.srt", "zh-Hans"); got != "/tmp/video.zh-Hans.srt" {
		t.Fatalf("target language was appended twice: %s", got)
	}
	if got := TranslatedSRTPath("/tmp/video.EN.SRT", "zh-Hans"); !strings.HasSuffix(got, "/video.zh-Hans.srt") {
		t.Fatalf("unexpected uppercase extension handling: %s", got)
	}
	if got := TranslatedSRTPath("/tmp/video.en.cleaned.srt", "zh-Hans"); got != "/tmp/video.zh-Hans.srt" {
		t.Fatalf("cleaned source suffix was retained: %s", got)
	}
}
