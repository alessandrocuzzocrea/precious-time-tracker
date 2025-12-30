package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"

	"github.com/user/precious-time-tracker/internal/database"
	"github.com/user/precious-time-tracker/internal/server"
	_ "modernc.org/sqlite"
)

func main() {
	// Setup DB
	db, err := sql.Open("sqlite", "./tracker.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Create table if not exists (rudimentary migration for v1)
	schema, err := os.ReadFile("./sql/schema/001_users_and_entries.sql")
	if err != nil {
		log.Fatal(err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		// Ignore error if table exists?
		// "table time_entries already exists"
		log.Printf("Schema exec warning (might already exist): %v", err)
	}

	dbQueries := database.New(db)
	srv := server.NewServer(dbQueries)

	log.Println("Server starting on :8080")
	if err := http.ListenAndServe(":8080", srv); err != nil {
		log.Fatal(err)
	}
}
