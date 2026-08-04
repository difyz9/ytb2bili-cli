package channel

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRecordObservation(t *testing.T) {
	m := NewMonitor(t.TempDir())

	t.Run("records and merges within a day", func(t *testing.T) {
		obs1 := []Observation{{VideoID: "a", Views: 100, ObservedAt: time.Now()}}
		if err := m.RecordObservation(obs1); err != nil {
			t.Fatal(err)
		}
		obs2 := []Observation{{VideoID: "a", Views: 200, ObservedAt: time.Now().Add(time.Hour)}}
		if err := m.RecordObservation(obs2); err != nil {
			t.Fatal(err)
		}

		// 同日文件合并为两条观测（同视频不同时刻，供 velocity）
		path := filepath.Join(m.obsDir, time.Now().Format("2006-01-02")+".json")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var all []Observation
		if err := json.Unmarshal(data, &all); err != nil {
			t.Fatal(err)
		}
		if len(all) != 2 {
			t.Fatalf("got %d observations, want 2", len(all))
		}
		if all[0].Views != 100 || all[1].Views != 200 {
			t.Fatalf("views = %d,%d; want 100,200", all[0].Views, all[1].Views)
		}
	})

	t.Run("empty observations no-op", func(t *testing.T) {
		if err := m.RecordObservation(nil); err != nil {
			t.Fatal(err)
		}
	})
}
