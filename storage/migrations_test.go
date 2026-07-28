package storage

import (
	"strings"
	"testing"
)

func TestInitialMigrationIsEmbedded(t *testing.T) {
	migration, err := migrationFiles.ReadFile("migrations/001_init.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(migration)
	for _, required := range []string{
		"CREATE TABLE IF NOT EXISTS users",
		"CREATE TABLE IF NOT EXISTS urls",
		"UNIQUE",
		"REFERENCES users(id) ON DELETE CASCADE",
		"idx_urls_expires_at",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("initial migration is missing %q", required)
		}
	}
}
