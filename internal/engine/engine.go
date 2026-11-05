package engine

import (
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"
	"time"

	"tango/internal/aria2"
	"tango/internal/config"
	"tango/internal/database"
	"tango/internal/trackerlist"
)

type Engine struct {
	client      *aria2.Client
	process     *aria2.Process
	db          *database.DB
	cfg         config.Config
	trackerList *trackerlist.List
	stopCh      chan struct{}
}

// SetTrackerList enables public tracker injection for new downloads.
func (e *Engine) SetTrackerList(tl *trackerlist.List) {
	e.trackerList = tl
}

// injectTrackers merges public tracker URLs into the aria2c download options.
func (e *Engine) injectTrackers(opts map[string]interface{}) {
	if e.trackerList == nil {
		return
	}
	trackers := e.trackerList.Trackers()
	if len(trackers) == 0 {
		return
	}
	opts["bt-tracker"] = strings.Join(trackers, ",")
	log.Printf("Injected %d public trackers into download", len(trackers))
}

func New(cfg config.Config, db *database.DB) (*Engine, error) {
	// Create aria2 process config
	rpcPort := 6800 // Extract from URL if needed
	procCfg := aria2.ProcessConfig{
		BinPath:        cfg.Aria2.BinPath,
		RPCPort:        rpcPort,
		RPCSecret:      cfg.Aria2.RPCSecret,
		RPCListenAll:   cfg.Aria2.RPCListenAll,
		DownloadDir:    cfg.DataDir,
		ListenPort:     cfg.Aria2.ListenPort,
		MaxConcurrent:  cfg.Aria2.MaxConcurrent,
		MaxConnPerSrv:  cfg.Aria2.MaxConnectionPerServer,
		DHTEnabled:     cfg.Aria2.DHTEnabled,
		PEXEnabled:     cfg.Aria2.PEXEnabled,
		MaxUploadSpeed: cfg.Aria2.MaxUploadSpeed,
		MaxDownSpeed:   cfg.Aria2.MaxDownloadSpeed,
		DiskCache:      cfg.Aria2.DiskCache,
		SeedRatio:      cfg.Seeding.TargetRatio,
	}

	proc := aria2.NewProcess(procCfg)
	if err := proc.Start(); err != nil {
		return nil, fmt.Errorf("starting aria2c: %w", err)
	}

	client := aria2.NewClient(cfg.Aria2.RPCUrl, cfg.Aria2.RPCSecret)

	// Wait for aria2c to be ready
	if err := proc.WaitReady(client, 10*time.Second); err != nil {
		proc.Stop()
		return nil, fmt.Errorf("aria2c not ready: %w", err)
	}

	ver, _ := client.GetVersion()
	if ver != nil {
		log.Printf("aria2c %s ready", ver.Version)
	}

	e := &Engine{
		client:  client,
		process: proc,
		db:      db,
		cfg:     cfg,
		stopCh:  make(chan struct{}),
	}

	// Start background monitors
	go e.seedingMonitor()
	go e.completionMonitor()
	go e.watchProcess()

	return e, nil
}

func (e *Engine) Close() error {
	close(e.stopCh)
	return e.process.Stop()
}

func (e *Engine) AddTorrent(uri string, targetPath string, label string) (*TangoDownload, error) {
	opts := map[string]interface{}{}
	if targetPath != "" {
		opts["dir"] = targetPath
	}
	e.injectTrackers(opts)

	gid, err := e.client.AddURI([]string{uri}, opts)
	if err != nil {
		return nil, fmt.Errorf("adding download: %w", err)
	}

	meta := database.DownloadMeta{
		GID:        gid,
		TargetPath: targetPath,
		Label:      label,
		AddedAt:    time.Now(),
	}
	if err := e.db.SaveDownload(meta); err != nil {
		e.client.ForceRemove(gid)
		return nil, fmt.Errorf("saving download metadata: %w", err)
	}

	return e.getDownload(gid)
}

func (e *Engine) AddTorrentFile(r io.Reader, targetPath string, label string) (*TangoDownload, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("reading torrent file: %w", err)
	}

	encoded := base64.StdEncoding.EncodeToString(data)
	opts := map[string]interface{}{}
	if targetPath != "" {
		opts["dir"] = targetPath
	}
	e.injectTrackers(opts)

	gid, err := e.client.AddTorrent(encoded, opts)
	if err != nil {
		return nil, fmt.Errorf("adding torrent file: %w", err)
	}

	meta := database.DownloadMeta{
		GID:        gid,
		TargetPath: targetPath,
		Label:      label,
		AddedAt:    time.Now(),
	}
	if err := e.db.SaveDownload(meta); err != nil {
		e.client.ForceRemove(gid)
		return nil, fmt.Errorf("saving download metadata: %w", err)
	}

	return e.getDownload(gid)
}

func (e *Engine) GetTorrent(gid string) (*TangoDownload, error) {
	return e.getDownload(gid)
}

