package db

import (
	"database/sql"
	"log"
	"os"

	"golang.org/x/crypto/bcrypt"
	_ "github.com/lib/pq"
)

var DB *sql.DB

func InitDB(defaultConn string) {
	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		log.Fatalf("CRITICAL ERROR: DATABASE_URL environment variable is not set. You must set DATABASE_URL to your PostgreSQL connection string to run the application.")
	}

	var err error
	DB, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}

	if err = DB.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}

	// PostgreSQL supports ADD COLUMN IF NOT EXISTS
	_, _ = DB.Exec("ALTER TABLE elections ADD COLUMN IF NOT EXISTS duration_minutes INTEGER DEFAULT 0")

	createTables()
	seedAdmins()
}

func createTables() {
	schema, err := os.ReadFile("db/schema.sql")
	if err != nil {
		log.Fatalf("Failed to read schema.sql: %v", err)
	}

	_, err = DB.Exec(string(schema))
	if err != nil {
		log.Fatalf("Failed to execute schema.sql: %v", err)
	}
}

func seedAdmins() {
	admin1Pass := os.Getenv("ADMIN1_PASSWORD")
	if admin1Pass == "" {
		admin1Pass = "admin123" // Fallback for dev
	}
	admin2Pass := os.Getenv("ADMIN2_PASSWORD")
	if admin2Pass == "" {
		admin2Pass = "admin123" // Fallback for dev
	}

	seedAdmin("admin1@nadsas.gmail.com", admin1Pass)
	seedAdmin("admin2@nadsas.gmail.com", admin2Pass)
}

func seedAdmin(email, password string) {
	var count int
	err := DB.QueryRow("SELECT COUNT(*) FROM admins WHERE email = $1", email).Scan(&count)
	if err != nil {
		log.Fatalf("Failed to check admin %s: %v", email, err)
	}

	if count == 0 {
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			log.Fatalf("Failed to hash password for %s: %v", email, err)
		}

		_, err = DB.Exec("INSERT INTO admins (email, password_hash) VALUES ($1, $2)", email, string(hash))
		if err != nil {
			log.Fatalf("Failed to seed admin %s: %v", email, err)
		}
		log.Printf("Seeded admin: %s", email)
	}
}
