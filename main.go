package main

import (
	"log"
	"net/http"
	"os"

	"nadsas_voting_sys/db"
	"nadsas_voting_sys/handlers"
)

func main() {
	// Initialize Database
	db.InitDB("")

	// Setup Router
	r := handlers.SetupRouter()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server starting on port %s", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatal(err)
	}
}
