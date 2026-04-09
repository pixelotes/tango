package api

import (
	"net/http"

	"tango/internal/config"
	"tango/internal/engine"
	"tango/web"

	"github.com/gorilla/mux"
)

func NewRouter(e *engine.Engine, cfg config.Config) http.Handler {
	r := mux.NewRouter()
	h := NewHandler(e)

	// API routes
	api := r.PathPrefix("/api").Subrouter()
	api.Use(corsMiddleware)

	api.HandleFunc("/health", h.Health).Methods("GET")
	api.HandleFunc("/stats", h.GetStats).Methods("GET")

	api.HandleFunc("/torrents", h.ListTorrents).Methods("GET")
	api.HandleFunc("/torrents", h.AddTorrent).Methods("POST")
	api.HandleFunc("/torrents/file", h.AddTorrentFile).Methods("POST")
	api.HandleFunc("/torrents/{id}", h.GetTorrent).Methods("GET")
	api.HandleFunc("/torrents/{id}", h.DeleteTorrent).Methods("DELETE")

	api.HandleFunc("/torrents/{id}/start", h.StartTorrent).Methods("POST")
	api.HandleFunc("/torrents/{id}/stop", h.StopTorrent).Methods("POST")

	api.HandleFunc("/torrents/{id}/files", h.GetFiles).Methods("GET")
	api.HandleFunc("/torrents/{id}/peers", h.GetPeers).Methods("GET")
	api.HandleFunc("/torrents/{id}/trackers", h.GetTrackers).Methods("GET")
	api.HandleFunc("/torrents/{id}/trackers", h.AddTracker).Methods("POST")

	// Handle CORS preflight
	api.Methods("OPTIONS").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// Web UI — serve embedded static files (optional)
	if cfg.WebUIEnabled {
		r.PathPrefix("/").Handler(web.FileServer())
	}

	return r
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
