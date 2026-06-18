package handlers

import (
	"html/template"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/csrf"
	"github.com/gorilla/sessions"
)

var (
	tmpls = make(map[string]*template.Template)
	store *sessions.CookieStore
)

func renderTemplate(w http.ResponseWriter, name string, data map[string]interface{}) {
	t, ok := tmpls[name]
	if !ok {
		http.Error(w, "Template not found", http.StatusInternalServerError)
		return
	}
	if name == "results_fragment.html" || name == "admin_print.html" {
		err := t.ExecuteTemplate(w, name, data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	err := t.ExecuteTemplate(w, "base.html", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// SetupRouter initializes and returns the Chi router
func SetupRouter() *chi.Mux {
	funcMap := template.FuncMap{
		"add1": func(x int) int {
			return x + 1
		},
	}

	// Parse templates separately so they don't overwrite the "content" block
	tmpls["index.html"] = template.Must(template.New("index.html").Funcs(funcMap).ParseFiles("templates/base.html", "templates/index.html"))
	tmpls["voting.html"] = template.Must(template.New("voting.html").Funcs(funcMap).ParseFiles("templates/base.html", "templates/voting.html"))
	tmpls["results.html"] = template.Must(template.New("results.html").Funcs(funcMap).ParseFiles("templates/base.html", "templates/results.html"))
	tmpls["results_fragment.html"] = template.Must(template.New("results_fragment.html").Funcs(funcMap).ParseFiles("templates/results_fragment.html"))
	tmpls["admin_login.html"] = template.Must(template.New("admin_login.html").Funcs(funcMap).ParseFiles("templates/base.html", "templates/admin_login.html"))
	tmpls["admin_dashboard.html"] = template.Must(template.New("admin_dashboard.html").Funcs(funcMap).ParseFiles("templates/base.html", "templates/admin_dashboard.html"))
	tmpls["admin_new_election.html"] = template.Must(template.New("admin_new_election.html").Funcs(funcMap).ParseFiles("templates/base.html", "templates/admin_new_election.html"))
	tmpls["admin_election.html"] = template.Must(template.New("admin_election.html").Funcs(funcMap).ParseFiles("templates/base.html", "templates/admin_election.html"))
	tmpls["admin_new_group.html"] = template.Must(template.New("admin_new_group.html").Funcs(funcMap).ParseFiles("templates/base.html", "templates/admin_new_group.html"))
	tmpls["admin_group.html"] = template.Must(template.New("admin_group.html").Funcs(funcMap).ParseFiles("templates/base.html", "templates/admin_group.html"))
	tmpls["admin_print.html"] = template.Must(template.New("admin_print.html").Funcs(funcMap).ParseFiles("templates/admin_print.html"))

	// Setup session store
	secret := os.Getenv("SESSION_SECRET")
	if secret == "" {
		secret = "super-secret-key-change-in-production"
	}
	store = sessions.NewCookieStore([]byte(secret))
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   86400, // 24 hours
		HttpOnly: true,
	}

	r := chi.NewRouter()

	// Middlewares
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// CSRF protection
	csrfMiddleware := csrf.Protect(
		[]byte(secret[:32]), // CSRF needs 32 byte key
		csrf.Secure(false),  // Set true if using HTTPS
		csrf.Path("/"),
		csrf.TrustedOrigins([]string{"localhost:8080", "127.0.0.1:8080"}),
	)
	r.Use(csrfMiddleware)

	// Static files
	fileServer := http.FileServer(http.Dir("./static"))
	r.Handle("/static/*", http.StripPrefix("/static/", fileServer))

	// Public Routes
	r.Get("/", HomeHandler)
	r.Get("/election/{id}", VotingPageHandler)
	r.Post("/vote", SubmitVoteHandler)
	r.Get("/results/{id}", ResultsPageHandler)
	r.Get("/results/{id}/fragment", ResultsFragmentHandler)

	// Admin Routes
	r.Route("/admin", func(r chi.Router) {
		r.Get("/login", AdminLoginHandler)
		r.Post("/login", AdminLoginPostHandler)
		
		// Protected Admin Routes
		r.Group(func(r chi.Router) {
			r.Use(RequireAuth)
			r.Get("/", AdminDashboardGroupsHandler)
			r.Get("/logout", AdminLogoutHandler)

			// Group routes
			r.Get("/group/new", AdminNewGroupHandler)
			r.Post("/group/new", AdminNewGroupPostHandler)
			r.Get("/group/{id}", AdminManageGroupHandler)
			r.Post("/group/{id}/delete", AdminDeleteGroupHandler)

			// Election routes
			r.Get("/election/new", AdminNewElectionHandler)
			r.Post("/election/new", AdminNewElectionPostHandler)
			r.Get("/election/{id}", AdminManageElectionHandler)
			r.Get("/election/{id}/print", AdminPrintElectionHandler)
			r.Post("/election/{id}/toggle", AdminToggleElectionHandler)
			r.Post("/election/{id}/delete", AdminDeleteElectionHandler)
			r.Post("/election/{id}/reset-votes", AdminResetVotesHandler)
			r.Get("/election/{id}/candidates", AdminManageCandidatesHandler)
			r.Post("/election/{id}/candidates", AdminAddCandidateHandler)
			r.Post("/election/{id}/candidates/{cid}/delete", AdminDeleteCandidateHandler)
		})
	})

	return r
}

// RequireAuth middleware checks for a valid admin session
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, _ := store.Get(r, "admin-session")
		if auth, ok := session.Values["authenticated"].(bool); !ok || !auth {
			http.Redirect(w, r, "/admin/login", http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}
