package schedule

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"time"
)

var scheduleUUIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

const (
	VisibilityFriendsExact  = "friends_exact"
	VisibilitySelectedExact = "selected_exact"
	VisibilityBusyOnly      = "busy_only"
)

type PrivacySettings struct {
	Visibility string   `json:"visibility"`
	Timezone   string   `json:"timezone"`
	ExactFor   []string `json:"exact_for"`
}

func (s *Service) GetPrivacySettings(userID string) (*PrivacySettings, error) {
	var settings PrivacySettings
	err := s.db.QueryRow(`
		SELECT availability_visibility, timezone FROM users
		WHERE id = $1 AND is_active = TRUE AND is_deleted = FALSE
	`, userID).Scan(&settings.Visibility, &settings.Timezone)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("user account is unavailable")
	}
	if err != nil {
		return nil, fmt.Errorf("load schedule privacy: %w", err)
	}
	rows, err := s.db.Query(`
		SELECT u.public_id FROM availability_shares shares
		JOIN users u ON u.id = shares.friend_user_id
		WHERE shares.owner_user_id = $1
		ORDER BY u.display_name
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("load exact availability grants: %w", err)
	}
	defer rows.Close()
	settings.ExactFor = make([]string, 0)
	for rows.Next() {
		var publicID string
		if err = rows.Scan(&publicID); err != nil {
			return nil, fmt.Errorf("scan availability grant: %w", err)
		}
		settings.ExactFor = append(settings.ExactFor, publicID)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate availability grants: %w", err)
	}
	return &settings, nil
}

func (s *Service) UpdatePrivacySettings(userID string, settings PrivacySettings) error {
	if settings.Visibility != VisibilityFriendsExact && settings.Visibility != VisibilitySelectedExact && settings.Visibility != VisibilityBusyOnly {
		return errors.New("visibility must be friends_exact, selected_exact, or busy_only")
	}
	if _, err := time.LoadLocation(settings.Timezone); err != nil {
		return errors.New("timezone must be a valid IANA timezone")
	}
	if len(settings.ExactFor) > 20 {
		return errors.New("at most 20 friends may receive exact schedule access")
	}
	uniqueIDs := make(map[string]struct{}, len(settings.ExactFor))
	for _, publicID := range settings.ExactFor {
		if !scheduleUUIDPattern.MatchString(publicID) {
			return errors.New("invalid friend public id")
		}
		if _, exists := uniqueIDs[publicID]; exists {
			return errors.New("duplicate friend public id")
		}
		uniqueIDs[publicID] = struct{}{}
	}
	if settings.Visibility != VisibilitySelectedExact && len(settings.ExactFor) != 0 {
		return errors.New("exact_for is only valid for selected_exact visibility")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin schedule privacy update: %w", err)
	}
	defer tx.Rollback()
	if err = lockScheduleUser(tx, userID); err != nil {
		return err
	}
	if _, err = tx.Exec(`
		UPDATE users SET availability_visibility = $2, timezone = $3, updated_at = NOW()
		WHERE id = $1
	`, userID, settings.Visibility, settings.Timezone); err != nil {
		return fmt.Errorf("update schedule privacy: %w", err)
	}
	if _, err = tx.Exec(`UPDATE availability_rules SET timezone = $2, updated_at = NOW() WHERE user_id = $1`, userID, settings.Timezone); err != nil {
		return fmt.Errorf("update rule timezone: %w", err)
	}
	if _, err = tx.Exec(`DELETE FROM availability_shares WHERE owner_user_id = $1`, userID); err != nil {
		return fmt.Errorf("replace exact schedule grants: %w", err)
	}
	if settings.Visibility == VisibilitySelectedExact {
		ids := make([]string, 0, len(uniqueIDs))
		for id := range uniqueIDs {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, publicID := range ids {
			result, insertErr := tx.Exec(`
				INSERT INTO availability_shares (owner_user_id, friend_user_id)
				SELECT $1, friend.id
				FROM users friend
			JOIN friend_requests relation
			  ON relation.user_low_id = LEAST(friend.id, $1::uuid)
			 AND relation.user_high_id = GREATEST(friend.id, $1::uuid)
			 AND relation.status = 'accepted'
			WHERE friend.public_id = $2 AND friend.is_active = TRUE AND friend.is_deleted = FALSE
			`, userID, publicID)
			if insertErr != nil {
				return fmt.Errorf("grant exact schedule access: %w", insertErr)
			}
			count, countErr := result.RowsAffected()
			if countErr != nil {
				return fmt.Errorf("check schedule grant: %w", countErr)
			}
			if count != 1 {
				return errors.New("exact schedule access can only be granted to an active friend")
			}
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit schedule privacy update: %w", err)
	}
	return nil
}

func (s *Service) FriendsAvailability(viewerID string, date string, publicIDs []string) ([]FriendDay, error) {
	if _, err := parseDate(date); err != nil {
		return nil, err
	}
	if len(publicIDs) == 0 || len(publicIDs) > 20 {
		return nil, errors.New("select between 1 and 20 friends")
	}
	unique := make(map[string]struct{}, len(publicIDs))
	results := make([]FriendDay, 0, len(publicIDs))
	for _, publicID := range publicIDs {
		if _, exists := unique[publicID]; exists {
			continue
		}
		unique[publicID] = struct{}{}
		if !scheduleUUIDPattern.MatchString(publicID) {
			return nil, errors.New("invalid friend public id")
		}
		var friendID string
		var item FriendDay
		var timezone string
		var visibility string
		var exactGrant bool
		err := s.db.QueryRow(`
			SELECT friend.id, friend.public_id, friend.display_name, friend.availability_visibility, friend.timezone,
			       EXISTS (SELECT 1 FROM availability_shares grants WHERE grants.owner_user_id = friend.id AND grants.friend_user_id = $1)
		FROM users viewer
		JOIN users friend ON friend.public_id = $2
		JOIN friend_requests relation
		  ON relation.user_low_id = LEAST(viewer.id, friend.id)
		 AND relation.user_high_id = GREATEST(viewer.id, friend.id)
		 AND relation.status = 'accepted'
		WHERE viewer.id = $1 AND viewer.is_active = TRUE AND viewer.is_deleted = FALSE
		  AND friend.is_active = TRUE AND friend.is_deleted = FALSE
		`, viewerID, publicID).Scan(&friendID, &item.PublicID, &item.DisplayName, &visibility, &timezone, &exactGrant)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("availability is only available for active friends")
		}
		if err != nil {
			return nil, fmt.Errorf("check friend availability access: %w", err)
		}
		item.Date = date
		item.Exact = visibility == VisibilityFriendsExact || (visibility == VisibilitySelectedExact && exactGrant)
		item.State, item.Intervals, err = s.effectiveDay(friendID, date)
		if err != nil {
			return nil, err
		}
		if !item.Exact {
			item.State = availabilitySummary(item.State, item.Intervals)
			item.Intervals = nil
		} else {
			item.Timezone = timezone
		}
		results = append(results, item)
	}
	return results, nil
}

func (s *Service) effectiveDay(userID string, date string) (string, []Interval, error) {
	var state string
	err := s.db.QueryRow(`SELECT data_state FROM availability WHERE user_id = $1 AND day = $2`, userID, date).Scan(&state)
	if err == nil {
		if state != DayCustom {
			return state, []Interval{}, nil
		}
		intervals, intervalErr := s.intervalsForDay(userID, date)
		return state, intervals, intervalErr
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", nil, fmt.Errorf("load availability override: %w", err)
	}
	day, err := parseDate(date)
	if err != nil {
		return "", nil, err
	}
	var userTimezone string
	if err = s.db.QueryRow(`SELECT timezone FROM users WHERE id = $1 AND is_active = TRUE AND is_deleted = FALSE`, userID).Scan(&userTimezone); err != nil {
		return "", nil, fmt.Errorf("load availability timezone: %w", err)
	}
	rows, err := s.db.Query(`
		SELECT start_time, end_time, kind FROM availability_rules
		WHERE user_id = $1 AND day_of_week = $2 AND is_active = TRUE
		  AND starts_on <= $3 AND (ends_on IS NULL OR ends_on >= $3)
		ORDER BY start_time
	`, userID, int(day.Weekday()), date)
	if err != nil {
		return "", nil, fmt.Errorf("load effective recurring rules: %w", err)
	}
	defer rows.Close()
	intervals := make([]Interval, 0)
	for rows.Next() {
		var start, end time.Time
		var kind string
		if err = rows.Scan(&start, &end, &kind); err != nil {
			return "", nil, fmt.Errorf("scan effective rule: %w", err)
		}
		intervals = append(intervals, Interval{StartTime: start.Format("15:04"), EndTime: end.Format("15:04"), Kind: kind})
	}
	if err = rows.Err(); err != nil {
		return "", nil, fmt.Errorf("iterate effective rules: %w", err)
	}
	if len(intervals) == 0 {
		return DayUnknown, intervals, nil
	}
	_ = userTimezone
	return DayCustom, intervals, nil
}

func (s *Service) intervalsForDay(userID string, date string) ([]Interval, error) {
	rows, err := s.db.Query(`
		SELECT id, starts_at, ends_at, kind FROM availability_intervals
		WHERE user_id = $1 AND day = $2 ORDER BY starts_at
	`, userID, date)
	if err != nil {
		return nil, fmt.Errorf("load intervals: %w", err)
	}
	defer rows.Close()
	intervals := make([]Interval, 0)
	for rows.Next() {
		var interval Interval
		var start, end time.Time
		if err = rows.Scan(&interval.ID, &start, &end, &interval.Kind); err != nil {
			return nil, fmt.Errorf("scan interval: %w", err)
		}
		interval.StartTime = start.Format("15:04")
		interval.EndTime = end.Format("15:04")
		intervals = append(intervals, interval)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate intervals: %w", err)
	}
	return intervals, nil
}

func availabilitySummary(state string, intervals []Interval) string {
	if state == DayFreeAll {
		return "free"
	}
	if state == DayBusyAll {
		return "busy"
	}
	for _, interval := range intervals {
		if interval.Kind == KindFree {
			return "free"
		}
	}
	for _, interval := range intervals {
		if interval.Kind == KindBusy {
			return "busy"
		}
	}
	return "unknown"
}

func (s *Service) GetTimezone(userID string) (string, error) {
	var timezone string
	err := s.db.QueryRow(`SELECT timezone FROM users WHERE id=$1 AND is_active=TRUE AND is_deleted=FALSE`, userID).Scan(&timezone)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errors.New("user account is unavailable")
	}
	if err != nil {
		return "", err
	}
	return timezone, nil
}
