package cli

import "testing"

func TestPublishCmdHasFlags(t *testing.T) {
	c := newPublishCmd()
	for _, name := range []string{"title", "desc", "tags", "tid", "cover", "source"} {
		if c.Flags().Lookup(name) == nil {
			t.Fatalf("publish command missing flag --%s", name)
		}
	}
}

func TestReviewCmdHasWaitFlag(t *testing.T) {
	c := newReviewCmd()
	if c.Flags().Lookup("wait") == nil {
		t.Fatal("review command missing flag --wait")
	}
}

func TestSubtitleCmdHasUpload(t *testing.T) {
	c := newSubtitleCmd()
	for _, sub := range c.Commands() {
		if sub.Name() == "upload" {
			return
		}
	}
	t.Fatal("subtitle command missing upload subcommand")
}
