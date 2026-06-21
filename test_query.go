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
	godotenv.Load()
	connStr := os.Getenv("DATABASE_URL")
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	var count int
	email := "admin1@nadsas.gmail.com"
	err = db.QueryRow("SELECT COUNT(*) FROM admins WHERE email = $1", email).Scan(&count)
	if err != nil {
		log.Fatal("Query error: ", err)
	}
	fmt.Printf("Count: %d\n", count)
}
