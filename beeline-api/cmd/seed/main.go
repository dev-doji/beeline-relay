// Command seed creates a workspace with one console key and one agent key, and prints the
// keys once. Keys are stored only as hashes; save the printed values somewhere safe.
//
//	go run ./cmd/seed -name "Demo workspace"
//	go run ./cmd/seed -name "Chaos sandbox" -sandbox
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/joho/godotenv"

	"github.com/dev-doji/beeline-relay/beeline-api/internal/auth"
	"github.com/dev-doji/beeline-relay/beeline-api/internal/storage"
)

func main() {
	name := flag.String("name", "Demo workspace", "workspace name")
	sandbox := flag.Bool("sandbox", false, "create a sandbox workspace (simulated payments only)")
	flag.Parse()

	_ = godotenv.Load()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		fail("DATABASE_URL is required")
	}

	ctx := context.Background()
	pool, err := storage.Open(ctx, dbURL, nil)
	if err != nil {
		fail(err.Error())
	}
	defer pool.Close()
	if err := storage.Migrate(pool); err != nil {
		fail(err.Error())
	}

	store := &auth.Store{DB: pool}
	ws, err := store.CreateWorkspace(ctx, *name, *sandbox)
	if err != nil {
		fail(err.Error())
	}
	_, consoleKey, err := store.CreateKey(ctx, ws, auth.KindConsole, "console")
	if err != nil {
		fail(err.Error())
	}
	_, agentKey, err := store.CreateKey(ctx, ws, auth.KindAgent, "reference-agent")
	if err != nil {
		fail(err.Error())
	}

	fmt.Printf("workspace:   %s (sandbox=%v)\n", ws, *sandbox)
	fmt.Printf("console key: %s\n", consoleKey)
	fmt.Printf("agent key:   %s\n", agentKey)
	fmt.Println("These keys are shown once. Store them in your local .env files, never in git.")
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, "seed:", msg)
	os.Exit(1)
}