func (e *Engine) ListTorrents() []*TangoDownload {
	var downloads []*TangoDownload

	// Gather all downloads from aria2c
	active, _ := e.client.TellActive()
	waiting, _ := e.client.TellWaiting(0, 1000)
	stopped, _ := e.client.TellStopped(0, 1000)

	all := append(active, waiting...)
	all = append(all, stopped...)

	for i := range all {
		s := &all[i]
		// Skip metadata-only downloads (magnet link metadata phase)
		if len(s.FollowedBy) > 0 {
			e.db.SetContentGID(s.GID, s.FollowedBy[0])
			continue
		}
		meta, err := e.db.GetDownload(s.GID)
		if err != nil {
			// Inherit metadata from parent if this is a followed download
			if s.Following != "" {
				meta = e.inheritMeta(s.Following, s.GID, s.Dir)
			} else {
				meta = database.DownloadMeta{
					GID:        s.GID,
					TargetPath: s.Dir,
					AddedAt:    time.Now(),
				}
			}
		}
		downloads = append(downloads, &TangoDownload{status: s, meta: meta})
	}

	return downloads
}

func (e *Engine) RemoveTorrent(gid string, deleteData bool) error {
	// Try to remove from aria2c (active or waiting)
	if _, err := e.client.Remove(gid); err != nil {
		// Maybe it's already stopped — try to clean the result
		e.client.RemoveDownloadResult(gid)
	}

	if deleteData {
		// Get status to find download directory
		status, err := e.client.TellStatus(gid)
		if err == nil && status.Dir != "" {
			removeDownloadFiles(status)
		}
	}

	e.db.DeleteDownload(gid)
	return nil
}

func (e *Engine) StartTorrent(gid string) error {
	_, err := e.client.Unpause(gid)
	return err
}

func (e *Engine) StopTorrent(gid string) error {
	_, err := e.client.Pause(gid)
	return err
}

type SessionStats struct {
	TorrentCount  int   `json:"torrent_count"`
	SpeedDownload int   `json:"speed_download"`
	SpeedUpload   int   `json:"speed_upload"`
	TotalDownload int64 `json:"total_download"`
	TotalUpload   int64 `json:"total_upload"`
	ActiveCount   int   `json:"active_count"`
	WaitingCount  int   `json:"waiting_count"`
	StoppedCount  int   `json:"stopped_count"`
}

func (e *Engine) GetStats() SessionStats {
	stat, err := e.client.GetGlobalStat()
	if err != nil {
		return SessionStats{}
	}

	dl, _ := strconv.Atoi(stat.DownloadSpeed)
	ul, _ := strconv.Atoi(stat.UploadSpeed)
	active, _ := strconv.Atoi(stat.NumActive)
	waiting, _ := strconv.Atoi(stat.NumWaiting)
	stopped, _ := strconv.Atoi(stat.NumStopped)

	return SessionStats{
		TorrentCount:  active + waiting + stopped,
		SpeedDownload: dl,
		SpeedUpload:   ul,
		ActiveCount:   active,
		WaitingCount:  waiting,
		StoppedCount:  stopped,
	}
}

func (e *Engine) GetPeers(gid string) ([]aria2.Peer, error) {
	return e.client.GetPeers(gid)
}

func (e *Engine) getDownload(gid string) (*TangoDownload, error) {
	status, err := e.client.TellStatus(gid)
	if err != nil {
		// GID not in aria2c — check DB for a metadata→content redirect
		meta, dbErr := e.db.GetDownload(gid)
		if dbErr == nil && meta.ContentGID != "" {
			return e.getDownload(meta.ContentGID)
		}
		return nil, fmt.Errorf("download not found: %s", gid)
	}

	// If this is a metadata download, follow to the real content download
	if len(status.FollowedBy) > 0 {
		// Persist the redirect so it survives after aria2c purges the metadata GID
		e.db.SetContentGID(gid, status.FollowedBy[0])
		return e.followToContent(gid, status.FollowedBy[0])
	}

	meta, err := e.db.GetDownload(gid)
	if err != nil {
		// Check if this is a followed download — inherit metadata from parent
		if status.Following != "" {
			meta = e.inheritMeta(status.Following, gid, status.Dir)
		} else {
			meta = database.DownloadMeta{
				GID:        gid,
				TargetPath: status.Dir,
				AddedAt:    time.Now(),
			}
		}
	}

	// Update name in DB if we now have it
	name := downloadName(status)
	if name != "" && meta.Name == "" {
		e.db.UpdateName(gid, name)
		meta.Name = name
	}

	return &TangoDownload{status: status, meta: meta}, nil
}

// followToContent redirects from a metadata GID to its content GID,
// propagating DB metadata (target_path, label) from the original.
func (e *Engine) followToContent(metaGID, contentGID string) (*TangoDownload, error) {
	status, err := e.client.TellStatus(contentGID)
	if err != nil {
		return nil, fmt.Errorf("download not found: %s", contentGID)
	}

	meta := e.inheritMeta(metaGID, contentGID, status.Dir)

	name := downloadName(status)
	if name != "" && meta.Name == "" {
		e.db.UpdateName(contentGID, name)
		meta.Name = name
	}

	return &TangoDownload{status: status, meta: meta}, nil
}

