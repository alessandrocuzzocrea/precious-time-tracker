package main

import (
	"database/sql"
	"log"
	"net/http"

	"github.com/pressly/goose/v3"
	"github.com/user/precious-time-tracker/internal/database"
	"github.com/user/precious-time-tracker/internal/server"
	"github.com/user/precious-time-tracker/sql/schema"
	_ "modernc.org/sqlite"
)

func main() {
	// Setup DB
	db, err := sql.Open("sqlite", "./tracker.db")
	if err != nil {
		log.Fatal(err)
	}
	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("Error closing database: %v", err)
		}
	}()

	// Run migrations
	goose.SetBaseFS(schema.FS)

	if err := goose.SetDialect("sqlite"); err != nil {
		log.Fatal(err)
	}

	if err := goose.Up(db, "."); err != nil {
		log.Fatal(err)
	}

	dbQueries := database.New(db)
	srv := server.NewServer(dbQueries, db)

	log.Println("Server starting on :8080")
	if err := http.ListenAndServe(":8080", srv); err != nil {
		log.Fatal(err)
	}
}
