package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/zolagz/ytb2bili-go/internal/config"
)

func TestExtractYouTubeID(t *testing.T) {
	for _, tc := range []struct{ url, want string }{
		{"https://www.youtube.com/watch?v=dQw4w9WgXcQ&list=x", "dQw4w9WgXcQ"},
		{"https://youtu.be/dQw4w9WgXcQ?t=1", "dQw4w9WgXcQ"},
		{"https://www.youtube.com/shorts/dQw4w9WgXcQ", "dQw4w9WgXcQ"},
		{"https://example.com/video", ""},
	} {
		if got := ExtractYouTubeID(tc.url); got != tc.want {
			t.Errorf("ExtractYouTubeID(%q)=%q want %q", tc.url, got, tc.want)
		}
	}
}

func TestPlanOnlyExpandsDependenciesWithoutLeavingTask(t *testing.T) {
	dataDir := t.TempDir()
	result, err := (&Processor{Config: &config.Config{DataDir: dataDir}}).Process(context.Background(), Request{
		URL: "https://youtu.be/dQw4w9WgXcQ", Chain: []string{"translate"}, PlanOnly: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"download", "transcribe", "translate"}
	if !reflect.DeepEqual(result.Plan, want) {
		t.Fatalf("plan=%v want=%v", result.Plan, want)
	}
	entries, err := os.ReadDir(filepath.Join(dataDir, "tasks"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("plan-only left task files: %v", entries)
	}
}

func TestCallerProvidedTaskIDIsPreserved(t *testing.T) {
	dataDir := t.TempDir()
	result, err := (&Processor{Config: &config.Config{DataDir: dataDir}}).Process(context.Background(), Request{
		URL: "https://youtu.be/dQw4w9WgXcQ", TaskID: "api-task-1", Chain: []string{"download"}, PlanOnly: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.TaskID != "api-task-1" {
		t.Fatalf("task id=%q", result.TaskID)
	}
}

func TestAudioSyncPlanExpandsTranslationDependencies(t *testing.T) {
	result, err := (&Processor{Config: &config.Config{DataDir: t.TempDir()}}).Process(context.Background(), Request{
		URL: "https://youtu.be/dQw4w9WgXcQ", Chain: []string{"audio-sync"}, PlanOnly: true, AudioDir: "/tmp/voice",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"download", "transcribe", "translate", "audio-sync"}
	if !reflect.DeepEqual(result.Plan, want) {
		t.Fatalf("plan=%v want=%v", result.Plan, want)
	}
}

func TestAgentPlannerRequiresGoal(t *testing.T) {
	_, err := (&Processor{Config: &config.Config{DataDir: t.TempDir(), LLMAPIKey: "unused"}}).Process(context.Background(), Request{
		URL: "https://youtu.be/dQw4w9WgXcQ", Planner: "agent", PlanOnly: true,
	})
	if err == nil {
		t.Fatal("expected missing goal error")
	}
}

func TestUnknownPlannerDoesNotLeaveTask(t *testing.T) {
	dataDir := t.TempDir()
	_, err := (&Processor{Config: &config.Config{DataDir: dataDir}}).Process(context.Background(), Request{
		URL: "https://youtu.be/dQw4w9WgXcQ", Planner: "magic", PlanOnly: true,
	})
	if err == nil {
		t.Fatal("expected unknown planner error")
	}
	if _, statErr := os.Stat(filepath.Join(dataDir, "tasks")); !os.IsNotExist(statErr) {
		t.Fatalf("unknown planner created task directory: %v", statErr)
	}
}
