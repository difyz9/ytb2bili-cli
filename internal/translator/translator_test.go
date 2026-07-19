package translator

import (
	"strings"
	"testing"
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
	if got := TranslatedSRTPath("/tmp/video.en.srt", "zh"); got != "/tmp/video.en.zh.srt" {
		t.Fatalf("unexpected translated path: %s", got)
	}
	if got := TranslatedSRTPath("/tmp/video.en.zh.srt", "zh"); got != "/tmp/video.en.zh.srt" {
		t.Fatalf("target language was appended twice: %s", got)
	}
	if got := TranslatedSRTPath("/tmp/video.EN.SRT", "zh"); !strings.HasSuffix(got, ".zh.srt") {
		t.Fatalf("unexpected uppercase extension handling: %s", got)
	}
}
