package walks

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var publicIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type Service struct{ db *sql.DB }

func NewService(db *sql.DB) *Service { return &Service{db: db} }

type Participant struct {
	PublicID    string     `json:"public_id"`
	DisplayName string     `json:"display_name"`
	Status      string     `json:"status"`
	Required    bool       `json:"is_required"`
	RespondedAt *time.Time `json:"responded_at,omitempty"`
}

type Event struct {
	ID           string        `json:"id"`
	Title        string        `json:"title"`
	Description  string        `json:"description,omitempty"`
	Status       string        `json:"status"`
	Start        time.Time     `json:"scheduled_start"`
	End          time.Time     `json:"scheduled_end"`
	Timezone     string        `json:"timezone"`
	Location     string        `json:"location_name,omitempty"`
	Latitude     *float64      `json:"latitude,omitempty"`
	Longitude    *float64      `json:"longitude,omitempty"`
	OrganizerID  string        `json:"organizer_public_id"`
	Organizer    string        `json:"organizer_name"`
	IsOrganizer  bool          `json:"is_organizer"`
	Participants []Participant `json:"participants"`
	History      []HistoryItem `json:"history"`
}

type HistoryItem struct {
	Type    string          `json:"event_type"`
	Actor   string          `json:"actor_name,omitempty"`
	Details json.RawMessage `json:"details"`
	At      time.Time       `json:"created_at"`
}

type CreateInput struct {
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	Start        time.Time `json:"scheduled_start"`
	End          time.Time `json:"scheduled_end"`
	Timezone     string    `json:"timezone"`
	Location     string    `json:"location_name"`
	Latitude     *float64  `json:"latitude"`
	Longitude    *float64  `json:"longitude"`
	Participants []string  `json:"participants"`
}

