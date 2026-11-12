package schedule

import (
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appdb "friendwalk/internal/db"
	"friendwalk/internal/testdb"
)

func TestPostgresAvailabilityRulesPrivacyAndOverrides(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set TEST_POSTGRES_DSN to an isolated PostgreSQL database whose name contains 'test'")
	}
	if dsn == os.Getenv("POSTGRES_DSN") {
		t.Fatal("TEST_POSTGRES_DSN must not point to POSTGRES_DSN")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || len(parsed.Path) < 2 || !containsScheduleTest(parsed.Path) {
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
	owner := createScheduleTestUser(t, db, 930001, "Schedule owner")
	friend := createScheduleTestUser(t, db, 930002, "Schedule friend")
	if _, err = db.Exec(`INSERT INTO friend_requests(user_low_id,user_high_id,requester_user_id,recipient_user_id,status,responded_at) VALUES(LEAST($1::uuid,$2::uuid),GREATEST($1::uuid,$2::uuid),$1,$2,'accepted',NOW())`, owner.id, friend.id); err != nil {
		t.Fatal(err)
	}
	service := NewService(db)
	if err = service.SetTimezone(owner.id, "Europe/Moscow"); err != nil {
		t.Fatal(err)
	}
	date := time.Now().UTC().AddDate(0, 0, 3).Format("2006-01-02")
	parsedDate, _ := time.Parse("2006-01-02", date)
	rule, err := service.AddRule(owner.id, Rule{DayOfWeek: int(parsedDate.Weekday()), StartTime: "16:00", EndTime: "19:00", Kind: KindFree, StartsOn: date, EndsOn: &date, Timezone: "Europe/Moscow"})
	if err != nil {
		t.Fatalf("add recurring rule: %v", err)
	}
	days, err := service.GetCalendar(owner.id, date, date)
	if err != nil || len(days) != 1 || days[0].State != DayCustom || len(days[0].Intervals) != 1 || days[0].Intervals[0].StartTime != "16:00" {
		t.Fatalf("recurrence not applied: %+v err=%v", days, err)
	}
	if err = service.SetDayState(owner.id, date, DayFreeAll); err != nil {
		t.Fatal(err)
	}
	days, err = service.GetCalendar(owner.id, date, date)
	if err != nil || days[0].State != DayFreeAll {
		t.Fatalf("date override did not win over recurrence: %+v err=%v", days, err)
	}
	if err = service.SetDayState(owner.id, date, DayUnknown); err != nil {
		t.Fatal(err)
	}
	days, err = service.GetCalendar(owner.id, date, date)
	if err != nil || days[0].State != DayCustom || days[0].Intervals[0].StartTime != "16:00" {
		t.Fatalf("clearing date override did not restore rule: %+v err=%v", days, err)
	}
	interval, err := service.AddDayInterval(owner.id, date, "10:00", "11:00", KindFree)
	if err != nil {
		t.Fatalf("add one-date interval: %v", err)
	}
	if _, err = service.UpdateDayInterval(owner.id, interval.ID, "12:00", "13:00", KindBusy); err != nil {
		t.Fatalf("edit one-date interval: %v", err)
	}
	if _, err = service.AddDayInterval(owner.id, date, "12:30", "14:00", KindFree); err == nil {
		t.Fatal("overlapping interval was accepted")
	}
	settings := PrivacySettings{Visibility: VisibilitySelectedExact, Timezone: "Europe/Moscow", ExactFor: []string{friend.publicID}}
	if err = service.UpdatePrivacySettings(owner.id, settings); err != nil {
		t.Fatalf("save selected friend schedule privacy: %v", err)
	}
	friendDays, err := service.FriendsAvailability(friend.id, date, []string{owner.publicID})
	if err != nil || len(friendDays) != 1 || !friendDays[0].Exact || len(friendDays[0].Intervals) != 1 || friendDays[0].Intervals[0].Kind != KindBusy {
		t.Fatalf("granted friend did not see exact schedule: %+v err=%v", friendDays, err)
	}
	settings.Visibility = VisibilityBusyOnly
	settings.ExactFor = nil
	if err = service.UpdatePrivacySettings(owner.id, settings); err != nil {
		t.Fatal(err)
	}
	friendDays, err = service.FriendsAvailability(friend.id, date, []string{owner.publicID})
	if err != nil || friendDays[0].Exact || len(friendDays[0].Intervals) != 0 {
		t.Fatalf("busy-only privacy revealed interval details: %+v err=%v", friendDays, err)
	}
	if err = service.DeleteRule(owner.id, rule.ID); err != nil {
		t.Fatalf("disable recurring rule: %v", err)
	}
	if err = service.DeleteDayInterval(owner.id, interval.ID); err != nil {
		t.Fatalf("delete one-date interval: %v", err)
	}
}

type scheduleTestUser struct{ id, publicID string }

func createScheduleTestUser(t *testing.T, db *sql.DB, telegramID int64, name string) scheduleTestUser {
	t.Helper()
	var user scheduleTestUser
	if err := db.QueryRow(`INSERT INTO users(telegram_id,display_name) VALUES($1,$2) RETURNING id,public_id`, telegramID, name).Scan(&user.id, &user.publicID); err != nil {
		t.Fatal(err)
	}
	return user
}
func containsScheduleTest(value string) bool { return strings.Contains(strings.ToLower(value), "test") }
