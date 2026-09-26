// Command localdb runs a persistent embedded PostgreSQL server for local
// development, with no Docker required. The platform Postgres binary is
// downloaded once from Maven Central into the user cache
// (~/.embedded-postgres-go) on first use.
//
// The data directory defaults to the current directory's .localdb/data and is
// kept across restarts, so `go run ./cmd/localdb` plus the main server with
// the printed DATABASE_URL gives a persistent local database. Press Ctrl+C to
// stop the server.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
)

func main() {
	port := flag.Uint("port", 5433, "TCP port for the embedded PostgreSQL server")
	data := flag.String("data", ".localdb/data", "directory for the persistent PostgreSQL data")
	username := flag.String("user", "goshorty", "database superuser name")
	password := flag.String("password", "goshorty", "database superuser password")
	database := flag.String("database", "goshorty", "database to create and expose")
	flag.Parse()

	dataDir, err := filepath.Abs(*data)
	if err != nil {
		log.Fatalf("resolve data directory: %v", err)
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		log.Fatalf("create data directory: %v", err)
	}

	pg := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Port(uint32(*port)).
		DataPath(dataDir).
		Username(*username).
		Password(*password).
		Database(*database).
		StartParameters(map[string]string{"listen_addresses": "127.0.0.1"}).
		StartTimeout(60 * time.Second).
		Logger(os.Stdout),
	)

	if err := pg.Start(); err != nil {
		log.Fatalf("start embedded postgres: %v", err)
	}
	defer func() {
		if err := pg.Stop(); err != nil {
			log.Printf("stop embedded postgres: %v", err)
		}
	}()

	connectionURL := fmt.Sprintf("postgres://%s:%s@127.0.0.1:%d/%s?sslmode=disable",
		*username, *password, *port, *database)
	fmt.Println("embedded PostgreSQL ready")
	fmt.Printf("DATABASE_URL=%s\n", connectionURL)
	fmt.Println("Press Ctrl+C to stop.")

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
}
