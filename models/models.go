package models

import "time"

type ElectionGroup struct {
	ID          int        `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	CreatedAt   time.Time  `json:"created_at"`
	Elections   []Election `json:"elections"`
}

type Election struct {
	ID              int       `json:"id"`
	GroupID         *int      `json:"group_id"`
	Title           string    `json:"title"`
	Description     string    `json:"description"`
	StartTime       time.Time `json:"start_time"`
	EndTime         time.Time `json:"end_time"`
	DurationMinutes int       `json:"duration_minutes"`
	IsActive        bool      `json:"is_active"`
	CreatedAt       time.Time `json:"created_at"`

	// Computed fields for UI
	Status         string `json:"status"`
	CandidateCount int    `json:"candidate_count"`
	TotalVotes     int    `json:"total_votes"`
}

type Candidate struct {
	ID         int    `json:"id"`
	ElectionID int    `json:"election_id"`
	Name       string `json:"name"`
	Position   string `json:"position"`
	Manifesto  string `json:"manifesto"`
	PhotoURL   string `json:"photo_url"`
	VoteCount  int    `json:"vote_count"`

	// Computed field for UI
	VotePercentage float64 `json:"vote_percentage"`
}

type Vote struct {
	ID          int       `json:"id"`
	ElectionID  int       `json:"election_id"`
	CandidateID int       `json:"candidate_id"`
	VoterHash   string    `json:"voter_hash"`
	VotedAt     time.Time `json:"voted_at"`
}

type Admin struct {
	ID           int       `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	LastLogin    time.Time `json:"last_login"`
}

