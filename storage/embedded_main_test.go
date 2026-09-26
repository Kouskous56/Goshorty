package storage

import (
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
)

const (
	// embeddedPGDefaultPort avoids the 5432 range so the embedded server never
	// clashes with a real PostgreSQL that a developer may have running.
	embeddedPGDefaultPort = 15432
	embeddedPGUser        = "postgres"
	embeddedPGPassword    = "postgres"
	embeddedPGDatabase    = "postgres"
)

// TestMain lets the PostgreSQL integration suite run without Docker or an
// external server. With EMBEDDED_PG=1 it starts a single embedded PostgreSQL
// instance (the platform binary is downloaded from Maven Central on first
// use), points TEST_DATABASE_URL at it, and stops it once the package is done.
//
// CI keeps supplying TEST_DATABASE_URL against its service container and
// plain `go test` (no variables set) preserves the existing per-test skips,
// so the embedded path only activates when explicitly requested locally.
func TestMain(m *testing.M) {
	if os.Getenv("EMBEDDED_PG") != "1" {
		os.Exit(m.Run())
	}

	port := embeddedPGDefaultPort
	if raw := os.Getenv("EMBEDDED_PG_PORT"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 65535 {
			fmt.Fprintf(os.Stderr, "invalid EMBEDDED_PG_PORT %q\n", raw)
			os.Exit(1)
		}
		port = parsed
	}

	dataDir, err := os.MkdirTemp("", "goshorty-embedded-pg-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "embedded postgres data dir: %v\n", err)
		os.Exit(1)
	}

	pg := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Port(uint32(port)).
		DataPath(dataDir).
		StartParameters(map[string]string{"listen_addresses": "127.0.0.1"}).
		StartTimeout(60 * time.Second).
		Logger(os.Stderr),
	)
	if err := pg.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "embedded postgres start: %v\n", err)
		os.Exit(1)
	}

	testDatabaseURL := fmt.Sprintf("postgres://%s:%s@127.0.0.1:%d/%s?sslmode=disable",
		embeddedPGUser, embeddedPGPassword, port, embeddedPGDatabase)
	if err := os.Setenv("TEST_DATABASE_URL", testDatabaseURL); err != nil {
		fmt.Fprintf(os.Stderr, "set TEST_DATABASE_URL: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "embedded postgres ready at %s\n", testDatabaseURL)

	// os.Exit skips deferred functions, so stop and clean up explicitly.
	code := m.Run()
	if err := pg.Stop(); err != nil {
		fmt.Fprintf(os.Stderr, "embedded postgres stop: %v\n", err)
		code = 1
	}
	if err := os.RemoveAll(dataDir); err != nil {
		fmt.Fprintf(os.Stderr, "remove embedded postgres data dir: %v\n", err)
	}
	os.Exit(code)
}
