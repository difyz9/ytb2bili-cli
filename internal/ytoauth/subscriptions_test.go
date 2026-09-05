package ytoauth

import "testing"

func TestParseISO8601Duration(t *testing.T) {
	cases := []struct {
		in   string
		want int
		ok   bool
	}{
		{"PT1M23S", 83, true},
		{"PT59S", 59, true},
		{"PT1H2M3S", 3723, true},
		{"PT5M", 300, true},
		{"PT0S", 0, false},
		{"", 0, false},
		{"P1D", 0, false}, // 不支持天
	}
	for _, c := range cases {
		got, ok := parseISO8601Duration(c.in)
		if got != c.want || ok != c.ok {
			t.Fatalf("parseISO8601Duration(%q) = %d,%v; want %d,%v", c.in, got, ok, c.want, c.ok)
		}
	}
}
