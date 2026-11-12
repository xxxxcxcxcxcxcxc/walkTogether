package walks

import (
	"errors"
	"time"

	"friendwalk/internal/schedule"
)

type TimeSlot struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

func (s *Service) CommonSlots(viewerID, date string, publicIDs []string, durationMinutes int) ([]TimeSlot, error) {
	if durationMinutes < 15 || durationMinutes > 720 {
		return nil, errors.New("duration must be between 15 and 720 minutes")
	}
	if len(publicIDs) == 0 || len(publicIDs) > 20 {
		return nil, errors.New("select between 1 and 20 friends")
	}
	self, err := schedule.NewService(s.db).GetCalendar(viewerID, date, date)
	if err != nil {
		return nil, err
	}
	timezone, err := schedule.NewService(s.db).GetTimezone(viewerID)
	if err != nil {
		return nil, err
	}
	if len(self) != 1 {
		return nil, errors.New("availability date is not available")
	}
	groups := [][]schedule.Range{toUTCRanges(self[0], timezone, date)}
	friends, err := schedule.NewService(s.db).FriendsAvailability(viewerID, date, publicIDs)
	if err != nil {
		return nil, err
	}
	for _, friend := range friends {
		if !friend.Exact {
			return nil, errors.New("exact availability is private; choose a time manually")
		}
		groups = append(groups, toUTCRanges(schedule.Day{State: friend.State, Intervals: friend.Intervals}, friend.Timezone, date))
	}
	common := schedule.FindCommonSlots(groups, time.Duration(durationMinutes)*time.Minute)
	slots := make([]TimeSlot, 0)
	step := 30 * time.Minute
	duration := time.Duration(durationMinutes) * time.Minute
	for _, interval := range common {
		for start := interval.Start; !start.Add(duration).After(interval.End); start = start.Add(step) {
			if start.Before(time.Now()) {
				continue
			}
			slots = append(slots, TimeSlot{Start: start.UTC(), End: start.Add(duration).UTC()})
			if len(slots) >= 32 {
				return slots, nil
			}
		}
	}
	return slots, nil
}

func toUTCRanges(day schedule.Day, timezone, date string) []schedule.Range {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return nil
	}
	parsed, err := time.Parse("2006-01-02", date)
	if err != nil {
		return nil
	}
	startOfDay := time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, location)
	if day.State == schedule.DayFreeAll {
		return []schedule.Range{{Start: startOfDay.UTC(), End: startOfDay.AddDate(0, 0, 1).UTC()}}
	}
	free := make([]schedule.Range, 0)
	busy := make([]schedule.Range, 0)
	for _, item := range day.Intervals {
		startClock, startErr := time.Parse("15:04", item.StartTime)
		endClock, endErr := time.Parse("15:04", item.EndTime)
		if startErr != nil || endErr != nil {
			continue
		}
		start := time.Date(parsed.Year(), parsed.Month(), parsed.Day(), startClock.Hour(), startClock.Minute(), 0, 0, location).UTC()
		end := time.Date(parsed.Year(), parsed.Month(), parsed.Day(), endClock.Hour(), endClock.Minute(), 0, 0, location).UTC()
		rangeValue := schedule.Range{Start: start, End: end}
		if item.Kind == schedule.KindFree {
			free = append(free, rangeValue)
		} else if item.Kind == schedule.KindBusy {
			busy = append(busy, rangeValue)
		}
	}
	for _, block := range busy {
		next := make([]schedule.Range, 0, len(free)+1)
		for _, available := range free {
			if !block.Start.Before(available.End) || !block.End.After(available.Start) {
				next = append(next, available)
				continue
			}
			if block.Start.After(available.Start) {
				next = append(next, schedule.Range{Start: available.Start, End: block.Start})
			}
			if block.End.Before(available.End) {
				next = append(next, schedule.Range{Start: block.End, End: available.End})
			}
		}
		free = next
	}
	if len(free) == 0 && day.State == schedule.DayFreeAll {
		return []schedule.Range{{Start: startOfDay.UTC(), End: startOfDay.AddDate(0, 0, 1).UTC()}}
	}
	return free
}
