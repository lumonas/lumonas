package main

import (
	"flag"
	"log/slog"
	"os"

	"github.com/lumonas/lumonas/internal/store"
)

var defaultDatabasePath = "/var/lib/lumonas/lumonas.db"

func migrate(databasePath string) error {
	database, err := store.Open(databasePath)
	if err != nil {
		return err
	}
	return database.Close()
}

func main() {
	databasePath := flag.String("db", envOr("LUMONAS_DB_PATH", defaultDatabasePath), "SQLite database path")
	flag.Parse()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := migrate(*databasePath); err != nil {
		logger.Error("database migration failed", "db", *databasePath, "error", err)
		os.Exit(1)
	}
	logger.Info("database migrations applied", "db", *databasePath)
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