func (s *Service) Create(ownerID string, input CreateInput) (*Event, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("walk service unavailable")
	}
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)
	input.Location = strings.TrimSpace(input.Location)
	if len(input.Title) > 120 || len(input.Description) > 2000 || len(input.Location) > 300 {
		return nil, errors.New("title, description, or location is too long")
	}
	if !input.Start.After(time.Now().Add(-time.Minute)) || !input.End.After(input.Start) {
		return nil, errors.New("walk time must be in the future and end after start")
	}
	if input.End.Sub(input.Start) < 15*time.Minute || input.End.Sub(input.Start) > 12*time.Hour {
		return nil, errors.New("walk duration must be between 15 minutes and 12 hours")
	}
	if _, err := time.LoadLocation(input.Timezone); err != nil {
		return nil, errors.New("timezone must be a valid IANA timezone")
	}
	if (input.Latitude == nil) != (input.Longitude == nil) {
		return nil, errors.New("latitude and longitude must be provided together")
	}
	if input.Latitude != nil && (*input.Latitude < -90 || *input.Latitude > 90 || *input.Longitude < -180 || *input.Longitude > 180) {
		return nil, errors.New("invalid map coordinates")
	}
	if len(input.Participants) == 0 || len(input.Participants) > 20 {
		return nil, errors.New("select between 1 and 20 friends")
	}
	unique := map[string]struct{}{}
	for _, id := range input.Participants {
		if !publicIDPattern.MatchString(id) {
			return nil, errors.New("invalid participant id")
		}
		if _, exists := unique[id]; exists {
			return nil, errors.New("duplicate participant")
		}
		unique[id] = struct{}{}
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin walk creation: %w", err)
	}
	defer tx.Rollback()
	var creatorName, creatorTimezone string
	if err = tx.QueryRow(`SELECT display_name, timezone FROM users WHERE id=$1 AND is_active=TRUE AND is_deleted=FALSE FOR UPDATE`, ownerID).Scan(&creatorName, &creatorTimezone); err != nil {
		return nil, errors.New("organizer account is unavailable")
	}
	if input.Timezone == "" {
		input.Timezone = creatorTimezone
	}
	var meetingID string
	err = tx.QueryRow(`INSERT INTO meetings (created_by,title,description,status,scheduled_start,scheduled_end,timezone,duration_minutes,location_name,latitude,longitude)
		VALUES ($1,$2,$3,'pending',$4,$5,$6,$7,NULLIF($8,''),$9,$10) RETURNING id`, ownerID, input.Title, input.Description, input.Start, input.End, input.Timezone, int(input.End.Sub(input.Start).Minutes()), input.Location, input.Latitude, input.Longitude).Scan(&meetingID)
	if err != nil {
		return nil, fmt.Errorf("create walk: %w", err)
	}
	if _, err = tx.Exec(`INSERT INTO meeting_participants(meeting_id,user_id,status,is_required,responded_at) VALUES($1,$2,'accepted',TRUE,NOW())`, meetingID, ownerID); err != nil {
		return nil, fmt.Errorf("add organizer to walk: %w", err)
	}
	for _, publicID := range input.Participants {
		var participantID, participantName string
		err = tx.QueryRow(`SELECT friend.id, friend.display_name FROM users friend
			JOIN friend_requests fr ON fr.user_low_id=LEAST(friend.id,$1::uuid) AND fr.user_high_id=GREATEST(friend.id,$1::uuid) AND fr.status='accepted'
			WHERE friend.public_id=$2 AND friend.is_active=TRUE AND friend.is_deleted=FALSE`, ownerID, publicID).Scan(&participantID, &participantName)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("all participants must be active friends")
		}
		if err != nil {
			return nil, fmt.Errorf("validate walk participant: %w", err)
		}
		if participantID == ownerID {
			return nil, errors.New("organizer cannot be invited as a participant")
		}
		if _, err = tx.Exec(`INSERT INTO meeting_participants(meeting_id,user_id,status,is_required) VALUES($1,$2,'pending',TRUE)`, meetingID, participantID); err != nil {
			return nil, fmt.Errorf("add walk participant: %w", err)
		}
		if err = queueNotification(tx, participantID, "walk_invitation", meetingID, creatorName, input.Title); err != nil {
			return nil, err
		}
	}
	if _, err = tx.Exec(`INSERT INTO meeting_time_options(meeting_id,proposed_by,scheduled_start,scheduled_end,status) VALUES($1,$2,$3,$4,'selected')`, meetingID, ownerID, input.Start, input.End); err != nil {
		return nil, fmt.Errorf("record selected walk time: %w", err)
	}
	if err = addHistory(tx, meetingID, ownerID, "created", map[string]any{"title": input.Title}); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit walk creation: %w", err)
	}
	return s.Get(ownerID, meetingID)
}

func (s *Service) List(userID string, status string) ([]Event, error) {
	if err := s.expireDueWalks(); err != nil {
		return nil, err
	}
	query := `SELECT m.id FROM meetings m JOIN meeting_participants p ON p.meeting_id=m.id WHERE p.user_id=$1`
	args := []any{userID}
	if status != "" {
		query += ` AND m.status=$2`
		args = append(args, status)
	}
	query += ` ORDER BY m.scheduled_start DESC LIMIT 100`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list walks: %w", err)
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	result := make([]Event, 0, len(ids))
	for _, id := range ids {
		item, getErr := s.Get(userID, id)
		if getErr != nil {
			return nil, getErr
		}
		result = append(result, *item)
	}
	return result, nil
}

