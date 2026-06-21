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
	rows, err := DB.Query("SELECT id, group_id, title, description, start_time, end_time, duration_minutes, is_active FROM elections ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var elections []models.Election
	for rows.Next() {
		var e models.Election
		var start, end sql.NullTime
		var gid sql.NullInt64
		if err := rows.Scan(&e.ID, &gid, &e.Title, &e.Description, &start, &end, &e.DurationMinutes, &e.IsActive); err != nil {
			return nil, err
		}
		if gid.Valid { v := int(gid.Int64); e.GroupID = &v }
		if start.Valid { e.StartTime = start.Time.Local() }
		if end.Valid { e.EndTime = end.Time.Local() }
		DB.QueryRow("SELECT COUNT(*) FROM candidates WHERE election_id = $1", e.ID).Scan(&e.CandidateCount)
		DB.QueryRow("SELECT COUNT(*) FROM votes WHERE election_id = $1", e.ID).Scan(&e.TotalVotes)
		
		if !e.EndTime.IsZero() && time.Now().After(e.EndTime) {
			e.Status = "Ended"
		} else if !e.StartTime.IsZero() && time.Now().Before(e.StartTime) {
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
	err := DB.QueryRow("SELECT id, group_id, title, description, start_time, end_time, duration_minutes, is_active FROM elections WHERE id = $1", id).
		Scan(&e.ID, &gid, &e.Title, &e.Description, &start, &end, &e.DurationMinutes, &e.IsActive)
	if err != nil {
		return e, err
	}
	if gid.Valid { v := int(gid.Int64); e.GroupID = &v }
	if start.Valid { e.StartTime = start.Time.Local() }
	if end.Valid { e.EndTime = end.Time.Local() }
	DB.QueryRow("SELECT COUNT(*) FROM candidates WHERE election_id = $1", id).Scan(&e.CandidateCount)
	DB.QueryRow("SELECT COUNT(*) FROM votes WHERE election_id = $1", id).Scan(&e.TotalVotes)

	if !e.EndTime.IsZero() && time.Now().After(e.EndTime) {
		e.Status = "Ended"
	} else if !e.StartTime.IsZero() && time.Now().Before(e.StartTime) {
		e.Status = "Upcoming"
	} else if e.IsActive {
		e.Status = "Active"
	} else {
		e.Status = "Closed"
	}

	return e, nil
}

func GetCandidatesForElection(electionID int) ([]models.Candidate, error) {
	rows, err := DB.Query("SELECT id, election_id, name, position, manifesto, photo_url, vote_count FROM candidates WHERE election_id = $1 ORDER BY vote_count DESC", electionID)
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
	err = tx.QueryRow("SELECT is_active, start_time, end_time FROM elections WHERE id = $1", electionID).Scan(&isActive, &start, &end)
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
	err = tx.QueryRow("SELECT COUNT(*) FROM votes WHERE election_id = $1 AND voter_hash = $2", electionID, voterHash).Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		return errors.New("you have already voted in this election")
	}


	for _, cid := range candidateIDs {
		// Get position of candidate
		var position string
		err = tx.QueryRow("SELECT position FROM candidates WHERE id = $1 AND election_id = $2", cid, electionID).Scan(&position)
		if err != nil {
			return fmt.Errorf("invalid candidate: %v", err)
		}

		// Insert vote
		_, err = tx.Exec("INSERT INTO votes (election_id, candidate_id, position, voter_hash) VALUES ($1, $2, $3, $4)", electionID, cid, position, voterHash)
		if err != nil {
			return fmt.Errorf("database error recording vote: %v", err)
		}

		// Increment vote count
		_, err = tx.Exec("UPDATE candidates SET vote_count = vote_count + 1 WHERE id = $1", cid)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func GetAllElectionsAdmin() ([]models.Election, error) {
	rows, err := DB.Query("SELECT id, group_id, title, description, start_time, end_time, duration_minutes, is_active FROM elections ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var elections []models.Election
	for rows.Next() {
		var e models.Election
		var start, end sql.NullTime
		var gid sql.NullInt64
		if err := rows.Scan(&e.ID, &gid, &e.Title, &e.Description, &start, &end, &e.DurationMinutes, &e.IsActive); err != nil {
			return nil, err
		}
		if gid.Valid { v := int(gid.Int64); e.GroupID = &v }
		if start.Valid { e.StartTime = start.Time.Local() }
		if end.Valid { e.EndTime = end.Time.Local() }
		DB.QueryRow("SELECT COUNT(*) FROM candidates WHERE election_id = $1", e.ID).Scan(&e.CandidateCount)
		DB.QueryRow("SELECT COUNT(*) FROM votes WHERE election_id = $1", e.ID).Scan(&e.TotalVotes)
		if !e.EndTime.IsZero() && time.Now().After(e.EndTime) {
			e.Status = "Ended"
		} else if !e.StartTime.IsZero() && time.Now().Before(e.StartTime) {
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

func CreateElection(title, description string, groupID *int, durationMinutes int) (int, error) {
	var id int
	err := DB.QueryRow("INSERT INTO elections (group_id, title, description, duration_minutes, is_active) VALUES ($1, $2, $3, $4, FALSE) RETURNING id", groupID, title, description, durationMinutes).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func ToggleElectionStatus(id int) error {
	var isActive bool
	var durationMinutes int
	err := DB.QueryRow("SELECT is_active, duration_minutes FROM elections WHERE id = $1", id).Scan(&isActive, &durationMinutes)
	if err != nil {
		return err
	}

	if isActive {
		// Deactivate: set is_active=FALSE, end_time to now (ends immediately)
		_, err = DB.Exec("UPDATE elections SET is_active = FALSE, end_time = $1 WHERE id = $2", time.Now().UTC(), id)
	} else {
		// Activate: set is_active=TRUE, start_time=now, end_time=now + duration
		now := time.Now().UTC()
		end := now.Add(time.Duration(durationMinutes) * time.Minute)
		_, err = DB.Exec("UPDATE elections SET is_active = TRUE, start_time = $1, end_time = $2 WHERE id = $3", now, end, id)
	}
	return err
}

func AddCandidate(electionID int, name, position, manifesto, photoURL string) error {
	_, err := DB.Exec("INSERT INTO candidates (election_id, name, position, manifesto, photo_url) VALUES ($1, $2, $3, $4, $5)", electionID, name, position, manifesto, photoURL)
	return err
}

func DeleteCandidate(id int) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Remove votes for this candidate
	if _, err = tx.Exec("DELETE FROM votes WHERE candidate_id = $1", id); err != nil {
		return err
	}
	// Delete candidate
	if _, err = tx.Exec("DELETE FROM candidates WHERE id = $1", id); err != nil {
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
	if _, err = tx.Exec("DELETE FROM votes WHERE election_id = $1", id); err != nil {
		return err
	}
	if _, err = tx.Exec("DELETE FROM candidates WHERE election_id = $1", id); err != nil {
		return err
	}
	if _, err = tx.Exec("DELETE FROM elections WHERE id = $1", id); err != nil {
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
		erows, _ := DB.Query("SELECT id, group_id, title, description, start_time, end_time, duration_minutes, is_active FROM elections WHERE group_id = $1 ORDER BY created_at ASC", g.ID)
		if erows != nil {
			for erows.Next() {
				var e models.Election
				var start, end sql.NullTime
				var gid sql.NullInt64
				erows.Scan(&e.ID, &gid, &e.Title, &e.Description, &start, &end, &e.DurationMinutes, &e.IsActive)
				if gid.Valid { v := int(gid.Int64); e.GroupID = &v }
				if start.Valid { e.StartTime = start.Time.Local() }
				if end.Valid { e.EndTime = end.Time.Local() }
				DB.QueryRow("SELECT COUNT(*) FROM candidates WHERE election_id = $1", e.ID).Scan(&e.CandidateCount)
				DB.QueryRow("SELECT COUNT(*) FROM votes WHERE election_id = $1", e.ID).Scan(&e.TotalVotes)
				if !e.EndTime.IsZero() && time.Now().After(e.EndTime) {
					e.Status = "Ended"
				} else if !e.StartTime.IsZero() && time.Now().Before(e.StartTime) {
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
	err := DB.QueryRow("SELECT id, name, description FROM election_groups WHERE id = $1", id).
		Scan(&g.ID, &g.Name, &g.Description)
	if err != nil {
		return g, err
	}
	erows, _ := DB.Query("SELECT id, group_id, title, description, start_time, end_time, duration_minutes, is_active FROM elections WHERE group_id = $1 ORDER BY created_at ASC", g.ID)
	if erows != nil {
		for erows.Next() {
			var e models.Election
			var start, end sql.NullTime
			var gid sql.NullInt64
			erows.Scan(&e.ID, &gid, &e.Title, &e.Description, &start, &end, &e.DurationMinutes, &e.IsActive)
			if gid.Valid { v := int(gid.Int64); e.GroupID = &v }
			if start.Valid { e.StartTime = start.Time.Local() }
			if end.Valid { e.EndTime = end.Time.Local() }
			DB.QueryRow("SELECT COUNT(*) FROM candidates WHERE election_id = $1", e.ID).Scan(&e.CandidateCount)
			DB.QueryRow("SELECT COUNT(*) FROM votes WHERE election_id = $1", e.ID).Scan(&e.TotalVotes)
			if !e.EndTime.IsZero() && time.Now().After(e.EndTime) {
				e.Status = "Ended"
			} else if !e.StartTime.IsZero() && time.Now().Before(e.StartTime) {
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
	var id int
	err := DB.QueryRow("INSERT INTO election_groups (name, description) VALUES ($1, $2) RETURNING id", name, description).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func DeleteGroup(id int) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// NULL-out group_id on elections (don't delete them)
	if _, err = tx.Exec("UPDATE elections SET group_id = NULL WHERE group_id = $1", id); err != nil {
		return err
	}
	if _, err = tx.Exec("DELETE FROM election_groups WHERE id = $1", id); err != nil {
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
	if _, err = tx.Exec("DELETE FROM votes WHERE election_id = $1", electionID); err != nil {
		return err
	}
	if _, err = tx.Exec("UPDATE candidates SET vote_count = 0 WHERE election_id = $1", electionID); err != nil {
		return err
	}
	return tx.Commit()
}

// UpdateElection updates the basic details of an election
func UpdateElection(id int, title, description string, durationMinutes int, groupID *int) error {
	_, err := DB.Exec("UPDATE elections SET title = $1, description = $2, duration_minutes = $3, group_id = $4 WHERE id = $5", title, description, durationMinutes, groupID, id)
	return err
}

// ActivateAllElectionsInGroup sets all elections in a group to active and computes their start/end times
func ActivateAllElectionsInGroup(groupID int) error {
	rows, err := DB.Query("SELECT id, duration_minutes FROM elections WHERE group_id = $1", groupID)
	if err != nil {
		return err
	}
	defer rows.Close()

	type elec struct {
		id  int
		dur int
	}
	var elecs []elec
	for rows.Next() {
		var el elec
		if err := rows.Scan(&el.id, &el.dur); err != nil {
			return err
		}
		elecs = append(elecs, el)
	}

	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	for _, el := range elecs {
		end := now.Add(time.Duration(el.dur) * time.Minute)
		_, err = tx.Exec("UPDATE elections SET is_active = TRUE, start_time = $1, end_time = $2 WHERE id = $3", now, end, el.id)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeactivateAllElectionsInGroup deactivates all elections in a group and sets their end_time to now (ended)
func DeactivateAllElectionsInGroup(groupID int) error {
	_, err := DB.Exec("UPDATE elections SET is_active = FALSE, end_time = $1 WHERE group_id = $2", time.Now().UTC(), groupID)
	return err
}
