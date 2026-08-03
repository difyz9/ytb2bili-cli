package queue

import "testing"

func TestRemove(t *testing.T) {
	q := New(t.TempDir())
	q.Add("abc", "https://youtu.be/abc", "A", "chan", "manual")
	q.Add("def", "https://youtu.be/def", "B", "chan", "manual")

	if err := q.Remove("abc"); err != nil {
		t.Fatal(err)
	}
	data, err := q.Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Videos) != 1 || data.Videos[0].VideoID != "def" {
		t.Fatalf("after remove, videos=%+v", data.Videos)
	}

	if err := q.Remove("missing"); err == nil {
		t.Fatal("expected error for missing video")
	}
}

func TestClear(t *testing.T) {
	q := New(t.TempDir())
	q.Add("abc", "https://youtu.be/abc", "A", "chan", "manual")
	q.Add("def", "https://youtu.be/def", "B", "chan", "manual")

	if err := q.Clear(); err != nil {
		t.Fatal(err)
	}
	data, err := q.Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Videos) != 0 {
		t.Fatalf("after clear, videos=%+v", data.Videos)
	}
}
