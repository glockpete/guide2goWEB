package handlers

import (
	"html/template"
	"net/http"
	"path/filepath"
	"github.com/gorilla/mux"
)

// Templates cache
var templates = template.Must(template.ParseGlob(filepath.Join("web", "templates", "*.html")))

// RegisterRoutes sets up the web routes and static file serving
func RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/", dashboardHandler)
	r.HandleFunc("/config", configHandler)
	r.HandleFunc("/api/config", configAPIHandler).Methods("GET", "POST")

	// Serve static files
	staticDir := http.Dir("web/static")
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(staticDir)))
}

// dashboardHandler renders the dashboard page
func dashboardHandler(w http.ResponseWriter, r *http.Request) {
	err := templates.ExecuteTemplate(w, "dashboard.html", nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// configHandler renders the config page
func configHandler(w http.ResponseWriter, r *http.Request) {
	err := templates.ExecuteTemplate(w, "config.html", nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// configAPIHandler handles GET/POST for config API
func configAPIHandler(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement config load/save logic
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"message": "Config API placeholder"}`))
} 