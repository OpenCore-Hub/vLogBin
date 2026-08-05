package service

import (
	"testing"
	"time"
)

func TestFillDaily(t *testing.T) {
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	values := map[string]int64{
		"2026-07-01": 100,
		"2026-07-03": 250,
	}

	got := fillDaily(start, values)

	if len(got) != trendDays {
		t.Fatalf("len = %d, want %d", len(got), trendDays)
	}

	tests := []struct {
		index int
		date  string
		value int64
	}{
		{0, "2026-07-01", 100}, // 有值
		{1, "2026-07-02", 0},   // 缺失日补零
		{2, "2026-07-03", 250}, // 有值
		{3, "2026-07-04", 0},   // 缺失日补零
	}
	for _, tt := range tests {
		if got[tt.index].Date != tt.date {
			t.Errorf("got[%d].Date = %q, want %q", tt.index, got[tt.index].Date, tt.date)
		}
		if got[tt.index].Value != tt.value {
			t.Errorf("got[%d].Value = %d, want %d", tt.index, got[tt.index].Value, tt.value)
		}
	}

	// 连续递增的 UTC 日轴，无缝隙
	for i := 1; i < len(got); i++ {
		prev, err := time.Parse("2006-01-02", got[i-1].Date)
		if err != nil {
			t.Fatalf("bad date %q: %v", got[i-1].Date, err)
		}
		cur, err := time.Parse("2006-01-02", got[i].Date)
		if err != nil {
			t.Fatalf("bad date %q: %v", got[i].Date, err)
		}
		if cur.Sub(prev) != 24*time.Hour {
			t.Fatalf("gap at %d: %s -> %s", i, got[i-1].Date, got[i].Date)
		}
	}

	// 末位应为窗口最后一天
	wantLast := start.AddDate(0, 0, trendDays-1).Format("2006-01-02")
	if got[len(got)-1].Date != wantLast {
		t.Errorf("last date = %q, want %q", got[len(got)-1].Date, wantLast)
	}
}

func TestFillDailyEmptyValues(t *testing.T) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	got := fillDaily(start, nil)

	if len(got) != trendDays {
		t.Fatalf("len = %d, want %d", len(got), trendDays)
	}
	for i, p := range got {
		if p.Value != 0 {
			t.Errorf("got[%d].Value = %d, want 0", i, p.Value)
		}
	}
}
