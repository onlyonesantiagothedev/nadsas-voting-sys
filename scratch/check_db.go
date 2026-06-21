package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func main() {
	_ = godotenv.Load()
	connStr := os.Getenv("DATABASE_URL")
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatalf("Open err: %v", err)
	}
	defer db.Close()

	var columnName, dataType string
	var charMaxLength sql.NullInt64
	err = db.QueryRow(`
		SELECT column_name, data_type, character_maximum_length 
		FROM information_schema.columns 
		WHERE table_name = 'candidates' AND column_name = 'photo_url'
	`).Scan(&columnName, &dataType, &charMaxLength)
	if err != nil {
		log.Fatalf("Query err: %v", err)
	}

	fmt.Printf("Column: %s, Type: %s, MaxLength: %v\n", columnName, dataType, charMaxLength)
}
