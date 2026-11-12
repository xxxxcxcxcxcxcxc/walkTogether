package schedule

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	DayUnknown = "unknown"
	DayFreeAll = "free_all"
	DayBusyAll = "busy_all"
	DayCustom  = "custom"
	KindFree   = "free"
	KindBusy   = "busy"
)

type Service struct {
	db *sql.DB
}

func NewService(connection *sql.DB) *Service {
	return &Service{db: connection}
}

type Interval struct {
	ID        string `json:"id,omitempty"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
	Kind      string `json:"kind"`
}

type Day struct {
	Date      string     `json:"date"`
	State     string     `json:"state"`
	Intervals []Interval `json:"intervals"`
}

type Rule struct {
	ID        string  `json:"id"`
	DayOfWeek int     `json:"day_of_week"`
	StartTime string  `json:"start_time"`
	EndTime   string  `json:"end_time"`
	Kind      string  `json:"kind"`
	StartsOn  string  `json:"starts_on"`
	EndsOn    *string `json:"ends_on,omitempty"`
	Timezone  string  `json:"timezone"`
	IsActive  bool    `json:"is_active"`
}

type FriendDay struct {
	PublicID    string     `json:"public_id"`
	DisplayName string     `json:"display_name"`
	Date        string     `json:"date"`
	State       string     `json:"state"`
	Exact       bool       `json:"exact"`
	Intervals   []Interval `json:"intervals,omitempty"`
	Timezone    string     `json:"timezone,omitempty"`
}

type Range struct {
	Start time.Time
	End   time.Time
}

func (s *Service) GetCalendar(userID string, from string, to string) ([]Day, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("schedule service unavailable")
	}
	fromDate, toDate, err := validateDateRange(from, to, 62)
	if err != nil {
		return nil, err
	}

	overrides := make(map[string]Day)
	rows, err := s.db.Query(`
		SELECT day, data_state FROM availability
		WHERE user_id = $1 AND day BETWEEN $2 AND $3
	`, userID, fromDate.Format("2006-01-02"), toDate.Format("2006-01-02"))
	if err != nil {
		return nil, fmt.Errorf("load availability days: %w", err)
	}
	for rows.Next() {
		var date time.Time
		var state string
		if err = rows.Scan(&date, &state); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan availability day: %w", err)
		}
		key := date.Format("2006-01-02")
		overrides[key] = Day{Date: key, State: state, Intervals: []Interval{}}
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterate availability days: %w", err)
	}
	_ = rows.Close()

	intervalRows, err := s.db.Query(`
		SELECT day, id, starts_at, ends_at, kind
		FROM availability_intervals
		WHERE user_id = $1 AND day BETWEEN $2 AND $3
		ORDER BY day, starts_at
	`, userID, fromDate.Format("2006-01-02"), toDate.Format("2006-01-02"))
	if err != nil {
		return nil, fmt.Errorf("load availability intervals: %w", err)
	}
	for intervalRows.Next() {
		var date time.Time
		var interval Interval
		var start time.Time
		var end time.Time
		if err = intervalRows.Scan(&date, &interval.ID, &start, &end, &interval.Kind); err != nil {
			_ = intervalRows.Close()
			return nil, fmt.Errorf("scan availability interval: %w", err)
		}
		key := date.Format("2006-01-02")
		day := overrides[key]
		day.Intervals = append(day.Intervals, Interval{ID: interval.ID, StartTime: start.Format("15:04"), EndTime: end.Format("15:04"), Kind: interval.Kind})
		overrides[key] = day
	}
	if err = intervalRows.Err(); err != nil {
		_ = intervalRows.Close()
		return nil, fmt.Errorf("iterate availability intervals: %w", err)
	}
	_ = intervalRows.Close()

	rules, err := s.listRulesInRange(userID, fromDate, toDate)
	if err != nil {
		return nil, err
	}

	days := make([]Day, 0, int(toDate.Sub(fromDate).Hours()/24)+1)
	for date := fromDate; !date.After(toDate); date = date.AddDate(0, 0, 1) {
		key := date.Format("2006-01-02")
		if day, exists := overrides[key]; exists {
			days = append(days, day)
			continue
		}
		day := Day{Date: key, State: DayUnknown, Intervals: []Interval{}}
		for _, rule := range rules {
			if rule.DayOfWeek == int(date.Weekday()) && !date.Before(parseISODate(rule.StartsOn)) && (rule.EndsOn == nil || !date.After(parseISODate(*rule.EndsOn))) {
				day.Intervals = append(day.Intervals, Interval{StartTime: rule.StartTime, EndTime: rule.EndTime, Kind: rule.Kind})
			}
		}
		if len(day.Intervals) > 0 {
			day.State = DayCustom
		}
		days = append(days, day)
	}
	return days, nil
}

func (s *Service) SetDayState(userID string, date string, state string) error {
	if state != DayUnknown && state != DayFreeAll && state != DayBusyAll && state != DayCustom {
		return errors.New("state must be unknown, free_all, busy_all, or custom")
	}
	if _, err := parseDate(date); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin availability update: %w", err)
	}
	defer tx.Rollback()
	if err = lockScheduleUser(tx, userID); err != nil {
		return err
	}
	if state == DayUnknown {
		if _, err = tx.Exec(`DELETE FROM availability WHERE user_id = $1 AND day = $2`, userID, date); err != nil {
			return fmt.Errorf("clear availability override: %w", err)
		}
	} else {
		if _, err = tx.Exec(`
			INSERT INTO availability (user_id, day, is_available, data_state)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (user_id, day) DO UPDATE
			SET is_available = EXCLUDED.is_available, data_state = EXCLUDED.data_state, updated_at = NOW()
		`, userID, date, state == DayFreeAll, state); err != nil {
			return fmt.Errorf("set availability day: %w", err)
		}
		if state != DayCustom {
			if _, err = tx.Exec(`DELETE FROM availability_intervals WHERE user_id = $1 AND day = $2`, userID, date); err != nil {
				return fmt.Errorf("clear day intervals: %w", err)
			}
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit availability update: %w", err)
	}
	return nil
}

func (s *Service) AddDayInterval(userID string, date string, startTime string, endTime string, kind string) (*Interval, error) {
	if kind != KindFree && kind != KindBusy {
		return nil, errors.New("interval kind must be free or busy")
	}
	if err := validateTimeRange(startTime, endTime); err != nil {
		return nil, err
	}
	if _, err := parseDate(date); err != nil {
		return nil, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin interval creation: %w", err)
	}
	defer tx.Rollback()
	if err = lockScheduleUser(tx, userID); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(`
		INSERT INTO availability (user_id, day, is_available, data_state)
		VALUES ($1, $2, TRUE, 'custom')
		ON CONFLICT (user_id, day) DO UPDATE SET data_state = 'custom', updated_at = NOW()
	`, userID, date); err != nil {
		return nil, fmt.Errorf("prepare custom day: %w", err)
	}
	var interval Interval
	var start time.Time
	var end time.Time
	err = tx.QueryRow(`
		INSERT INTO availability_intervals (user_id, day, starts_at, ends_at, kind)
		SELECT $1, $2, $3::time, $4::time, $5
		WHERE NOT EXISTS (
			SELECT 1 FROM availability_intervals
			WHERE user_id = $1 AND day = $2 AND starts_at < $4::time AND ends_at > $3::time
		)
		RETURNING id, starts_at, ends_at, kind
	`, userID, date, startTime, endTime, kind).Scan(&interval.ID, &start, &end, &interval.Kind)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("availability intervals must not overlap")
	}
	if err != nil {
		return nil, fmt.Errorf("create availability interval: %w", err)
	}
	interval.StartTime = start.Format("15:04")
	interval.EndTime = end.Format("15:04")
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit interval creation: %w", err)
	}
	return &interval, nil
}

func (s *Service) DeleteDayInterval(userID string, intervalID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin interval deletion: %w", err)
	}
	defer tx.Rollback()
	if err = lockScheduleUser(tx, userID); err != nil {
		return err
	}
	var date time.Time
	result, err := tx.Exec(`DELETE FROM availability_intervals WHERE id = $1 AND user_id = $2`, intervalID, userID)
	if err != nil {
		return fmt.Errorf("delete availability interval: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check deleted interval: %w", err)
	}
	if count != 1 {
		return sql.ErrNoRows
	}
	_ = date
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit interval deletion: %w", err)
	}
	return nil
}

func (s *Service) UpdateDayInterval(userID, intervalID, startTime, endTime, kind string) (*Interval, error) {
	if kind != KindFree && kind != KindBusy {
		return nil, errors.New("interval kind must be free or busy")
	}
	if err := validateTimeRange(startTime, endTime); err != nil {
		return nil, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin interval update: %w", err)
	}
	defer tx.Rollback()
	if err = lockScheduleUser(tx, userID); err != nil {
		return nil, err
	}
	var interval Interval
	var start, end time.Time
	err = tx.QueryRow(`
		UPDATE availability_intervals AS current
		SET starts_at=$3::time, ends_at=$4::time, kind=$5, updated_at=NOW()
		WHERE current.id=$1 AND current.user_id=$2
		  AND NOT EXISTS (
			SELECT 1 FROM availability_intervals other
			WHERE other.user_id=current.user_id AND other.day=current.day AND other.id<>current.id
			  AND other.starts_at<$4::time AND other.ends_at>$3::time
		  )
		RETURNING current.id,current.starts_at,current.ends_at,current.kind
	`, intervalID, userID, startTime, endTime, kind).Scan(&interval.ID, &start, &end, &interval.Kind)
	if errors.Is(err, sql.ErrNoRows) {
		var exists bool
		if checkErr := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM availability_intervals WHERE id=$1 AND user_id=$2)`, intervalID, userID).Scan(&exists); checkErr != nil {
			return nil, checkErr
		}
		if exists {
			return nil, errors.New("availability intervals must not overlap")
		}
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, fmt.Errorf("update availability interval: %w", err)
	}
	interval.StartTime = start.Format("15:04")
	interval.EndTime = end.Format("15:04")
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit interval update: %w", err)
	}
	return &interval, nil
}

