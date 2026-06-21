package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"

	"nadsas_voting_sys/db"
	"nadsas_voting_sys/handlers"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	db.InitDB("")

	// Create test election
	electionID, err := db.CreateElection("Agent Test Election", "Testing image upload", nil, 60)
	if err != nil {
		log.Fatalf("Failed to create election: %v", err)
	}
	defer func() {
		// Clean up after test
		_ = db.DeleteElection(electionID)
	}()

	// Add test candidate with a valid base64 1x1 png image
	testImage := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII="
	err = db.AddCandidate(electionID, "Test Candidate", "Tester", "Just testing", testImage)
	if err != nil {
		log.Fatalf("Failed to add candidate: %v", err)
	}

	// Activate the election so the voting page actually renders the candidates
	err = db.ToggleElectionStatus(electionID)
	if err != nil {
		log.Fatalf("Failed to activate election: %v", err)
	}

	// Setup router to test
	r := handlers.SetupRouter()

	// Create a test request for the voting page which is public
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/election/%d", electionID), nil)
	w := httptest.NewRecorder()

	// Serve the request
	r.ServeHTTP(w, req)

	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		log.Fatalf("Expected status 200, got %d", res.StatusCode)
	}

	bodyBytes, _ := io.ReadAll(res.Body)
	body := string(bodyBytes)

	// Check if the image tag was rendered correctly without unsafe: prefix
	if strings.Contains(body, "unsafe:data:image") {
		fmt.Println("TEST FAILED: Found 'unsafe:' prefix in image src.")
		os.Exit(1)
	}

	if !strings.Contains(body, "src=\"data:image/png;base64,iVBORw0") {
		fmt.Println("TEST FAILED: Image source not found in HTML. Body preview:")
		// print where data:image is or the entire body
		idx := strings.Index(body, "data:image")
		if idx != -1 {
			start := idx - 50
			if start < 0 { start = 0 }
			end := idx + 100
			if end > len(body) { end = len(body) }
			fmt.Println(body[start:end])
		} else {
			fmt.Println("No data:image found in body at all.")
		}
		os.Exit(1)
	}

	fmt.Println("TEST PASSED: Image was rendered successfully without 'unsafe:' prefix.")
}
