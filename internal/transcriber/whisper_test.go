package transcriber

import "testing"

func TestParseWhisperConsoleOutput(t *testing.T) {
	t.Run("parses segment lines", func(t *testing.T) {
		out := "[00:00:00.000 --> 00:00:10.500]   And so my fellow Americans\n" +
			"[00:00:10.500 --> 00:00:11.000]   ask what you can do\n"
		segments := parseWhisperConsoleOutput(out)
		if len(segments) != 2 {
			t.Fatalf("got %d segments, want 2", len(segments))
		}
		s := segments[0]
		if s.StartTime != 0 || s.EndTime != 10.5 {
			t.Fatalf("segment[0] times = %f→%f, want 0→10.5", s.StartTime, s.EndTime)
		}
		if s.Transcript != "And so my fellow Americans" {
			t.Fatalf("segment[0] text = %q", s.Transcript)
		}
	})

	t.Run("skips non-segment lines", func(t *testing.T) {
		out := "whisper_print_timings: total time = 129.30 ms\n" +
			"[00:00:00.000 --> 00:00:01.000]   hello\n" +
			"ggml_metal_free: deallocating\n"
		segments := parseWhisperConsoleOutput(out)
		if len(segments) != 1 {
			t.Fatalf("got %d segments, want 1", len(segments))
		}
	})

	t.Run("empty output", func(t *testing.T) {
		if segments := parseWhisperConsoleOutput(""); len(segments) != 0 {
			t.Fatalf("got %d segments, want 0", len(segments))
		}
	})
}

func TestTimestampToSeconds(t *testing.T) {
	cases := []struct {
		h, m, s, ms string
		want        float64
	}{
		{"00", "00", "00", "000", 0},
		{"00", "00", "01", "500", 1.5},
		{"01", "02", "03", "250", 3723.25},
	}
	for _, c := range cases {
		if got := timestampToSeconds(c.h, c.m, c.s, c.ms); got != c.want {
			t.Fatalf("timestampToSeconds(%s,%s,%s,%s) = %v, want %v", c.h, c.m, c.s, c.ms, got, c.want)
		}
	}
}
