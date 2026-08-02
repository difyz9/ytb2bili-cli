package cmd

import (
	"strings"
	"testing"

	"github.com/difyz9/bilibili-go-sdk/bilibili"
	"github.com/zolagz/ytb2bili-go/internal/channel"
	"github.com/zolagz/ytb2bili-go/internal/search"
	"github.com/zolagz/ytb2bili-go/internal/storage"
)

func TestTaskProgress(t *testing.T) {
	task := &storage.Task{Steps: map[string]storage.Step{
		"download":   {Status: "completed"},
		"transcribe": {Status: "completed"},
		"translate":  {Status: "failed"},
		"metadata":   {},
	}}
	done, total := taskProgress(task)
	if done != 2 || total != 4 {
		t.Fatalf("got done=%d total=%d, want 2/4", done, total)
	}
}

func TestTaskProgressEmptySteps(t *testing.T) {
	done, total := taskProgress(&storage.Task{})
	if done != 0 || total != 0 {
		t.Fatalf("got %d/%d, want 0/0", done, total)
	}
}

func TestFormatTaskSummaryUsesRealStepCount(t *testing.T) {
	task := &storage.Task{
		ID:        "abc123",
		Status:    "running",
		SourceURL: "https://www.youtube.com/watch?v=01234567890",
		UpdatedAt: "2026-07-31T19:00:00+08:00",
		Steps: map[string]storage.Step{
			"download":   {Status: "completed"},
			"transcribe": {Status: "running"},
			"translate":  {},
			"metadata":   {},
			"upload":     {},
			"tts":        {},
			"audio-sync": {},
		},
	}
	got := formatTaskSummary(task)
	if !strings.Contains(got, "[1/7]") {
		t.Fatalf("summary does not show real step count, got %q", got)
	}
	if !strings.Contains(got, "abc123") {
		t.Fatalf("summary missing task id, got %q", got)
	}
}

func TestFormatTaskDetail(t *testing.T) {
	task := &storage.Task{
		ID:        "abc",
		Status:    "failed",
		SourceURL: "https://example.com/v",
		Title:     "A Title",
		BVID:      "BV1xx",
		CreatedAt: "2026-07-31T19:00:00+08:00",
		UpdatedAt: "2026-07-31T19:05:00+08:00",
		Steps: map[string]storage.Step{
			"download": {Status: "completed"},
			"upload":   {Status: "failed", Error: "超时"},
		},
	}
	got := formatTaskDetail(task)
	for _, want := range []string{"abc", "A Title", "BV1xx", "download", "upload", "超时"} {
		if !strings.Contains(got, want) {
			t.Fatalf("detail missing %q:\n%s", want, got)
		}
	}
}

func TestSearchFilter(t *testing.T) {
	t.Run("empty returns nil", func(t *testing.T) {
		if got := searchFilter("", "", ""); got != nil {
			t.Fatalf("got %+v, want nil", got)
		}
	})
	t.Run("populates fields", func(t *testing.T) {
		got := searchFilter("view_count", "this_week", "long")
		if got.SortBy != "view_count" || got.UploadDate != "this_week" || got.Duration != "long" {
			t.Fatalf("got %+v", got)
		}
	})
}

func TestFormatSearchResultsMarksSubmitted(t *testing.T) {
	result := &search.SearchResult{
		Query: "flutter",
		Videos: []search.Video{
			{ID: "aaa", Title: "One", Channel: "ChanA", Duration: "10:00", Views: "1M", PublishTime: "2026-01-01", URL: "https://youtu.be/aaa"},
			{ID: "bbb", Title: "Two", Channel: "ChanB", Duration: "5:00", Views: "2K", PublishTime: "2026-02-01", URL: "https://youtu.be/bbb"},
		},
	}
	got := formatSearchResults(result, map[string]bool{"aaa": true})
	if !strings.Contains(got, "One ✅已提交") {
		t.Fatalf("missing submitted marker:\n%s", got)
	}
	if strings.Contains(got, "Two ✅已提交") {
		t.Fatalf("unexpected submitted marker:\n%s", got)
	}
	if !strings.Contains(got, "找到 2 个视频") {
		t.Fatalf("missing count line:\n%s", got)
	}
}

