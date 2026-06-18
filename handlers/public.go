package handlers

import (
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"time"

	"nadsas_voting_sys/db"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/csrf"
	"nadsas_voting_sys/models"
)

func HomeHandler(w http.ResponseWriter, r *http.Request) {
	elections, err := db.GetActiveElections()
	if err != nil {
		http.Error(w, "Failed to load elections", http.StatusInternalServerError)
		return
	}

	success := r.URL.Query().Get("success") == "1"

	data := map[string]interface{}{
		"Elections": elections,
		"Success":   success,
	}
	renderTemplate(w, "index.html", data)
}

func VotingPageHandler(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, _ := strconv.Atoi(idStr)

	election, err := db.GetElection(id)
	if err != nil {
		http.Error(w, "Election not found", http.StatusNotFound)
		return
	}

	candidates, err := db.GetCandidatesForElection(id)
	if err != nil {
		http.Error(w, "Failed to load candidates", http.StatusInternalServerError)
		return
	}

	grouped := make(map[string][]models.Candidate)
	for _, c := range candidates {
		grouped[c.Position] = append(grouped[c.Position], c)
	}

	hasVoted := false
	if _, err := r.Cookie(fmt.Sprintf("voted_election_%d", id)); err == nil {
		hasVoted = true
	}

	isUpcoming := false
	if !election.StartTime.IsZero() && time.Now().Before(election.StartTime) {
		isUpcoming = true
	}

	hasEnded := false
	if !isUpcoming {
		if !election.IsActive || (!election.EndTime.IsZero() && time.Now().After(election.EndTime)) {
			hasEnded = true
		}
	}

	data := map[string]interface{}{
		"Election":       election,
		"CandidatesByPos": grouped,
		"HasVoted":       hasVoted,
		"HasEnded":       hasEnded,
		"IsUpcoming":     isUpcoming,
		csrf.TemplateTag: csrf.TemplateField(r),
	}
	renderTemplate(w, "voting.html", data)
}

func SubmitVoteHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	electionID, _ := strconv.Atoi(r.FormValue("election_id"))
	identifier := r.FormValue("voter_identifier")
	cookieName := fmt.Sprintf("voted_election_%d", electionID)

	w.Header().Set("Content-Type", "text/html")

	election, err := db.GetElection(electionID)
	if err != nil {
		fmt.Fprintf(w, "<div class='mt-4 p-3 bg-red-100 text-red-700 rounded'>Error: Election not found.</div>")
		return
	}
	if !election.StartTime.IsZero() && time.Now().Before(election.StartTime) {
		fmt.Fprintf(w, "<div class='mt-4 p-3 bg-red-100 text-red-700 rounded'>Error: Voting for this election has not started yet.</div>")
		return
	}
	if !election.IsActive || (!election.EndTime.IsZero() && time.Now().After(election.EndTime)) {
		fmt.Fprintf(w, "<div class='mt-4 p-3 bg-red-100 text-red-700 rounded'>Error: Voting for this election has ended.</div>")
		return
	}

	if _, err := r.Cookie(cookieName); err == nil {
		fmt.Fprintf(w, "<div class='mt-4 p-3 bg-red-100 text-red-700 rounded'>Error: This device has already been used to vote in this election.</div>")
		return
	}

	matched, _ := regexp.MatchString(`^DSA/\d{4}/\d+$`, identifier)
	if !matched {
		fmt.Fprintf(w, "<div class='mt-4 p-3 bg-red-100 text-red-700 rounded'>Error: Invalid Voter ID format. Must be like DSA/2023/1234.</div>")
		return
	}

	var candidateIDs []int
	for key, values := range r.Form {
		// Use len > 0 to check safely, though strings.HasPrefix is main check
		if len(key) > 5 && key[:5] == "role_" && len(values) > 0 {
			cid, _ := strconv.Atoi(values[0])
			if cid > 0 {
				candidateIDs = append(candidateIDs, cid)
			}
		}
	}

	if len(candidateIDs) == 0 {
		fmt.Fprintf(w, "<div class='mt-4 p-3 bg-red-100 text-red-700 rounded'>Error: You must select at least one candidate.</div>")
		return
	}

	pepper := os.Getenv("VOTER_HASH_PEPPER")
	if pepper == "" {
		pepper = "default-pepper"
	}

	err = db.RecordVotes(electionID, candidateIDs, identifier, pepper)
	if err != nil {
		fmt.Fprintf(w, "<div class='mt-4 p-3 bg-red-100 text-red-700 rounded'>Error: %s</div>", err.Error())
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "true",
		Path:     "/",
		Expires:  time.Now().Add(24 * time.Hour), // 24 hour cooldown
		HttpOnly: true,
	})

	w.Header().Set("HX-Redirect", "/?success=1")
	w.WriteHeader(http.StatusOK)
}

func ResultsPageHandler(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, _ := strconv.Atoi(idStr)

	election, err := db.GetElection(id)
	if err != nil {
		http.Error(w, "Election not found", http.StatusNotFound)
		return
	}

	data := map[string]interface{}{
		"Election": election,
	}
	renderTemplate(w, "results.html", data)
}

func ResultsFragmentHandler(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, _ := strconv.Atoi(idStr)

	election, _ := db.GetElection(id)
	candidates, _ := db.GetCandidatesForElection(id)

	for i, c := range candidates {
		if election.TotalVotes > 0 {
			candidates[i].VotePercentage = float64(c.VoteCount) / float64(election.TotalVotes) * 100
		} else {
			candidates[i].VotePercentage = 0
		}
	}

	// Check if requester is a logged-in admin
	session, _ := store.Get(r, "admin-session")
	isAdmin, _ := session.Values["authenticated"].(bool)
	
	showResults := false
	if isAdmin {
		showResults = true
	} else if !election.IsActive || time.Now().After(election.EndTime) {
		// Results are public once admin closes/deactivates the election or if the end time has passed
		showResults = true
	}

	data := map[string]interface{}{
		"Election":    election,
		"Candidates":  candidates,
		"ShowResults": showResults,
		"IsAdmin":     isAdmin,
	}
	renderTemplate(w, "results_fragment.html", data)
}
