package main

import (
	"database/sql"
	"log/slog"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func main() {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		slog.Error("DATABASE_URL is required")
		os.Exit(1)
	}

	command := "up"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	migrationsDir := "migrations"
	if len(os.Args) > 2 {
		migrationsDir = os.Args[2]
	}
	var args []string
	if len(os.Args) > 3 {
		args = os.Args[3:]
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		slog.Error("open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := goose.SetDialect("postgres"); err != nil {
		slog.Error("set goose dialect", "error", err)
		os.Exit(1)
	}
	if err := goose.Run(command, db, migrationsDir, args...); err != nil {
		slog.Error("run migrations", "command", command, "error", err)
		os.Exit(1)
	}
}
