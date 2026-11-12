package schedule

import (
	"testing"
	"time"
)

func TestFindCommonSlotsAcrossParticipants(t *testing.T) {
	base := time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)
	clock := func(hour int) time.Time { return base.Add(time.Duration(hour) * time.Hour) }
	groups := [][]Range{
		{{Start: clock(15), End: clock(20)}},
		{{Start: clock(16), End: clock(19)}},
		{{Start: clock(17), End: clock(21)}},
	}
	slots := FindCommonSlots(groups, 90*time.Minute)
	if len(slots) != 1 {
		t.Fatalf("got %d common ranges, want 1", len(slots))
	}
	if !slots[0].Start.Equal(clock(17)) || !slots[0].End.Equal(clock(19)) {
		t.Fatalf("common range = %s-%s, want 17:00-19:00", slots[0].Start, slots[0].End)
	}
}

func TestFindCommonSlotsUsesEveryRequiredParticipant(t *testing.T) {
	base := time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)
	clock := func(hour int) time.Time { return base.Add(time.Duration(hour) * time.Hour) }
	groups := [][]Range{
		{{Start: clock(10), End: clock(14)}},
		{{Start: clock(15), End: clock(20)}},
	}
	if got := FindCommonSlots(groups, time.Hour); len(got) != 0 {
		t.Fatalf("disjoint availability produced slots: %+v", got)
	}
	if got := FindCommonSlots(nil, time.Hour); len(got) != 0 {
		t.Fatalf("empty participants produced slots: %+v", got)
	}
}

func TestValidateScheduleRanges(t *testing.T) {
	if err := validateTimeRange("10:00", "11:30"); err != nil {
		t.Fatalf("valid same-day interval rejected: %v", err)
	}
	for _, pair := range [][2]string{{"10:00", "10:00"}, {"11:00", "10:00"}, {"25:00", "26:00"}} {
		if err := validateTimeRange(pair[0], pair[1]); err == nil {
			t.Errorf("invalid interval %s-%s accepted", pair[0], pair[1])
		}
	}
	if _, _, err := validateDateRange("2026-11-01", "2026-11-10", 62); err != nil {
		t.Fatalf("valid date range rejected: %v", err)
	}
	if _, _, err := validateDateRange("2026-11-10", "2026-11-01", 62); err == nil {
		t.Fatal("reversed date range accepted")
	}
}
