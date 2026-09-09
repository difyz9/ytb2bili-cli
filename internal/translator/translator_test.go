package translator

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
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

func TestTranslateSRTFileDeduplicatesConsecutiveDuplicates(t *testing.T) {
	directory := t.TempDir()
	inputPath := filepath.Join(directory, "video.zh.srt")
	outputPath := filepath.Join(directory, "video.zh-Hans.srt")
	input := `1
00:00:00,000 --> 00:00:01,000
重复字幕

2
00:00:01,000 --> 00:00:02,000
重复字幕

3
00:00:02,000 --> 00:00:03,000
下一条字幕

`
	if err := os.WriteFile(inputPath, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}
	engine := New(Config{SourceLang: "zh", TargetLang: "zh-Hans"})
	if err := engine.TranslateSRTFile(context.Background(), inputPath, outputPath); err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := ParseSRT(string(written))
	if err != nil {
		t.Fatal(err)
	}
	// 连续重复字幕被去重（保留首条时间码），条目重新编号
	if len(entries) != 2 {
		t.Fatalf("translated entries=%d, want 2", len(entries))
	}
	if entries[0].Index != 1 || entries[0].TimeCode != "00:00:00,000 --> 00:00:01,000" || entries[0].Text != "重复字幕" {
		t.Fatalf("first entry changed: %#v", entries[0])
	}
	if entries[1].Index != 2 || entries[1].TimeCode != "00:00:02,000 --> 00:00:03,000" || entries[1].Text != "下一条字幕" {
		t.Fatalf("second entry changed: %#v", entries[1])
	}
}

func TestTranslateSRTFileWritesPartialOnGroupFailure(t *testing.T) {
	directory := t.TempDir()
	inputPath := filepath.Join(directory, "video.en.srt")
	outputPath := filepath.Join(directory, "video.zh-Hans.srt")
	input := `1
00:00:00,000 --> 00:00:01,000
good one

2
00:00:01,000 --> 00:00:02,000
good two

3
00:00:02,000 --> 00:00:03,000
bad three

4
00:00:03,000 --> 00:00:04,000
good four

`
	if err := os.WriteFile(inputPath, []byte(input), 0644); err != nil {
		t.Fatal(err)
	}

	provider := failingTextProvider{failOn: "bad"}
	engine := New(Config{
		SourceLang: "en", TargetLang: "zh-Hans",
		BatchSize: 2, MaxWorkers: 1, RetryCount: 0,
		Router: NewRouter(provider, nil, 0),
	})
	engine.client = newLLMTestClient(`{"needs_translation":true,"detected_language":"en","reason":"test"}`)

	err := engine.TranslateSRTFile(context.Background(), inputPath, outputPath)
	if err == nil || !strings.Contains(err.Error(), "已保存部分翻译") {
		t.Fatalf("expected partial-save error, got %v", err)
	}
	if _, statErr := os.Stat(outputPath); !os.IsNotExist(statErr) {
		t.Fatalf("final output should not be written on failure, stat err=%v", statErr)
	}
	partialPath := filepath.Join(directory, "video.zh-Hans.partial.srt")
	written, readErr := os.ReadFile(partialPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	content := string(written)
	if !strings.Contains(content, "zh:good one") || !strings.Contains(content, "zh:good two") {
		t.Fatalf("partial output lost successful translations:\n%s", content)
	}
	if !strings.Contains(content, "[未翻译] bad three") || !strings.Contains(content, "[未翻译] good four") {
		t.Fatalf("partial output did not mark untranslated entries:\n%s", content)
	}
}

type failingTextProvider struct{ failOn string }

func (f failingTextProvider) Name() string { return "test-provider" }

func (f failingTextProvider) TranslateBatch(_ context.Context, texts []string, _, _ string) ([]string, error) {
	for _, text := range texts {
		if strings.Contains(text, f.failOn) {
			return nil, fmt.Errorf("forced failure for %q", text)
		}
	}
	out := make([]string, len(texts))
	for i, text := range texts {
		out[i] = "zh:" + text
	}
	return out, nil
}

func TestTranslationPlanProjectsRollingLinesBackToEveryCue(t *testing.T) {
	entries := []SRTEntry{
		{Index: 1, Text: "This is Pencil"},
		{Index: 2, Text: "This is Pencil\na design tool"},
		{Index: 3, Text: "a design tool"},
	}
	plan, err := buildTranslationPlan(entries)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.units) != 2 || plan.units[0] != "This is Pencil" || plan.units[1] != "a design tool" {
		t.Fatalf("unexpected semantic units: %#v", plan.units)
	}
	projected, err := plan.project([]string{"这是 Pencil", "一款设计工具"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"这是 Pencil", "这是 Pencil\n一款设计工具", "一款设计工具"}
	for i := range want {
		if projected[i] != want[i] {
			t.Fatalf("projected[%d]=%q, want %q", i, projected[i], want[i])
		}
	}
}

func TestTranslationPlanNormalizesEquivalentRollingLines(t *testing.T) {
	entries := []SRTEntry{
		{Index: 1, Text: "same   rolling line"},
		{Index: 2, Text: " same rolling line \nnext line"},
	}
	plan, err := buildTranslationPlan(entries)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.units) != 2 || plan.entryUnits[0][0] != plan.entryUnits[1][0] {
		t.Fatalf("equivalent lines did not share translation memory: %#v", plan)
	}
}

