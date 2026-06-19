package handlers

import (
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"nadsas_voting_sys/db"
	"nadsas_voting_sys/models"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/csrf"
	"golang.org/x/crypto/bcrypt"
)

func AdminLoginHandler(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{
		csrf.TemplateTag: csrf.TemplateField(r),
	}
	renderTemplate(w, "admin_login.html", data)
}

func AdminLoginPostHandler(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	email := r.FormValue("email")
	password := r.FormValue("password")

	var hash string
	err = db.DB.QueryRow("SELECT password_hash FROM admins WHERE email = $1", email).Scan(&hash)
	if err != nil {
		if err == sql.ErrNoRows {
			data := map[string]interface{}{"Error": "Invalid credentials", csrf.TemplateTag: csrf.TemplateField(r)}
			renderTemplate(w, "admin_login.html", data)
			return
		}
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		data := map[string]interface{}{"Error": "Invalid credentials", csrf.TemplateTag: csrf.TemplateField(r)}
		renderTemplate(w, "admin_login.html", data)
		return
	}

	// Update last login
	db.DB.Exec("UPDATE admins SET last_login = $1 WHERE email = $2", time.Now(), email)

	// Set session
	session, _ := store.Get(r, "admin-session")
	session.Values["authenticated"] = true
	session.Values["email"] = email
	session.Save(r, w)

	http.Redirect(w, r, "/admin", http.StatusFound)
}

func AdminLogoutHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, "admin-session")
	session.Values["authenticated"] = false
	session.Save(r, w)
	http.Redirect(w, r, "/admin/login", http.StatusFound)
}

func AdminDashboardHandler(w http.ResponseWriter, r *http.Request) {
	elections, _ := db.GetAllElectionsAdmin()
	
	var totalElections, activeElections, totalVotes int
	for _, e := range elections {
		totalElections++
		if e.IsActive {
			activeElections++
		}
		totalVotes += e.TotalVotes
	}

	data := map[string]interface{}{
		"Elections":       elections,
		"TotalElections":  totalElections,
		"ActiveElections": activeElections,
		"TotalVotes":      totalVotes,
		csrf.TemplateTag:  csrf.TemplateField(r),
	}
	renderTemplate(w, "admin_dashboard.html", data)
}

func AdminNewElectionHandler(w http.ResponseWriter, r *http.Request) {
	groups, _ := db.GetAllGroups()
	data := map[string]interface{}{
		"Groups":          groups,
		csrf.TemplateTag: csrf.TemplateField(r),
	}
	renderTemplate(w, "admin_new_election.html", data)
}

func AdminNewElectionPostHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	title := r.FormValue("title")
	desc := r.FormValue("description")
	durationHoursStr := r.FormValue("duration_hours")
	durationMinutesStr := r.FormValue("duration_minutes")

	var durationMinutes int
	if h, err := strconv.Atoi(durationHoursStr); err == nil && h >= 0 {
		durationMinutes += h * 60
	}
	if m, err := strconv.Atoi(durationMinutesStr); err == nil && m >= 0 {
		durationMinutes += m
	}
	if durationMinutes <= 0 {
		durationMinutes = 60 // Fallback to 1 hour
	}

	groupIDStr := r.FormValue("group_id")
	var groupID *int
	if groupIDStr != "" {
		if gid, err := strconv.Atoi(groupIDStr); err == nil {
			groupID = &gid
		}
	}

	id, err := db.CreateElection(title, desc, groupID, durationMinutes)
	if err != nil {
		http.Error(w, "Failed to create election", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/admin/election/%d", id), http.StatusFound)
}

func AdminManageElectionHandler(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, _ := strconv.Atoi(idStr)

	election, _ := db.GetElection(id)
	candidates, _ := db.GetCandidatesForElection(id)
	groups, _ := db.GetAllGroups()

	// Parse duration back into hours and minutes
	hours := election.DurationMinutes / 60
	minutes := election.DurationMinutes % 60

	var groupIDVal int
	if election.GroupID != nil {
		groupIDVal = *election.GroupID
	}

	data := map[string]interface{}{
		"Election":        election,
		"Candidates":      candidates,
		"Groups":          groups,
		"GroupIDVal":      groupIDVal,
		"DurationHours":   hours,
		"DurationMinutes": minutes,
		csrf.TemplateTag:  csrf.TemplateField(r),
	}
	renderTemplate(w, "admin_election.html", data)
}

func AdminToggleElectionHandler(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, _ := strconv.Atoi(idStr)
	db.ToggleElectionStatus(id)
	
	// Could return HTMX fragment or redirect. Since standard form submit might be used:
	http.Redirect(w, r, fmt.Sprintf("/admin/election/%d", id), http.StatusFound)
}

func AdminManageCandidatesHandler(w http.ResponseWriter, r *http.Request) {
	// Not strictly necessary as a separate GET if we manage them on the election page,
	// but keeping it if needed for full page edits.
}

func AdminAddCandidateHandler(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, _ := strconv.Atoi(idStr)
	
	r.ParseMultipartForm(5 << 20) // 5MB max

	name := r.FormValue("name")
	position := r.FormValue("position")
	manifesto := r.FormValue("manifesto")
	
	var photoURL string
	file, handler, err := r.FormFile("photo")
	if err == nil {
		defer file.Close()
		ext := filepath.Ext(handler.Filename)
		filename := fmt.Sprintf("candidate_%d_%d%s", id, time.Now().UnixNano(), ext)
		savePath := filepath.Join("static", "img", "candidates", filename)
		
		dst, err := os.Create(savePath)
		if err == nil {
			defer dst.Close()
			io.Copy(dst, file)
			photoURL = "/static/img/candidates/" + filename
		}
	}

	db.AddCandidate(id, name, position, manifesto, photoURL)
	http.Redirect(w, r, fmt.Sprintf("/admin/election/%d", id), http.StatusFound)
}

func AdminDeleteCandidateHandler(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	cidStr := chi.URLParam(r, "cid")
	cid, _ := strconv.Atoi(cidStr)

	err := db.DeleteCandidate(cid)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/admin/election/"+idStr, http.StatusFound)
}

func AdminDeleteElectionHandler(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, _ := strconv.Atoi(idStr)

	err := db.DeleteElection(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/admin", http.StatusFound)
}

// ── Group Handlers ────────────────────────────────────────────────────────────

func AdminNewGroupHandler(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{
		csrf.TemplateTag: csrf.TemplateField(r),
	}
	renderTemplate(w, "admin_new_group.html", data)
}

func AdminNewGroupPostHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	name := r.FormValue("name")
	desc := r.FormValue("description")
	id, err := db.CreateGroup(name, desc)
	if err != nil {
		http.Error(w, "Failed to create group", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/admin/group/%d", id), http.StatusFound)
}

func AdminManageGroupHandler(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, _ := strconv.Atoi(idStr)
	group, err := db.GetGroup(id)
	if err != nil {
		http.Error(w, "Group not found", http.StatusNotFound)
		return
	}
	// Load all groups for dropdown in new-election form
	groups, _ := db.GetAllGroups()
	data := map[string]interface{}{
		"Group":         group,
		"AllGroups":     groups,
		csrf.TemplateTag: csrf.TemplateField(r),
	}
	renderTemplate(w, "admin_group.html", data)
}

func AdminDeleteGroupHandler(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, _ := strconv.Atoi(idStr)
	if err := db.DeleteGroup(id); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/admin", http.StatusFound)
}

func AdminDashboardGroupsHandler(w http.ResponseWriter, r *http.Request) {
	groups, _ := db.GetAllGroups()
	standaloneElections, _ := db.GetAllElectionsAdmin()
	// Filter to only ungrouped elections
	var ungrouped []interface{}
	for _, e := range standaloneElections {
		if e.GroupID == nil {
			ungrouped = append(ungrouped, e)
		}
	}

	var totalElections, activeElections, totalVotes int
	for _, g := range groups {
		for _, e := range g.Elections {
			totalElections++
			if e.IsActive { activeElections++ }
			totalVotes += e.TotalVotes
		}
	}
	for _, e := range standaloneElections {
		if e.GroupID == nil {
			totalElections++
			if e.IsActive { activeElections++ }
			totalVotes += e.TotalVotes
		}
	}

	data := map[string]interface{}{
		"Groups":          groups,
		"Ungrouped":       ungrouped,
		"TotalElections":  totalElections,
		"ActiveElections": activeElections,
		"TotalVotes":      totalVotes,
		csrf.TemplateTag:  csrf.TemplateField(r),
	}
	renderTemplate(w, "admin_dashboard.html", data)
}

func AdminResetVotesHandler(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, _ := strconv.Atoi(idStr)
	if err := db.ResetVotes(id); err != nil {
		http.Error(w, "Failed to reset votes: "+err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/admin/election/%d", id), http.StatusFound)
}

func AdminPrintElectionHandler(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, _ := strconv.Atoi(idStr)

	election, err := db.GetElection(id)
	if err != nil {
		http.Error(w, "Election not found", http.StatusNotFound)
		return
	}

	candidates, _ := db.GetCandidatesForElection(id)

	for i, c := range candidates {
		if election.TotalVotes > 0 {
			candidates[i].VotePercentage = float64(c.VoteCount) / float64(election.TotalVotes) * 100
		} else {
			candidates[i].VotePercentage = 0
		}
	}

	data := map[string]interface{}{
		"Election":   election,
		"Candidates": candidates,
	}

	renderTemplate(w, "admin_print.html", data)
}

func AdminEditElectionPostHandler(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, _ := strconv.Atoi(idStr)

	r.ParseForm()
	title := r.FormValue("title")
	desc := r.FormValue("description")
	durationHoursStr := r.FormValue("duration_hours")
	durationMinutesStr := r.FormValue("duration_minutes")

	var durationMinutes int
	if h, err := strconv.Atoi(durationHoursStr); err == nil && h >= 0 {
		durationMinutes += h * 60
	}
	if m, err := strconv.Atoi(durationMinutesStr); err == nil && m >= 0 {
		durationMinutes += m
	}
	if durationMinutes <= 0 {
		durationMinutes = 60
	}

	groupIDStr := r.FormValue("group_id")
	var groupID *int
	if groupIDStr != "" {
		if gid, err := strconv.Atoi(groupIDStr); err == nil {
			groupID = &gid
		}
	}

	err := db.UpdateElection(id, title, desc, durationMinutes, groupID)
	if err != nil {
		http.Error(w, "Failed to update election: "+err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/admin/election/%d", id), http.StatusFound)
}

func AdminGroupActivateAllHandler(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, _ := strconv.Atoi(idStr)

	err := db.ActivateAllElectionsInGroup(id)
	if err != nil {
		http.Error(w, "Failed to activate elections: "+err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/admin/group/%d", id), http.StatusFound)
}

func AdminGroupDeactivateAllHandler(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, _ := strconv.Atoi(idStr)

	err := db.DeactivateAllElectionsInGroup(id)
	if err != nil {
		http.Error(w, "Failed to deactivate elections: "+err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/admin/group/%d", id), http.StatusFound)
}

func AdminPrintGroupHandler(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, _ := strconv.Atoi(idStr)

	group, err := db.GetGroup(id)
	if err != nil {
		http.Error(w, "Group not found", http.StatusNotFound)
		return
	}

	type ElectionWithCandidates struct {
		models.Election
		Candidates []models.Candidate
	}
	var dataElections []ElectionWithCandidates

	for _, e := range group.Elections {
		candidates, _ := db.GetCandidatesForElection(e.ID)
		for i, c := range candidates {
			if e.TotalVotes > 0 {
				candidates[i].VotePercentage = float64(c.VoteCount) / float64(e.TotalVotes) * 100
			} else {
				candidates[i].VotePercentage = 0
			}
		}
		dataElections = append(dataElections, ElectionWithCandidates{
			Election:   e,
			Candidates: candidates,
		})
	}

	data := map[string]interface{}{
		"Group":     group,
		"Elections": dataElections,
	}

	renderTemplate(w, "admin_group_print.html", data)
}
