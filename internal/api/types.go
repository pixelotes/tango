package api

type AddTorrentRequest struct {
	URI        string `json:"uri"`
	TargetPath string `json:"target_path"`
	Label      string `json:"label"`
}

type HealthResponse struct {
	Status   string `json:"status"`
	Version  string `json:"version"`
	Torrents int    `json:"torrents"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}