func TestParseSRTPreservesCueWithBlankLineBeforeText(t *testing.T) {
	content := `1
00:00:00,160 --> 00:00:02,869

This is the first subtitle.

2
00:00:02,869 --> 00:00:03,000
Second subtitle.
`
	entries, err := ParseSRT(content)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("parsed entries=%d, want 2: %#v", len(entries), entries)
	}
	if entries[0].Text != "This is the first subtitle." {
		t.Fatalf("first subtitle text=%q", entries[0].Text)
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

func TestShouldTranslateUsesLanguageDecision(t *testing.T) {
	translator := New(Config{
		APIKey: "test", BaseURL: "http://llm.test", Model: "test-model",
		SourceLang: "auto", TargetLang: "zh-Hans",
	})
	translator.client = newLLMTestClient("```json\n{\"needs_translation\":false,\"detected_language\":\"zh\",\"reason\":\"主体为中文\"}\n```")
	result, err := translator.TranslateTexts(context.Background(), []string{"这已经是中文字幕。"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.SkippedTranslation || result.DetectedLanguage != "zh" {
		t.Fatalf("unexpected language decision result: %#v", result)
	}
}

func TestTranslateGroupRejectsCountMismatch(t *testing.T) {
	translator := New(Config{
		APIKey: "test", BaseURL: "http://llm.test", Model: "test-model",
		SourceLang: "en", TargetLang: "zh-Hans",
	})
	translator.client = newLLMTestClient("第一句")
	_, err := translator.translateGroup(context.Background(), []string{"one", "two"}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "当前批次包含 2 条") {
		t.Fatalf("expected count mismatch, got %v", err)
	}
}

func TestTranslateGroupParsesIndexedJSONWithoutMergingDuplicates(t *testing.T) {
	translator := New(Config{
		APIKey: "test", BaseURL: "http://llm.test", Model: "test-model",
		SourceLang: "en", TargetLang: "zh-Hans",
	})
	translator.client = newLLMTestClient(`{"translations":[{"index":1,"text":"重复字幕"},{"index":2,"text":"重复字幕"}]}`)
	translated, err := translator.translateGroup(context.Background(), []string{"same", "same"}, []string{"before"}, []string{"after"})
	if err != nil {
		t.Fatal(err)
	}
	if len(translated) != 2 || translated[0] != "重复字幕" || translated[1] != "重复字幕" {
		t.Fatalf("duplicate subtitles were not preserved: %#v", translated)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func newLLMTestClient(content string) *http.Client {
	body := `{"choices":[{"message":{"content":` + strconv.Quote(content) + `}}]}`
	return &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})}
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

func TestMergeShortEntriesProducesValidTimeCodes(t *testing.T) {
	entries := []SRTEntry{
		{Index: 1, TimeCode: "00:00:00,000 --> 00:00:00,500", Text: "Hello"},
		{Index: 2, TimeCode: "00:00:00,500 --> 00:00:02,000", Text: "world."},
		{Index: 3, TimeCode: "00:00:02,000 --> 00:00:05,000", Text: "Next sentence."},
	}
	got := MergeShortEntries(entries)
	if len(got) != 2 {
		t.Fatalf("merged entries=%d, want 2", len(got))
	}
	if got[0].TimeCode != "00:00:00,000 --> 00:00:02,000" {
		t.Fatalf("first time code=%q", got[0].TimeCode)
	}
	if got[1].TimeCode != "00:00:02,000 --> 00:00:05,000" {
		t.Fatalf("second time code=%q", got[1].TimeCode)
	}
	parsed, err := ParseSRT(GenerateSRT(got, []string{"你好，世界。", "下一句。"}))
	if err != nil || len(parsed) != 2 {
		t.Fatalf("generated SRT is invalid: entries=%d err=%v", len(parsed), err)
	}
}
