package api

import (
	"encoding/json"
	"net/http"

	"tango/internal/engine"

	"github.com/gorilla/mux"
)

const Version = "0.1.0"

type Handler struct {
	engine *engine.Engine
}

func NewHandler(e *engine.Engine) *Handler {
	return &Handler{engine: e}
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	stats := h.engine.GetStats()
	writeJSON(w, http.StatusOK, HealthResponse{
		Status:   "ok",
		Version:  Version,
		Torrents: stats.TorrentCount,
	})
}

func (h *Handler) GetStats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.engine.GetStats())
}

func (h *Handler) ListTorrents(w http.ResponseWriter, r *http.Request) {
	downloads := h.engine.ListTorrents()
	result := make([]engine.DownloadInfo, 0, len(downloads))
	for _, d := range downloads {
		result = append(result, d.Info())
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) AddTorrent(w http.ResponseWriter, r *http.Request) {
	var req AddTorrentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.URI == "" {
		writeError(w, http.StatusBadRequest, "uri is required")
		return
	}

	d, err := h.engine.AddTorrent(req.URI, req.TargetPath, req.Label)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, d.Info())
}

func (h *Handler) AddTorrentFile(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file field is required")
		return
	}
	defer file.Close()

	targetPath := r.FormValue("target_path")
	label := r.FormValue("label")

	d, err := h.engine.AddTorrentFile(file, targetPath, label)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, d.Info())
}

func (h *Handler) GetTorrent(w http.ResponseWriter, r *http.Request) {
	gid := mux.Vars(r)["id"]
	d, err := h.engine.GetTorrent(gid)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	peers, _ := h.engine.GetPeers(gid)
	writeJSON(w, http.StatusOK, d.InfoWithDetails(peers))
}

func (h *Handler) DeleteTorrent(w http.ResponseWriter, r *http.Request) {
	gid := mux.Vars(r)["id"]
	deleteData := r.URL.Query().Get("delete_data") == "true"

	if err := h.engine.RemoveTorrent(gid, deleteData); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) StartTorrent(w http.ResponseWriter, r *http.Request) {
	gid := mux.Vars(r)["id"]
	if err := h.engine.StartTorrent(gid); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) StopTorrent(w http.ResponseWriter, r *http.Request) {
	gid := mux.Vars(r)["id"]
	if err := h.engine.StopTorrent(gid); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) GetFiles(w http.ResponseWriter, r *http.Request) {
	gid := mux.Vars(r)["id"]
	d, err := h.engine.GetTorrent(gid)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	info := d.InfoWithDetails(nil)
	writeJSON(w, http.StatusOK, info.Files)
}

func (h *Handler) GetPeers(w http.ResponseWriter, r *http.Request) {
	gid := mux.Vars(r)["id"]
	peers, err := h.engine.GetPeers(gid)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, peers)
}

func (h *Handler) AddTracker(w http.ResponseWriter, r *http.Request) {
	// Tango injects trackers automatically; accept the request as a no-op for Storm API compatibility.
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) GetTrackers(w http.ResponseWriter, r *http.Request) {
	gid := mux.Vars(r)["id"]
	d, err := h.engine.GetTorrent(gid)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	info := d.InfoWithDetails(nil)
	writeJSON(w, http.StatusOK, info.TrackerList)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, ErrorResponse{Error: msg})
}
