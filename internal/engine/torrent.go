package engine

import (
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"tango/internal/aria2"
	"tango/internal/database"
)

type TangoDownload struct {
	status *aria2.Status
	meta   database.DownloadMeta
}

type DownloadInfo struct {
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	InfoHash      string        `json:"info_hash"`
	Status        string        `json:"status"`
	Progress      float64       `json:"progress"`
	Size          int64         `json:"size"`
	Downloaded    int64         `json:"downloaded"`
	Uploaded      int64         `json:"uploaded"`
	DownloadSpeed int           `json:"download_speed"`
	UploadSpeed   int           `json:"upload_speed"`
	ETA           *int          `json:"eta"`
	Peers         int           `json:"peers"`
	Seeders       int           `json:"seeders"`
	SeedRatio     float64       `json:"seed_ratio"`
	Label         string        `json:"label"`
	TargetPath    string        `json:"target_path"`
	SaveDir       string        `json:"save_dir"`
	Moved         bool          `json:"moved"`
	AddedAt       time.Time     `json:"added_at"`
	Files         []FileInfo    `json:"files,omitempty"`
	PeerList      []PeerInfo    `json:"peer_list,omitempty"`
	TrackerList   []TrackerInfo `json:"tracker_list,omitempty"`
}

type FileInfo struct {
	Path      string  `json:"path"`
	Size      int64   `json:"size"`
	Completed int64   `json:"completed"`
	Progress  float64 `json:"progress"`
}

type PeerInfo struct {
	Addr          string `json:"addr"`
	DownloadSpeed int    `json:"download_speed"`
	UploadSpeed   int    `json:"upload_speed"`
	Seeder        bool   `json:"seeder"`
}

type TrackerInfo struct {
	URL string `json:"url"`
}

func (d *TangoDownload) Info() DownloadInfo {
	s := d.status
	total, _ := strconv.ParseInt(s.TotalLength, 10, 64)
	completed, _ := strconv.ParseInt(s.CompletedLength, 10, 64)
	uploaded, _ := strconv.ParseInt(s.UploadLength, 10, 64)
	dlSpeed, _ := strconv.Atoi(s.DownloadSpeed)
	ulSpeed, _ := strconv.Atoi(s.UploadSpeed)
	connections, _ := strconv.Atoi(s.Connections)
	seeders, _ := strconv.Atoi(s.NumSeeders)

	info := DownloadInfo{
		ID:            s.GID,
		Name:          downloadName(s),
		InfoHash:      s.InfoHash,
		Status:        mapStatus(s),
		Size:          total,
		Downloaded:    completed,
		Uploaded:      uploaded,
		DownloadSpeed: dlSpeed,
		UploadSpeed:   ulSpeed,
		Peers:         connections,
		Seeders:       seeders,
		Label:         d.meta.Label,
		TargetPath:    d.meta.TargetPath,
		SaveDir:       s.Dir,
		Moved:         d.meta.Moved,
		AddedAt:       d.meta.AddedAt,
	}

	if total > 0 {
		info.Progress = float64(completed) / float64(total)
	}

	// Seed ratio
	if completed > 0 {
		info.SeedRatio = float64(uploaded) / float64(completed)
	}

	// ETA calculation
	if dlSpeed > 0 && total > completed {
		eta := int(total-completed) / dlSpeed
		info.ETA = &eta
	}

	return info
}

func (d *TangoDownload) InfoWithDetails(peers []aria2.Peer) DownloadInfo {
	info := d.Info()
	info.Files = d.fileInfos()
	info.TrackerList = d.trackerInfos()
	info.PeerList = mapPeers(peers)
	return info
}

func mapPeers(peers []aria2.Peer) []PeerInfo {
	if len(peers) == 0 {
		return nil
	}
	result := make([]PeerInfo, len(peers))
	for i, p := range peers {
		dl, _ := strconv.Atoi(p.DownloadSpeed)
		ul, _ := strconv.Atoi(p.UploadSpeed)
		result[i] = PeerInfo{
			Addr:          p.IP + ":" + p.Port,
			DownloadSpeed: dl,
			UploadSpeed:   ul,
			Seeder:        p.Seeder == "true",
		}
	}
	return result
}

func (d *TangoDownload) fileInfos() []FileInfo {
	dir := d.status.Dir
	var files []FileInfo
	for _, f := range d.status.Files {
		size, _ := strconv.ParseInt(f.Length, 10, 64)
		completed, _ := strconv.ParseInt(f.CompletedLength, 10, 64)
		var progress float64
		if size > 0 {
			progress = float64(completed) / float64(size)
		}
		// aria2c returns absolute paths; make them relative to save dir
		// so Reel (and other consumers) don't double the prefix
		path := f.Path
		if dir != "" {
			path = strings.TrimPrefix(path, filepath.Clean(dir)+"/")
		}
		files = append(files, FileInfo{
			Path:      path,
			Size:      size,
			Completed: completed,
			Progress:  progress,
		})
	}
	return files
}

func (d *TangoDownload) trackerInfos() []TrackerInfo {
	if d.status.BitTorrent == nil {
		return nil
	}
	var trackers []TrackerInfo
	for _, tier := range d.status.BitTorrent.AnnounceList {
		for _, url := range tier {
			trackers = append(trackers, TrackerInfo{URL: url})
		}
	}
	return trackers
}

// downloadName extracts the torrent name from status.
func downloadName(s *aria2.Status) string {
	if s.BitTorrent != nil && s.BitTorrent.Info != nil && s.BitTorrent.Info.Name != "" {
		return s.BitTorrent.Info.Name
	}
	// For non-torrent downloads, use first file name
	if len(s.Files) > 0 && s.Files[0].Path != "" {
		return s.Files[0].Path
	}
	return s.GID
}

// mapStatus maps aria2c status strings to user-friendly status names.
func mapStatus(s *aria2.Status) string {
	if s.Seeder == "true" {
		return "Seeding"
	}
	switch s.Status {
	case "active":
		return "Downloading"
	case "waiting":
		return "Waiting"
	case "paused":
		return "Stopped"
	case "error":
		return "Error"
	case "complete":
		return "Complete"
	case "removed":
		return "Removed"
	default:
		return s.Status
	}
}