// inheritMeta copies DB metadata (target_path, label) from a parent GID to a child GID.
func (e *Engine) inheritMeta(parentGID, childGID, dir string) database.DownloadMeta {
	parentMeta, err := e.db.GetDownload(parentGID)
	if err != nil {
		return database.DownloadMeta{
			GID:        childGID,
			TargetPath: dir,
			AddedAt:    time.Now(),
		}
	}

	childMeta := database.DownloadMeta{
		GID:        childGID,
		TargetPath: parentMeta.TargetPath,
		Label:      parentMeta.Label,
		AddedAt:    parentMeta.AddedAt,
	}
	e.db.SaveDownload(childMeta)
	return childMeta
}

// completionMonitor checks for completed downloads and handles file moving.
func (e *Engine) completionMonitor() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			e.checkCompletions()
		case <-e.stopCh:
			return
		}
	}
}

func (e *Engine) checkCompletions() {
	stopped, _ := e.client.TellStopped(0, 1000)
	for _, s := range stopped {
		if s.Status != "complete" {
			continue
		}
		// Skip metadata-only downloads — the real content has its own GID
		if len(s.FollowedBy) > 0 {
			continue
		}
		meta, err := e.db.GetDownload(s.GID)
		if err != nil || meta.Moved {
			continue
		}
		if !meta.CompletedAt.Valid {
			e.db.MarkCompleted(s.GID)
		}
		// Move files if target_path differs from download dir
		if meta.TargetPath != "" && meta.TargetPath != s.Dir {
			e.moveFiles(&s, meta)
		} else {
			e.db.MarkMoved(s.GID)
		}
	}
}

// seedingMonitor checks active torrents and enforces seed ratio limits.
func (e *Engine) seedingMonitor() {
	interval := time.Duration(e.cfg.Seeding.CheckIntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			e.checkSeedingLimits()
		case <-e.stopCh:
			return
		}
	}
}

func (e *Engine) checkSeedingLimits() {
	active, err := e.client.TellActive()
	if err != nil {
		return
	}

	for _, s := range active {
		// Only check seeding torrents
		if s.Seeder != "true" {
			continue
		}

		uploaded, _ := strconv.ParseInt(s.UploadLength, 10, 64)
		completed, _ := strconv.ParseInt(s.CompletedLength, 10, 64)

		if completed <= 0 {
			continue
		}

		ratio := float64(uploaded) / float64(completed)

		// Update stats in DB
		e.db.UpdateSeedStats(s.GID, ratio, uploaded, completed)

		// Check ratio limit
		if e.cfg.Seeding.TargetRatio > 0 && ratio >= e.cfg.Seeding.TargetRatio {
			log.Printf("Torrent %s reached seed ratio %.2f (target: %.2f), stopping",
				downloadName(&s), ratio, e.cfg.Seeding.TargetRatio)

			if e.cfg.Seeding.AutoRemove {
				e.client.ForceRemove(s.GID)
				if e.cfg.Seeding.AutoDeleteData {
					removeDownloadFiles(&s)
				}
				e.db.DeleteDownload(s.GID)
			} else {
				e.client.Pause(s.GID)
			}
		}

		// Check time limit
		if e.cfg.Seeding.MaxSeedTimeMinutes > 0 {
			meta, err := e.db.GetDownload(s.GID)
			if err != nil || !meta.CompletedAt.Valid {
				continue
			}
			elapsed := time.Since(meta.CompletedAt.Time)
			maxTime := time.Duration(e.cfg.Seeding.MaxSeedTimeMinutes) * time.Minute
			if elapsed >= maxTime {
				log.Printf("Torrent %s exceeded max seed time %v, stopping", downloadName(&s), maxTime)
				e.client.Pause(s.GID)
			}
		}
	}
}

// watchProcess monitors the aria2c subprocess and restarts it if it crashes.
func (e *Engine) watchProcess() {
	for {
		select {
		case err := <-e.process.Done():
			if err != nil {
				log.Printf("WARNING: aria2c exited with error: %v", err)
			} else {
				log.Println("WARNING: aria2c exited unexpectedly")
			}
		case <-e.stopCh:
			return
		}

		// Restart aria2c after a brief pause
		log.Println("Restarting aria2c in 2 seconds...")
		select {
		case <-time.After(2 * time.Second):
		case <-e.stopCh:
			return
		}

		if err := e.process.Start(); err != nil {
			log.Printf("ERROR: failed to restart aria2c: %v", err)
			return
		}
		if err := e.process.WaitReady(e.client, 10*time.Second); err != nil {
			log.Printf("ERROR: aria2c not ready after restart: %v", err)
			return
		}
		log.Println("aria2c restarted successfully, session restored from disk")
	}
}
