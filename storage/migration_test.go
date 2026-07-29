package storage

import "testing"

func TestMigrationChecksumIsDeterministicAndSensitive(t *testing.T) {
	first := migrationChecksum([]byte("SELECT 1;\n"))
	if len(first) != 64 {
		t.Fatalf("expected SHA-256 hex digest, got %q", first)
	}
	if first != migrationChecksum([]byte("SELECT 1;\n")) {
		t.Fatal("same migration must have the same checksum")
	}
	if first == migrationChecksum([]byte("SELECT 2;\n")) {
		t.Fatal("different migrations must not have the same checksum")
	}
}
