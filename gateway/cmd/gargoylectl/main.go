// Command gargoylectl is a small operator CLI for managing Gargoyle client
// records directly against Postgres, ahead of the HTTP admin API.
// Its only job is create-client: generate a fresh
// API key, persist its hash, and print the plaintext key exactly once.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"gargoyle/internal/client"
	"gargoyle/internal/config"
	"gargoyle/internal/db"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "create-client":
		err = runCreateClient(os.Args[2:])
	default:
		printUsage()
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "gargoylectl:", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `usage: gargoylectl create-client -name <name> -target <url> [-rate-limit <n>] [-plan <tier>]

Reads GARGOYLE_DATABASE_URL the same way the gargoyle server does (see
internal/config).`)
}

func runCreateClient(args []string) error {
	fs := flag.NewFlagSet("create-client", flag.ExitOnError)
	name := fs.String("name", "", "client name (required)")
	target := fs.String("target", "", "upstream backend URL, e.g. http://localhost:9001 (required)")
	rateLimit := fs.Int("rate-limit", 60, "requests per minute")
	planTier := fs.String("plan", "free", "plan tier: free, pro, enterprise")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" || *target == "" {
		return fmt.Errorf("-name and -target are required")
	}

	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		return err
	}

	apiKey, err := client.GenerateAPIKey()
	if err != nil {
		return err
	}

	store := client.NewPostgresStore(pool)
	created, err := store.CreateClient(ctx, client.NewClientParams{
		Name:       *name,
		APIKeyHash: client.HashAPIKey(apiKey),
		TargetURL:  *target,
		RateLimit:  *rateLimit,
		PlanTier:   *planTier,
	})
	if err != nil {
		return err
	}

	fmt.Printf("Created client %q (id=%s, target=%s, rate_limit=%d, plan=%s)\n",
		created.Name, created.ID, created.TargetURL, created.RateLimit, created.PlanTier,
	)
	fmt.Println()
	fmt.Println("API key (shown once — store it securely, Gargoyle only keeps its hash):")
	fmt.Println(apiKey)

	return nil
}
