package db

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"nadsas_voting_sys/models"
	"time"
)

// Helper for voter hashing
func HashVoter(identifier, pepper string) string {
	hasher := sha256.New()
	hasher.Write([]byte(identifier + pepper))
	return hex.EncodeToString(hasher.Sum(nil))
}

func GetActiveElections() ([]models.Election, error) {
	rows, err := DB.Query("SELECT id, group_id, title, description, start_time, end_time, is_active FROM elections ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var elections []models.Election
	for rows.Next() {
		var e models.Election
		var start, end sql.NullTime
		var gid sql.NullInt64
		if err := rows.Scan(&e.ID, &gid, &e.Title, &e.Description, &start, &end, &e.IsActive); err != nil {
			return nil, err
		}
		if gid.Valid { v := int(gid.Int64); e.GroupID = &v }
		if start.Valid { e.StartTime = start.Time.Local() }
		if end.Valid { e.EndTime = end.Time.Local() }
		DB.QueryRow("SELECT COUNT(*) FROM candidates WHERE election_id = ?", e.ID).Scan(&e.CandidateCount)
		DB.QueryRow("SELECT COUNT(*) FROM votes WHERE election_id = ?", e.ID).Scan(&e.TotalVotes)
		
		if end.Valid && !end.Time.IsZero() && time.Now().After(end.Time) {
			e.Status = "Ended"
		} else if start.Valid && !start.Time.IsZero() && time.Now().Before(start.Time) {
			e.Status = "Upcoming"
		} else if e.IsActive {
			e.Status = "Active"
		} else {
			e.Status = "Closed"
		}
		
		if e.Status != "Closed" {
			elections = append(elections, e)
		}
	}
	return elections, nil
}

func GetElection(id int) (models.Election, error) {
	var e models.Election
	var start, end sql.NullTime
	var gid sql.NullInt64
	err := DB.QueryRow("SELECT id, group_id, title, description, start_time, end_time, is_active FROM elections WHERE id = ?", id).
		Scan(&e.ID, &gid, &e.Title, &e.Description, &start, &end, &e.IsActive)
	if err != nil {
		return e, err
	}
	if gid.Valid { v := int(gid.Int64); e.GroupID = &v }
	if start.Valid { e.StartTime = start.Time.Local() }
	if end.Valid { e.EndTime = end.Time.Local() }
	DB.QueryRow("SELECT COUNT(*) FROM votes WHERE election_id = ?", id).Scan(&e.TotalVotes)
	return e, nil
}

func GetCandidatesForElection(electionID int) ([]models.Candidate, error) {
	rows, err := DB.Query("SELECT id, election_id, name, position, manifesto, photo_url, vote_count FROM candidates WHERE election_id = ? ORDER BY vote_count DESC", electionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidates []models.Candidate
	for rows.Next() {
		var c models.Candidate
		var photo sql.NullString
		if err := rows.Scan(&c.ID, &c.ElectionID, &c.Name, &c.Position, &c.Manifesto, &photo, &c.VoteCount); err != nil {
			return nil, err
		}
		if photo.Valid {
			c.PhotoURL = photo.String
		}
		candidates = append(candidates, c)
	}
	return candidates, nil
}

func RecordVotes(electionID int, candidateIDs []int, voterIdentifier, pepper string) error {
	voterHash := HashVoter(voterIdentifier, pepper)
	
	// Start transaction
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Check if active and within start/end time
	var isActive bool
	var start, end sql.NullTime
	err = tx.QueryRow("SELECT is_active, start_time, end_time FROM elections WHERE id = ?", electionID).Scan(&isActive, &start, &end)
	if err != nil {
		return err
	}
	if !isActive {
		return errors.New("this election is not currently open for voting")
	}
	if start.Valid && time.Now().Before(start.Time) {
		return errors.New("this election has not started yet")
	}
	if end.Valid && time.Now().After(end.Time) {
		return errors.New("this election has already ended")
	}

	// Check if voter has ANY votes in this election
	var count int
	err = tx.QueryRow("SELECT COUNT(*) FROM votes WHERE election_id = ? AND voter_hash = ?", electionID, voterHash).Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		return errors.New("you have already voted in this election")
	}

	// Global 24-hour cooldown check for this voter across all elections
	var lastVoted sql.NullTime
	err = tx.QueryRow("SELECT MAX(voted_at) FROM votes WHERE voter_hash = ?", voterHash).Scan(&lastVoted)
	if err == nil && lastVoted.Valid {
		if time.Since(lastVoted.Time) < 24*time.Hour {
			return errors.New("this Voter ID has already been used in the last 24 hours and is currently locked")
		}
	}

	for _, cid := range candidateIDs {
		// Get position of candidate
		var position string
		err = tx.QueryRow("SELECT position FROM candidates WHERE id = ? AND election_id = ?", cid, electionID).Scan(&position)
		if err != nil {
			return fmt.Errorf("invalid candidate: %v", err)
		}

		// Insert vote
		_, err = tx.Exec("INSERT INTO votes (election_id, candidate_id, position, voter_hash) VALUES (?, ?, ?, ?)", electionID, cid, position, voterHash)
		if err != nil {
			return fmt.Errorf("database error recording vote: %v", err)
		}

		// Increment vote count
		_, err = tx.Exec("UPDATE candidates SET vote_count = vote_count + 1 WHERE id = ?", cid)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func GetAllElectionsAdmin() ([]models.Election, error) {
	rows, err := DB.Query("SELECT id, group_id, title, description, start_time, end_time, is_active FROM elections ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var elections []models.Election
	for rows.Next() {
		var e models.Election
		var start, end sql.NullTime
		var gid sql.NullInt64
		if err := rows.Scan(&e.ID, &gid, &e.Title, &e.Description, &start, &end, &e.IsActive); err != nil {
			return nil, err
		}
		if gid.Valid { v := int(gid.Int64); e.GroupID = &v }
		if start.Valid { e.StartTime = start.Time.Local() }
		if end.Valid { e.EndTime = end.Time.Local() }
		DB.QueryRow("SELECT COUNT(*) FROM candidates WHERE election_id = ?", e.ID).Scan(&e.CandidateCount)
		DB.QueryRow("SELECT COUNT(*) FROM votes WHERE election_id = ?", e.ID).Scan(&e.TotalVotes)
		if end.Valid && !end.Time.IsZero() && time.Now().After(end.Time) {
			e.Status = "Ended"
		} else if start.Valid && !start.Time.IsZero() && time.Now().Before(start.Time) {
			e.Status = "Upcoming"
		} else if e.IsActive {
			e.Status = "Active"
		} else {
			e.Status = "Closed"
		}
		elections = append(elections, e)
	}
	return elections, nil
}

func CreateElection(title, description string, groupID *int, start, end *time.Time) (int, error) {
	var startVal, endVal interface{}
	if start != nil { startVal = *start }
	if end != nil { endVal = *end }
	res, err := DB.Exec("INSERT INTO elections (group_id, title, description, start_time, end_time, is_active) VALUES (?, ?, ?, ?, ?, 0)", groupID, title, description, startVal, endVal)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return int(id), nil
}

func ToggleElectionStatus(id int) error {
	_, err := DB.Exec("UPDATE elections SET is_active = NOT is_active WHERE id = ?", id)
	return err
}

func AddCandidate(electionID int, name, position, manifesto, photoURL string) error {
	_, err := DB.Exec("INSERT INTO candidates (election_id, name, position, manifesto, photo_url) VALUES (?, ?, ?, ?, ?)", electionID, name, position, manifesto, photoURL)
	return err
}

func DeleteCandidate(id int) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Remove votes for this candidate
	if _, err = tx.Exec("DELETE FROM votes WHERE candidate_id = ?", id); err != nil {
		return err
	}
	// Recalculate vote_count for sibling candidates (not strictly needed but safe)
	if _, err = tx.Exec("DELETE FROM candidates WHERE id = ?", id); err != nil {
		return err
	}
	return tx.Commit()
}

func DeleteElection(id int) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("DELETE FROM votes WHERE election_id = ?", id); err != nil {
		return err
	}
	if _, err = tx.Exec("DELETE FROM candidates WHERE election_id = ?", id); err != nil {
		return err
	}
	if _, err = tx.Exec("DELETE FROM elections WHERE id = ?", id); err != nil {
		return err
	}
	return tx.Commit()
}

// ── Election Group functions ─────────────────────────────────────────────────

func GetAllGroups() ([]models.ElectionGroup, error) {
	rows, err := DB.Query("SELECT id, name, description FROM election_groups ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []models.ElectionGroup
	for rows.Next() {
		var g models.ElectionGroup
		if err := rows.Scan(&g.ID, &g.Name, &g.Description); err != nil {
			return nil, err
		}
		// Load elections inside group
		erows, _ := DB.Query("SELECT id, group_id, title, description, start_time, end_time, is_active FROM elections WHERE group_id = ? ORDER BY created_at ASC", g.ID)
		if erows != nil {
			for erows.Next() {
				var e models.Election
				var start, end sql.NullTime
				var gid sql.NullInt64
				erows.Scan(&e.ID, &gid, &e.Title, &e.Description, &start, &end, &e.IsActive)
				if gid.Valid { v := int(gid.Int64); e.GroupID = &v }
				if start.Valid { e.StartTime = start.Time.Local() }
				if end.Valid { e.EndTime = end.Time.Local() }
				if end.Valid && !end.Time.IsZero() && time.Now().After(end.Time) {
					e.Status = "Ended"
				} else if start.Valid && !start.Time.IsZero() && time.Now().Before(start.Time) {
					e.Status = "Upcoming"
				} else if e.IsActive {
					e.Status = "Active"
				} else {
					e.Status = "Closed"
				}
				g.Elections = append(g.Elections, e)
			}
			erows.Close()
		}
		groups = append(groups, g)
	}
	return groups, nil
}

func GetGroup(id int) (models.ElectionGroup, error) {
	var g models.ElectionGroup
	err := DB.QueryRow("SELECT id, name, description FROM election_groups WHERE id = ?", id).
		Scan(&g.ID, &g.Name, &g.Description)
	if err != nil {
		return g, err
	}
	erows, _ := DB.Query("SELECT id, group_id, title, description, start_time, end_time, is_active FROM elections WHERE group_id = ? ORDER BY created_at ASC", g.ID)
	if erows != nil {
		for erows.Next() {
			var e models.Election
			var start, end sql.NullTime
			var gid sql.NullInt64
			erows.Scan(&e.ID, &gid, &e.Title, &e.Description, &start, &end, &e.IsActive)
			if gid.Valid { v := int(gid.Int64); e.GroupID = &v }
			if start.Valid { e.StartTime = start.Time.Local() }
			if end.Valid { e.EndTime = end.Time.Local() }
			if end.Valid && !end.Time.IsZero() && time.Now().After(end.Time) {
				e.Status = "Ended"
			} else if start.Valid && !start.Time.IsZero() && time.Now().Before(start.Time) {
				e.Status = "Upcoming"
			} else if e.IsActive {
				e.Status = "Active"
			} else {
				e.Status = "Closed"
			}
			g.Elections = append(g.Elections, e)
		}
		erows.Close()
	}
	return g, nil
}

func CreateGroup(name, description string) (int, error) {
	res, err := DB.Exec("INSERT INTO election_groups (name, description) VALUES (?, ?)", name, description)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return int(id), nil
}

func DeleteGroup(id int) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// NULL-out group_id on elections (don't delete them)
	if _, err = tx.Exec("UPDATE elections SET group_id = NULL WHERE group_id = ?", id); err != nil {
		return err
	}
	if _, err = tx.Exec("DELETE FROM election_groups WHERE id = ?", id); err != nil {
		return err
	}
	return tx.Commit()
}

// ResetVotes clears all votes for an election and resets candidate vote counts to 0.
func ResetVotes(electionID int) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("DELETE FROM votes WHERE election_id = ?", electionID); err != nil {
		return err
	}
	if _, err = tx.Exec("UPDATE candidates SET vote_count = 0 WHERE election_id = ?", electionID); err != nil {
		return err
	}
	return tx.Commit()
}