func (s *Service) ListRules(userID string) ([]Rule, error) {
	rows, err := s.db.Query(`
		SELECT id, day_of_week, start_time, end_time, kind, starts_on, ends_on, timezone, is_active
		FROM availability_rules WHERE user_id = $1 ORDER BY day_of_week, start_time
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list availability rules: %w", err)
	}
	defer rows.Close()
	rules := make([]Rule, 0)
	for rows.Next() {
		var rule Rule
		var start time.Time
		var end time.Time
		var startsOn time.Time
		var endsOn sql.NullTime
		if err = rows.Scan(&rule.ID, &rule.DayOfWeek, &start, &end, &rule.Kind, &startsOn, &endsOn, &rule.Timezone, &rule.IsActive); err != nil {
			return nil, fmt.Errorf("scan availability rule: %w", err)
		}
		rule.StartTime = start.Format("15:04")
		rule.EndTime = end.Format("15:04")
		rule.StartsOn = startsOn.Format("2006-01-02")
		if endsOn.Valid {
			value := endsOn.Time.Format("2006-01-02")
			rule.EndsOn = &value
		}
		rules = append(rules, rule)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate availability rules: %w", err)
	}
	return rules, nil
}

func (s *Service) AddRule(userID string, rule Rule) (*Rule, error) {
	if rule.DayOfWeek < 0 || rule.DayOfWeek > 6 {
		return nil, errors.New("day_of_week must be 0 through 6")
	}
	if rule.Kind != KindFree && rule.Kind != KindBusy {
		return nil, errors.New("rule kind must be free or busy")
	}
	if err := validateTimeRange(rule.StartTime, rule.EndTime); err != nil {
		return nil, err
	}
	startsOn, err := parseDate(rule.StartsOn)
	if err != nil {
		return nil, err
	}
	var endsOn any
	if rule.EndsOn != nil {
		endDate, parseErr := parseDate(*rule.EndsOn)
		if parseErr != nil || endDate.Before(startsOn) {
			return nil, errors.New("ends_on must not be before starts_on")
		}
		endsOn = *rule.EndsOn
	}
	if _, err = time.LoadLocation(rule.Timezone); err != nil {
		return nil, errors.New("timezone must be a valid IANA timezone")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin recurring rule creation: %w", err)
	}
	defer tx.Rollback()
	if err = lockScheduleUser(tx, userID); err != nil {
		return nil, err
	}
	var id string
	var start time.Time
	var end time.Time
	err = tx.QueryRow(`
		INSERT INTO availability_rules (user_id, day_of_week, start_time, end_time, kind, starts_on, ends_on, timezone, is_active)
		VALUES ($1, $2, $3::time, $4::time, $5, $6, $7, $8, TRUE)
		RETURNING id, start_time, end_time
	`, userID, rule.DayOfWeek, rule.StartTime, rule.EndTime, rule.Kind, rule.StartsOn, endsOn, rule.Timezone).Scan(&id, &start, &end)
	if err != nil {
		return nil, fmt.Errorf("create recurring rule: %w", err)
	}
	rule.ID = id
	rule.StartTime = start.Format("15:04")
	rule.EndTime = end.Format("15:04")
	rule.IsActive = true
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit recurring rule: %w", err)
	}
	return &rule, nil
}

func (s *Service) DeleteRule(userID string, ruleID string) error {
	result, err := s.db.Exec(`UPDATE availability_rules SET is_active = FALSE, updated_at = NOW() WHERE id = $1 AND user_id = $2 AND is_active = TRUE`, ruleID, userID)
	if err != nil {
		return fmt.Errorf("deactivate availability rule: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check deactivated rule: %w", err)
	}
	if count != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Service) SetTimezone(userID string, timezone string) error {
	if _, err := time.LoadLocation(timezone); err != nil {
		return errors.New("timezone must be a valid IANA timezone")
	}
	result, err := s.db.Exec(`UPDATE users SET timezone = $2, updated_at = NOW() WHERE id = $1 AND is_active = TRUE AND is_deleted = FALSE`, userID, timezone)
	if err != nil {
		return fmt.Errorf("update user timezone: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check timezone update: %w", err)
	}
	if count != 1 {
		return errors.New("user account is unavailable")
	}
	return nil
}

func (s *Service) listRulesInRange(userID string, from time.Time, to time.Time) ([]Rule, error) {
	rows, err := s.db.Query(`
		SELECT id, day_of_week, start_time, end_time, kind, starts_on, ends_on, timezone, is_active
		FROM availability_rules
		WHERE user_id = $1 AND is_active = TRUE
		  AND starts_on <= $3 AND (ends_on IS NULL OR ends_on >= $2)
	`, userID, from.Format("2006-01-02"), to.Format("2006-01-02"))
	if err != nil {
		return nil, fmt.Errorf("load recurring rules: %w", err)
	}
	defer rows.Close()
	rules := make([]Rule, 0)
	for rows.Next() {
		var rule Rule
		var start, end, startsOn time.Time
		var endsOn sql.NullTime
		if err = rows.Scan(&rule.ID, &rule.DayOfWeek, &start, &end, &rule.Kind, &startsOn, &endsOn, &rule.Timezone, &rule.IsActive); err != nil {
			return nil, fmt.Errorf("scan recurring rule: %w", err)
		}
		rule.StartTime = start.Format("15:04")
		rule.EndTime = end.Format("15:04")
		rule.StartsOn = startsOn.Format("2006-01-02")
		if endsOn.Valid {
			value := endsOn.Time.Format("2006-01-02")
			rule.EndsOn = &value
		}
		rules = append(rules, rule)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recurring rules: %w", err)
	}
	return rules, nil
}

func lockScheduleUser(tx *sql.Tx, userID string) error {
	var id string
	err := tx.QueryRow(`SELECT id FROM users WHERE id = $1 AND is_active = TRUE AND is_deleted = FALSE FOR UPDATE`, userID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("user account is unavailable")
	}
	if err != nil {
		return fmt.Errorf("lock schedule owner: %w", err)
	}
	return nil
}

func validateTimeRange(start string, end string) error {
	startTime, err := time.Parse("15:04", start)
	if err != nil {
		return errors.New("time must use HH:MM format")
	}
	endTime, err := time.Parse("15:04", end)
	if err != nil || !endTime.After(startTime) {
		return errors.New("end time must be after start time on the same day")
	}
	return nil
}

func validateDateRange(from string, to string, maxDays int) (time.Time, time.Time, error) {
	start, err := parseDate(from)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	end, err := parseDate(to)
	if err != nil || end.Before(start) || end.Sub(start).Hours()/24 > float64(maxDays) {
		return time.Time{}, time.Time{}, fmt.Errorf("date range must be ordered and no longer than %d days", maxDays)
	}
	return start, end, nil
}

func parseDate(value string) (time.Time, error) {
	date, err := time.Parse("2006-01-02", strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, errors.New("date must use YYYY-MM-DD format")
	}
	return date.UTC(), nil
}

func parseISODate(value string) time.Time {
	date, _ := time.Parse("2006-01-02", value)
	return date.UTC()
}

func FindCommonSlots(groups [][]Range, duration time.Duration) []Range {
	if len(groups) == 0 || duration <= 0 {
		return nil
	}
	common := append([]Range(nil), groups[0]...)
	for _, group := range groups[1:] {
		next := make([]Range, 0)
		for _, left := range common {
			for _, right := range group {
				start := left.Start
				if right.Start.After(start) {
					start = right.Start
				}
				end := left.End
				if right.End.Before(end) {
					end = right.End
				}
				if end.Sub(start) >= duration {
					next = append(next, Range{Start: start, End: end})
				}
			}
		}
		common = mergeRanges(next)
		if len(common) == 0 {
			return nil
		}
	}
	return common
}

func mergeRanges(ranges []Range) []Range {
	if len(ranges) < 2 {
		return ranges
	}
	for i := 0; i < len(ranges); i++ {
		for j := i + 1; j < len(ranges); j++ {
			if ranges[j].Start.Before(ranges[i].Start) {
				ranges[i], ranges[j] = ranges[j], ranges[i]
			}
		}
	}
	merged := []Range{ranges[0]}
	for _, current := range ranges[1:] {
		last := &merged[len(merged)-1]
		if !current.Start.After(last.End) {
			if current.End.After(last.End) {
				last.End = current.End
			}
		} else {
			merged = append(merged, current)
		}
	}
	return merged
}