func TestRenderHistory(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if got := renderHistory(nil); !strings.Contains(got, "暂无提交历史") {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("lists entries", func(t *testing.T) {
		videos := []storage.SubmittedVideo{
			{Title: "Vid A", BVID: "BV1", YouTubeID: "abc", SubmittedAt: "2026-07-31T19:00:00+08:00"},
		}
		got := renderHistory(videos)
		for _, want := range []string{"Vid A", "BV1", "abc"} {
			if !strings.Contains(got, want) {
				t.Fatalf("history missing %q:\n%s", want, got)
			}
		}
	})
}

func TestRenderDiscoveredVideos(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if got := renderDiscoveredVideos(nil); !strings.Contains(got, "暂无发现视频") {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("lists videos", func(t *testing.T) {
		videos := []channel.DiscoveredVideo{
			{VideoID: "xyz", Title: "New Upload", URL: "https://youtu.be/xyz", Status: "new", PublishedAt: "2026-07-31T19:00:00+08:00"},
		}
		got := renderDiscoveredVideos(videos)
		for _, want := range []string{"New Upload", "[new]", "xyz"} {
			if !strings.Contains(got, want) {
				t.Fatalf("missing %q:\n%s", want, got)
			}
		}
	})
}

func TestSearchCmdHasNewFlags(t *testing.T) {
	c := newSearchCmd()
	for _, name := range []string{"sort", "upload-date", "duration", "json", "history", "submit", "max"} {
		if c.Flags().Lookup(name) == nil {
			t.Fatalf("search command missing flag --%s", name)
		}
	}
}

func TestChannelCmdHasSyncAndVideos(t *testing.T) {
	c := newChannelCmd()
	names := map[string]bool{}
	for _, sub := range c.Commands() {
		names[sub.Name()] = true
	}
	if !names["sync"] {
		t.Fatal("channel command missing sync subcommand")
	}
	if !names["videos"] {
		t.Fatal("channel command missing videos subcommand")
	}
}

func TestTaskCmdHasShow(t *testing.T) {
	c := newTaskCmd()
	for _, sub := range c.Commands() {
		if sub.Name() == "show" {
			return
		}
	}
	t.Fatal("task command missing show subcommand")
}

func TestRenderReviewStatus(t *testing.T) {
	t.Run("passed", func(t *testing.T) {
		got := renderReviewStatus(&bilibili.VideoReviewStatus{BVid: "BV1xx", Title: "T", Passed: true, State: 0})
		for _, want := range []string{"BV1xx", "审核通过"} {
			if !strings.Contains(got, want) {
				t.Fatalf("missing %q:\n%s", want, got)
			}
		}
	})

	t.Run("rejected with reason", func(t *testing.T) {
		got := renderReviewStatus(&bilibili.VideoReviewStatus{BVid: "BV1xx", Rejected: true, RejectReason: "标题违规", State: -2})
		for _, want := range []string{"被驳回", "标题违规"} {
			if !strings.Contains(got, want) {
				t.Fatalf("missing %q:\n%s", want, got)
			}
		}
	})

	t.Run("reviewing", func(t *testing.T) {
		got := renderReviewStatus(&bilibili.VideoReviewStatus{BVid: "BV1xx", Reviewing: true, State: 1})
		if !strings.Contains(got, "审核中") {
			t.Fatalf("missing 审核中:\n%s", got)
		}
	})

	t.Run("unknown state uses StateDesc", func(t *testing.T) {
		got := renderReviewStatus(&bilibili.VideoReviewStatus{BVid: "BV1xx", State: 123, StateDesc: "锁定"})
		if !strings.Contains(got, "锁定") {
			t.Fatalf("missing StateDesc:\n%s", got)
		}
	})
}
