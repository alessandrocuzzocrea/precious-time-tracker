package main

import (
	"database/sql"
	"log"
	"net/http"

	"github.com/pressly/goose/v3"
	"github.com/alessandrocuzzocrea/precious-time-tracker/internal/database"
	"github.com/alessandrocuzzocrea/precious-time-tracker/internal/server"
	"github.com/alessandrocuzzocrea/precious-time-tracker/internal/service"
	"github.com/alessandrocuzzocrea/precious-time-tracker/sql/schema"
	_ "modernc.org/sqlite"
)

func main() {
	// Setup DB
	db, err := sql.Open("sqlite", "./precious-time-tracker.sqlite3")
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
	svc := service.New(dbQueries, db)
	srv := server.NewServer(svc)

	log.Println("Server starting on :8080")
	if err := http.ListenAndServe(":8080", srv); err != nil {
		log.Fatal(err)
	}
}
