package walks

import (
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appdb "friendwalk/internal/db"
	"friendwalk/internal/places"
	"friendwalk/internal/testdb"
)

func TestPostgresWalkLifecycleIntegration(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set TEST_POSTGRES_DSN to an isolated PostgreSQL database whose name contains 'test'")
	}
	if dsn == os.Getenv("POSTGRES_DSN") {
		t.Fatal("TEST_POSTGRES_DSN must not point to POSTGRES_DSN")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.Contains(parsed.Path, "test") {
		t.Fatal("TEST_POSTGRES_DSN must name an isolated test database")
	}
	migrations, err := filepath.Abs(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	db, err := appdb.Connect(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	testdb.LockSerializesDestructiveIntegrationTests(t, db)
	if err = appdb.ApplyMigrations(dsn, migrations); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if _, err = db.Exec(`TRUNCATE users CASCADE`); err != nil {
		t.Fatal(err)
	}
	owner := createWalkTestUser(t, db, 920001, "Organizer")
	first := createWalkTestUser(t, db, 920002, "First Walker")
	second := createWalkTestUser(t, db, 920003, "Second Walker")
	addWalkTestFriend(t, db, owner.id, first.id)
	addWalkTestFriend(t, db, owner.id, second.id)
	if _, err = db.Exec(`UPDATE users SET timezone='Europe/Moscow' WHERE id IN ($1,$2,$3)`, owner.id, first.id, second.id); err != nil {
		t.Fatal(err)
	}
	service := NewService(db)
	favorites := places.NewService(db)
	favorite, err := favorites.Create(owner.id, places.Place{Name: "Park", Address: "Main entrance", Latitude: 55.75, Longitude: 37.61})
	if err != nil {
		t.Fatalf("save favorite place: %v", err)
	}
	listedPlaces, err := favorites.List(owner.id)
	if err != nil || len(listedPlaces) != 1 || listedPlaces[0].ID != favorite.ID {
		t.Fatalf("favorite place was not listed: %+v err=%v", listedPlaces, err)
	}
	if err = favorites.Delete(second.id, favorite.ID); err == nil {
		t.Fatal("another user deleted a favorite place")
	}
	if err = favorites.Delete(owner.id, favorite.ID); err != nil {
		t.Fatalf("delete favorite place: %v", err)
	}
	start := time.Now().Add(3 * time.Hour).Truncate(time.Minute)
	moscow, _ := time.LoadLocation("Europe/Moscow")
	availabilityDate := start.In(moscow).Format("2006-01-02")
	setWalkAvailability(t, db, owner.id, availabilityDate, "15:00", "20:00")
	setWalkAvailability(t, db, first.id, availabilityDate, "16:00", "19:00")
	setWalkAvailability(t, db, second.id, availabilityDate, "17:00", "21:00")
	slots, err := service.CommonSlots(owner.id, availabilityDate, []string{first.publicID, second.publicID}, 90)
	if err != nil || len(slots) == 0 || slots[0].Start.In(moscow).Format("15:04") != "17:00" {
		t.Fatalf("common availability slots are incorrect: %+v err=%v", slots, err)
	}
	if _, err = db.Exec(`UPDATE users SET availability_visibility='busy_only' WHERE id=$1`, second.id); err != nil {
		t.Fatal(err)
	}
	if _, err = service.CommonSlots(owner.id, availabilityDate, []string{first.publicID, second.publicID}, 90); err == nil {
		t.Fatal("private friend availability was used to calculate exact common slots")
	}
	if _, err = db.Exec(`UPDATE users SET availability_visibility='friends_exact' WHERE id=$1`, second.id); err != nil {
		t.Fatal(err)
	}
	created, err := service.Create(owner.id, CreateInput{Title: "After work", Description: "Meet at the park", Start: start, End: start.Add(90 * time.Minute), Timezone: "Europe/Moscow", Location: "Park entrance", Participants: []string{first.publicID, second.publicID}})
	if err != nil {
		t.Fatalf("create walk: %v", err)
	}
	if created.Status != "pending" || len(created.Participants) != 3 {
		t.Fatalf("new walk status/participant count: status=%s participants=%d", created.Status, len(created.Participants))
	}
	if err = service.Respond(first.id, created.ID, "accept"); err != nil {
		t.Fatalf("first accept: %v", err)
	}
	pending, err := service.Get(owner.id, created.ID)
	if err != nil || pending.Status != "pending" {
		t.Fatalf("walk confirmed before all required users accepted: %+v err=%v", pending, err)
	}
	if err = service.Respond(second.id, created.ID, "accept"); err != nil {
		t.Fatalf("second accept: %v", err)
	}
	confirmed, err := service.Get(owner.id, created.ID)
	if err != nil || confirmed.Status != "confirmed" {
		t.Fatalf("walk not confirmed after all required users accepted: %+v err=%v", confirmed, err)
	}
	if len(ICS(confirmed)) == 0 || !strings.Contains(string(ICS(confirmed)), "UID:") {
		t.Fatal("ICS export did not include a unique event identifier")
	}
	links := CalendarLinks(confirmed)
	if !strings.HasPrefix(links["google"], "https://calendar.google.com/") || !strings.HasPrefix(links["outlook"], "https://outlook.live.com/") {
		t.Fatalf("calendar links are incomplete: %+v", links)
	}
	if err = service.Reschedule(owner.id, created.ID, start.Add(24*time.Hour), start.Add(25*time.Hour), "New place", nil, nil); err != nil {
		t.Fatalf("reschedule: %v", err)
	}
	rescheduled, err := service.Get(owner.id, created.ID)
	if err != nil || rescheduled.Status != "rescheduled" {
		t.Fatalf("rescheduled status missing: %+v err=%v", rescheduled, err)
	}
	if err = service.Respond(first.id, created.ID, "accept"); err != nil {
		t.Fatalf("first re-confirmation: %v", err)
	}
	if err = service.Respond(second.id, created.ID, "accept"); err != nil {
		t.Fatalf("second re-confirmation: %v", err)
	}
	confirmed, err = service.Get(owner.id, created.ID)
	if err != nil || confirmed.Status != "confirmed" {
		t.Fatalf("rescheduled walk not reconfirmed: %+v err=%v", confirmed, err)
	}
	if len(confirmed.History) < 7 {
		t.Fatalf("history is incomplete: got %d entries", len(confirmed.History))
	}
	if err = service.Respond(owner.id, created.ID, "accept"); err == nil {
		t.Fatal("organizer accepted own invitation")
	}
	if _, err = service.Get(second.id, created.ID); err != nil {
		t.Fatalf("participant could not read walk: %v", err)
	}
	if _, err = service.Get(createWalkTestUser(t, db, 920004, "Outsider").id, created.ID); err == nil {
		t.Fatal("non-participant accessed walk")
	}
	var jobs int
	if err = db.QueryRow(`SELECT COUNT(*) FROM notification_jobs WHERE notification_type LIKE 'walk_%'`).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if jobs < 6 {
		t.Fatalf("walk events were not queued in outbox: count=%d", jobs)
	}

	rejected, err := service.Create(owner.id, CreateInput{Title: "Rejected walk", Start: start.Add(48 * time.Hour), End: start.Add(49 * time.Hour), Timezone: "Europe/Moscow", Participants: []string{first.publicID}})
	if err != nil {
		t.Fatal(err)
	}
	if err = service.Respond(first.id, rejected.ID, "reject"); err != nil {
		t.Fatal(err)
	}
	got, err := service.Get(owner.id, rejected.ID)
	if err != nil || got.Status != "rejected" {
		t.Fatalf("rejected status not set: %+v err=%v", got, err)
	}
	cancelled, err := service.Create(owner.id, CreateInput{Title: "Cancelled walk", Start: start.Add(72 * time.Hour), End: start.Add(73 * time.Hour), Timezone: "Europe/Moscow", Participants: []string{second.publicID}})
	if err != nil {
		t.Fatal(err)
	}
	if err = service.Cancel(owner.id, cancelled.ID); err != nil {
		t.Fatal(err)
	}
	got, err = service.Get(owner.id, cancelled.ID)
	if err != nil || got.Status != "cancelled" {
		t.Fatalf("cancelled status not set: %+v err=%v", got, err)
	}
	if err = service.Cancel(first.id, cancelled.ID); err == nil {
		t.Fatal("non-organizer cancelled walk")
	}
}

type walkTestUser struct{ id, publicID string }

func createWalkTestUser(t *testing.T, db *sql.DB, telegramID int64, name string) walkTestUser {
	t.Helper()
	var user walkTestUser
	if err := db.QueryRow(`INSERT INTO users(telegram_id,display_name) VALUES($1,$2) RETURNING id,public_id`, telegramID, name).Scan(&user.id, &user.publicID); err != nil {
		t.Fatal(err)
	}
	return user
}
func addWalkTestFriend(t *testing.T, db *sql.DB, owner, friend string) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO friend_requests(user_low_id,user_high_id,requester_user_id,recipient_user_id,status,responded_at) VALUES(LEAST($1::uuid,$2::uuid),GREATEST($1::uuid,$2::uuid),$1,$2,'accepted',NOW())`, owner, friend)
	if err != nil {
		t.Fatal(err)
	}
}

func setWalkAvailability(t *testing.T, db *sql.DB, userID, date, start, end string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO availability(user_id,day,is_available,data_state) VALUES($1,$2,TRUE,'custom')`, userID, date); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO availability_intervals(user_id,day,starts_at,ends_at,kind) VALUES($1,$2,$3::time,$4::time,'free')`, userID, date, start, end); err != nil {
		t.Fatal(err)
	}
}
