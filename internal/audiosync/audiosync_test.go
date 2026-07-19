package audiosync

import (
	"os"
	"strings"
	"testing"
)

func TestScriptPathFindsBundledSkill(t *testing.T) {
	path, err := scriptPath()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(path, "skills/audio-video-sync/scripts/audio_processor_v2.py") {
		t.Fatalf("path=%q", path)
	}
}

func TestConfiguredMissingScriptFails(t *testing.T) {
	t.Setenv("YTB2BILI_AUDIO_SYNC_SCRIPT", t.TempDir()+"/missing.py")
	_, err := scriptPath()
	if err == nil {
		t.Fatal("expected error")
	}
	_ = os.Unsetenv("YTB2BILI_AUDIO_SYNC_SCRIPT")
}