func (s *Service) Get(userID, meetingID string) (*Event, error) {
	if err := s.expireDueWalks(); err != nil {
		return nil, err
	}
	var item Event
	err := s.db.QueryRow(`SELECT m.id,m.title,COALESCE(m.description,''),m.status,m.scheduled_start,m.scheduled_end,m.timezone,COALESCE(m.location_name,''),m.latitude,m.longitude,owner.public_id,owner.display_name,(m.created_by=$1)
		FROM meetings m JOIN meeting_participants access ON access.meeting_id=m.id AND access.user_id=$1
		JOIN users owner ON owner.id=m.created_by WHERE m.id=$2`, userID, meetingID).Scan(&item.ID, &item.Title, &item.Description, &item.Status, &item.Start, &item.End, &item.Timezone, &item.Location, &item.Latitude, &item.Longitude, &item.OrganizerID, &item.Organizer, &item.IsOrganizer)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("walk is not available")
	}
	if err != nil {
		return nil, fmt.Errorf("load walk: %w", err)
	}
	item.Participants = []Participant{}
	rows, err := s.db.Query(`SELECT u.public_id,u.display_name,p.status,p.is_required,p.responded_at FROM meeting_participants p JOIN users u ON u.id=p.user_id WHERE p.meeting_id=$1 ORDER BY (p.user_id=(SELECT created_by FROM meetings WHERE id=$1)) DESC,u.display_name`, meetingID)
	if err != nil {
		return nil, fmt.Errorf("load walk participants: %w", err)
	}
	for rows.Next() {
		var p Participant
		var at sql.NullTime
		if err = rows.Scan(&p.PublicID, &p.DisplayName, &p.Status, &p.Required, &at); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if at.Valid {
			p.RespondedAt = &at.Time
		}
		item.Participants = append(item.Participants, p)
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()
	item.History = []HistoryItem{}
	rows, err = s.db.Query(`SELECT h.event_type,COALESCE(u.display_name,''),h.details,h.created_at FROM meeting_history h LEFT JOIN users u ON u.id=h.actor_user_id WHERE h.meeting_id=$1 ORDER BY h.created_at`, meetingID)
	if err != nil {
		return nil, fmt.Errorf("load walk history: %w", err)
	}
	for rows.Next() {
		var h HistoryItem
		if err = rows.Scan(&h.Type, &h.Actor, &h.Details, &h.At); err != nil {
			_ = rows.Close()
			return nil, err
		}
		item.History = append(item.History, h)
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()
	return &item, nil
}

func (s *Service) Respond(userID, meetingID, action string) error {
	if action != "accept" && action != "reject" {
		return errors.New("action must be accept or reject")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var status, ownerID, title string
	if err = tx.QueryRow(`SELECT status,created_by,title FROM meetings WHERE id=$1 FOR UPDATE`, meetingID).Scan(&status, &ownerID, &title); errors.Is(err, sql.ErrNoRows) {
		return errors.New("walk is not available")
	}
	if err != nil {
		return err
	}
	if status != "pending" && status != "rescheduled" {
		return errors.New("walk is not awaiting responses")
	}
	var actorName string
	if err = tx.QueryRow(`SELECT display_name FROM users WHERE id=$1 AND is_active=TRUE AND is_deleted=FALSE`, userID).Scan(&actorName); err != nil {
		return errors.New("account is unavailable")
	}
	value := "accepted"
	event := "participant_accepted"
	if action == "reject" {
		value = "rejected"
		event = "participant_rejected"
	}
	result, err := tx.Exec(`UPDATE meeting_participants SET status=$3,responded_at=NOW() WHERE meeting_id=$1 AND user_id=$2 AND user_id<>$4 AND status='pending'`, meetingID, userID, value, ownerID)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return errors.New("walk invitation is not available")
	}
	if err = addHistory(tx, meetingID, userID, event, map[string]any{}); err != nil {
		return err
	}
	if action == "reject" {
		_, err = tx.Exec(`UPDATE meetings SET status='rejected',updated_at=NOW() WHERE id=$1`, meetingID)
		if err != nil {
			return err
		}
		if err = queueNotification(tx, ownerID, "walk_rejected", meetingID, actorName, title); err != nil {
			return err
		}
	} else {
		if err = queueNotification(tx, ownerID, "walk_accepted", meetingID, actorName, title); err != nil {
			return err
		}
		var pending, rejected int
		if err = tx.QueryRow(`SELECT COUNT(*) FILTER(WHERE status='pending' AND is_required),COUNT(*) FILTER(WHERE status='rejected' AND is_required) FROM meeting_participants WHERE meeting_id=$1`, meetingID).Scan(&pending, &rejected); err != nil {
			return err
		}
		if pending == 0 && rejected == 0 {
			_, err = tx.Exec(`UPDATE meetings SET status='confirmed',updated_at=NOW() WHERE id=$1`, meetingID)
			if err != nil {
				return err
			}
			if err = addHistory(tx, meetingID, userID, "confirmed", map[string]any{}); err != nil {
				return err
			}
			if err = notifyParticipants(tx, meetingID, userID, "walk_confirmed", actorName, title); err != nil {
				return err
			}
			if err = scheduleReminderNotifications(tx, meetingID); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func (s *Service) Cancel(userID, meetingID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var title, status string
	err = tx.QueryRow(`SELECT title,status FROM meetings WHERE id=$1 AND created_by=$2 FOR UPDATE`, meetingID, userID).Scan(&title, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("only the organizer can cancel this walk")
	}
	if err != nil {
		return err
	}
	if status == "cancelled" || status == "completed" || status == "expired" {
		return errors.New("walk cannot be cancelled in its current state")
	}
	var actor string
	_ = tx.QueryRow(`SELECT display_name FROM users WHERE id=$1`, userID).Scan(&actor)
	if _, err = tx.Exec(`UPDATE meetings SET status='cancelled',updated_at=NOW() WHERE id=$1`, meetingID); err != nil {
		return err
	}
	if err = addHistory(tx, meetingID, userID, "cancelled", map[string]any{}); err != nil {
		return err
	}
	if err = notifyParticipants(tx, meetingID, userID, "walk_cancelled", actor, title); err != nil {
		return err
	}
	if err = cancelWalkReminders(tx, meetingID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) Reschedule(userID, meetingID string, start, end time.Time, location string, latitude, longitude *float64) error {
	if !start.After(time.Now()) || !end.After(start) || end.Sub(start) < 15*time.Minute || end.Sub(start) > 12*time.Hour {
		return errors.New("new walk time must be in the future and end after start")
	}
	if (latitude == nil) != (longitude == nil) || (latitude != nil && (*latitude < -90 || *latitude > 90 || *longitude < -180 || *longitude > 180)) {
		return errors.New("invalid map coordinates")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var title, status string
	err = tx.QueryRow(`SELECT title,status FROM meetings WHERE id=$1 AND created_by=$2 FOR UPDATE`, meetingID, userID).Scan(&title, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("only the organizer can reschedule this walk")
	}
	if err != nil {
		return err
	}
	if status != "confirmed" && status != "pending" && status != "rescheduled" {
		return errors.New("walk cannot be rescheduled in its current state")
	}
	if _, err = tx.Exec(`UPDATE meetings SET scheduled_start=$2,scheduled_end=$3,duration_minutes=$4,location_name=NULLIF($5,''),latitude=$6,longitude=$7,status='rescheduled',version=version+1,updated_at=NOW() WHERE id=$1`, meetingID, start, end, int(end.Sub(start).Minutes()), strings.TrimSpace(location), latitude, longitude); err != nil {
		return err
	}
	if _, err = tx.Exec(`UPDATE meeting_participants SET status=CASE WHEN user_id=$2 THEN 'accepted' ELSE 'pending' END,responded_at=CASE WHEN user_id=$2 THEN NOW() ELSE NULL END WHERE meeting_id=$1 AND is_required=TRUE`, meetingID, userID); err != nil {
		return err
	}
	if _, err = tx.Exec(`UPDATE meeting_time_options SET status='rejected' WHERE meeting_id=$1 AND status IN ('selected','proposed')`, meetingID); err != nil {
		return err
	}
	if err = cancelWalkReminders(tx, meetingID); err != nil {
		return err
	}
	if _, err = tx.Exec(`INSERT INTO meeting_time_options(meeting_id,proposed_by,scheduled_start,scheduled_end,status) VALUES($1,$2,$3,$4,'selected')`, meetingID, userID, start, end); err != nil {
		return err
	}
	if err = addHistory(tx, meetingID, userID, "rescheduled", map[string]any{"scheduled_start": start, "scheduled_end": end, "location_name": location}); err != nil {
		return err
	}
	var actor string
	_ = tx.QueryRow(`SELECT display_name FROM users WHERE id=$1`, userID).Scan(&actor)
	if err = notifyParticipants(tx, meetingID, userID, "walk_rescheduled", actor, title); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) Complete(userID, meetingID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`UPDATE meetings SET status='completed',updated_at=NOW() WHERE id=$1 AND created_by=$2 AND status='confirmed' AND scheduled_end<=NOW()`, meetingID, userID)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return errors.New("only an ended confirmed walk can be completed by its organizer")
	}
	if _, err = tx.Exec(`INSERT INTO meeting_history(meeting_id,actor_user_id,event_type) VALUES($1,$2,'completed')`, meetingID, userID); err != nil {
		return err
	}
	if err = cancelWalkReminders(tx, meetingID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) Export(userID, meetingID string) (*Event, error) {
	item, err := s.Get(userID, meetingID)
	if err != nil {
		return nil, err
	}
	if item.Status != "confirmed" && item.Status != "completed" {
		return nil, errors.New("only confirmed walks can be exported")
	}
	return item, nil
}

func (s *Service) expireDueWalks() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT id,created_by,title,version FROM meetings WHERE status IN ('pending','rescheduled') AND scheduled_start<=NOW() FOR UPDATE SKIP LOCKED`)
	if err != nil {
		return err
	}
	type dueWalk struct {
		id, owner, title string
		version          int
	}
	items := []dueWalk{}
	for rows.Next() {
		var item dueWalk
		if err = rows.Scan(&item.id, &item.owner, &item.title, &item.version); err != nil {
			_ = rows.Close()
			return err
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()
	for _, item := range items {
		if _, err = tx.Exec(`UPDATE meetings SET status='expired',updated_at=NOW() WHERE id=$1`, item.id); err != nil {
			return err
		}
		if _, err = tx.Exec(`INSERT INTO meeting_history(meeting_id,actor_user_id,event_type,details) VALUES($1,NULL,'expired','{}'::jsonb)`, item.id); err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]string{"actor_name": "WalkTogether", "title": item.title})
		key := fmt.Sprintf("walk_expired:%s:%s:%d", item.id, item.owner, item.version)
		if _, err = tx.Exec(`INSERT INTO notification_jobs(user_id,notification_type,payload,dedupe_key) VALUES($1,'walk_expired',$2,$3) ON CONFLICT (dedupe_key) WHERE dedupe_key IS NOT NULL DO NOTHING`, item.owner, payload, key); err != nil {
			return err
		}
		if err = cancelWalkReminders(tx, item.id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func queueNotification(tx *sql.Tx, userID, kind, meetingID, actor, title string) error {
	payload, _ := json.Marshal(map[string]string{"actor_name": actor, "title": title})
	var version int
	if err := tx.QueryRow(`SELECT version FROM meetings WHERE id=$1`, meetingID).Scan(&version); err != nil {
		return fmt.Errorf("read walk notification version: %w", err)
	}
	_, err := tx.Exec(`INSERT INTO notification_jobs(user_id,notification_type,payload,dedupe_key) VALUES($1,$2,$3,$4) ON CONFLICT (dedupe_key) WHERE dedupe_key IS NOT NULL DO NOTHING`, userID, kind, payload, fmt.Sprintf("%s:%s:%s:%d", kind, meetingID, userID, version))
	if err != nil {
		return fmt.Errorf("queue walk notification: %w", err)
	}
	return nil
}
func notifyParticipants(tx *sql.Tx, meetingID, except, kind, actor, title string) error {
	rows, err := tx.Query(`SELECT user_id FROM meeting_participants WHERE meeting_id=$1 AND user_id<>$2`, meetingID, except)
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()
	for _, id := range ids {
		if err = queueNotification(tx, id, kind, meetingID, actor, title); err != nil {
			return err
		}
	}
	return nil
}

func scheduleReminderNotifications(tx *sql.Tx, meetingID string) error {
	var starts time.Time
	var title, organizerName string
	var version int
	if err := tx.QueryRow(`SELECT m.scheduled_start,m.title,m.version,u.display_name FROM meetings m JOIN users u ON u.id=m.created_by WHERE m.id=$1`, meetingID).Scan(&starts, &title, &version, &organizerName); err != nil {
		return err
	}
	rows, err := tx.Query(`SELECT user_id FROM meeting_participants WHERE meeting_id=$1`, meetingID)
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()
	for _, id := range ids {
		payload, _ := json.Marshal(map[string]string{"actor_name": organizerName, "title": title})
		key := fmt.Sprintf("walk_reminder:%s:%s:%d", meetingID, id, version)
		if _, err = tx.Exec(`INSERT INTO notification_jobs(user_id,notification_type,payload,scheduled_for,dedupe_key)
			VALUES($1,'walk_reminder',$2,GREATEST(NOW(),$3::timestamptz-INTERVAL '1 hour'),$4)
			ON CONFLICT (dedupe_key) WHERE dedupe_key IS NOT NULL DO NOTHING`, id, payload, starts, key); err != nil {
			return fmt.Errorf("schedule walk reminder: %w", err)
		}
	}
	return nil
}

func cancelWalkReminders(tx *sql.Tx, meetingID string) error {
	_, err := tx.Exec(`UPDATE notification_jobs SET status='cancelled',updated_at=NOW()
		WHERE notification_type='walk_reminder' AND status='queued' AND dedupe_key LIKE $1`, "walk_reminder:"+meetingID+":%")
	return err
}
func addHistory(tx *sql.Tx, meetingID, actor, event string, details any) error {
	data, err := json.Marshal(details)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO meeting_history(meeting_id,actor_user_id,event_type,details) VALUES($1,$2,$3,$4)`, meetingID, actor, event, data)
	if err != nil {
		return fmt.Errorf("record walk history: %w", err)
	}
	return nil
}

func CalendarLinks(item *Event) map[string]string {
	start := item.Start.UTC().Format("20060102T150405Z")
	end := item.End.UTC().Format("20060102T150405Z")
	google := url.Values{"action": {"TEMPLATE"}, "text": {item.Title}, "details": {item.Description}, "location": {item.Location}, "dates": {start + "/" + end}}
	outlook := url.Values{"path": {"/calendar/action/compose"}, "rru": {"addevent"}, "subject": {item.Title}, "body": {item.Description}, "location": {item.Location}, "startdt": {item.Start.Format(time.RFC3339)}, "enddt": {item.End.Format(time.RFC3339)}}
	return map[string]string{"google": "https://calendar.google.com/calendar/render?" + google.Encode(), "outlook": "https://outlook.live.com/calendar/0/deeplink/compose?" + outlook.Encode()}
}

func ICS(item *Event) []byte {
	uid := item.ID + "@walktogether"
	escape := func(value string) string {
		value = strings.ReplaceAll(value, "\\", "\\\\")
		value = strings.ReplaceAll(value, ";", `\;`)
		value = strings.ReplaceAll(value, ",", `\,`)
		value = strings.ReplaceAll(value, "\n", `\n`)
		return value
	}
	lines := []string{"BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//WalkTogether//Walks//RU", "CALSCALE:GREGORIAN", "BEGIN:VEVENT", "UID:" + uid, "DTSTAMP:" + time.Now().UTC().Format("20060102T150405Z"), "DTSTART:" + item.Start.UTC().Format("20060102T150405Z"), "DTEND:" + item.End.UTC().Format("20060102T150405Z"), "SUMMARY:" + escape(item.Title), "DESCRIPTION:" + escape(item.Description), "LOCATION:" + escape(item.Location), "END:VEVENT", "END:VCALENDAR"}
	return []byte(strings.Join(lines, "\r\n") + "\r\n")
}
