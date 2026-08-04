package metadata

import "testing"

func TestParseMetaResponse(t *testing.T) {
	fallback := &VideoMeta{Title: "FB", Tags: []string{}}

	t.Run("parses plain JSON", func(t *testing.T) {
		got := parseMetaResponse(`{"title":"视频标题","description":"简介","tags":["a","b"]}`, fallback)
		if got.Title != "视频标题" || got.Description != "简介" || len(got.Tags) != 2 {
			t.Fatalf("got %+v", got)
		}
	})

	t.Run("strips markdown fences", func(t *testing.T) {
		resp := "```json\n{\"title\":\"T\",\"tags\":[\"x\"]}\n```"
		got := parseMetaResponse(resp, fallback)
		if got.Title != "T" {
			t.Fatalf("got %+v", got)
		}
	})

	t.Run("strips leading prose", func(t *testing.T) {
		resp := "Here you go:\n{\"title\":\"T2\"}"
		got := parseMetaResponse(resp, fallback)
		if got.Title != "T2" {
			t.Fatalf("got %+v", got)
		}
	})

	t.Run("falls back on garbage", func(t *testing.T) {
		got := parseMetaResponse("no json here", fallback)
		if got != fallback || got.Title != "FB" {
			t.Fatalf("got %+v", got)
		}
	})

	t.Run("fills empty title from fallback", func(t *testing.T) {
		got := parseMetaResponse(`{"title":"","tags":[]}`, fallback)
		if got.Title != "FB" {
			t.Fatalf("got %+v", got)
		}
	})

	t.Run("defaults nil tags to empty slice", func(t *testing.T) {
		got := parseMetaResponse(`{"title":"T"}`, fallback)
		if got.Tags == nil {
			t.Fatal("tags should be non-nil empty slice")
		}
	})

	t.Run("truncates title over 80 chars", func(t *testing.T) {
		long := ""
		for i := 0; i < 100; i++ {
			long += "字"
		}
		got := parseMetaResponse(`{"title":"`+long+`","tags":[]}`, fallback)
		if len([]rune(got.Title)) != 80 {
			t.Fatalf("title length = %d, want 80", len([]rune(got.Title)))
		}
	})

	t.Run("keeps title at or under 80 chars", func(t *testing.T) {
		if got := ClampTitle("短标题"); got != "短标题" {
			t.Fatalf("got %q", got)
		}
		if got := ClampTitle("标题标题标题标题标题标题标题标题标题标题标题标题标题标题标题标题标题标题标题标题标题标题标题标题标题标题标题标题标题标题标题标题标题标题标题标题标题标题标题标题标题标题"); len([]rune(got)) != 80 {
			t.Fatalf("length = %d, want 80", len([]rune(got)))
		}
	})
}
