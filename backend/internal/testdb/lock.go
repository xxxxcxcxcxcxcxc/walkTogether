package testdb

import (
	"context"
	"database/sql"
	"testing"
)

// LockSerializesDestructiveIntegrationTests prevents different Go packages from
// truncating the same isolated test database at the same time.
func LockSerializesDestructiveIntegrationTests(t *testing.T, db *sql.DB) {
	t.Helper()
	connection, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("open PostgreSQL integration lock connection: %v", err)
	}
	const key int64 = 746271908341
	if _, err = connection.ExecContext(context.Background(), `SELECT pg_advisory_lock($1)`, key); err != nil {
		_ = connection.Close()
		t.Fatalf("acquire PostgreSQL integration lock: %v", err)
	}
	t.Cleanup(func() {
		_, _ = connection.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, key)
		_ = connection.Close()
	})
}
