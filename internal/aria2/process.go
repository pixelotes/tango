package aria2

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Process manages the aria2c subprocess.
type Process struct {
	cmd     *exec.Cmd
	binPath string
	args    []string
	done    chan error
	session string // path to session file for save/restore
}

// ProcessConfig holds the configuration for spawning aria2c.
type ProcessConfig struct {
	BinPath        string
	RPCPort        int
	RPCSecret      string
	RPCListenAll   bool
	DownloadDir    string
	ListenPort     int
	MaxConcurrent  int
	MaxConnPerSrv  int
	MinSplitSize   string
	DHTEnabled     bool
	PEXEnabled     bool
	MaxUploadSpeed int
	MaxDownSpeed   int
	DiskCache      int64
	SeedRatio      float64
}

// BaselineTrackers are always active, so a fresh install has peer and metadata
// sources before any public tracker list is loaded.
var BaselineTrackers = []string{
	"udp://router.bittorrent.com:6881/announce",
	"udp://router.utorrent.com:6881/announce",
	"udp://dht.transmissionbt.com:6881/announce",
	"udp://dht.libtorrent.org:25401/announce",
}

// NewProcess creates a new aria2c process manager.
func NewProcess(cfg ProcessConfig) *Process {
	binPath := cfg.BinPath
	if binPath == "" {
		binPath = "aria2c"
	}

	args := []string{
		// RPC
		"--enable-rpc=true",
		fmt.Sprintf("--rpc-listen-port=%d", cfg.RPCPort),
		fmt.Sprintf("--rpc-listen-all=%t", cfg.RPCListenAll),
		// Downloads
		fmt.Sprintf("--dir=%s", cfg.DownloadDir),
		fmt.Sprintf("--max-concurrent-downloads=%d", cfg.MaxConcurrent),
		"--file-allocation=none",
		"--continue=true",
		// BitTorrent
		fmt.Sprintf("--listen-port=%d", cfg.ListenPort),
		fmt.Sprintf("--enable-dht=%t", cfg.DHTEnabled),
		fmt.Sprintf("--enable-dht6=%t", cfg.DHTEnabled),
		fmt.Sprintf("--enable-peer-exchange=%t", cfg.PEXEnabled),
		"--bt-enable-lpd=true",
		"--bt-save-metadata=true",
		"--bt-load-saved-metadata=true",
		"--bt-detach-seed-only=true",
		"--follow-torrent=mem",
		// Public trackers as baseline peer/metadata sources (always active)
		"--bt-tracker=" + strings.Join(BaselineTrackers, ","),
		// Seeding: Tango's monitor enforces limits, let aria2c seed up to ratio
		fmt.Sprintf("--seed-ratio=%.2f", cfg.SeedRatio),
		// Keep completed/stopped downloads queryable
		"--max-download-result=1000",
		// Session persistence: save state periodically, restore on restart
		fmt.Sprintf("--save-session=%s/aria2.session", cfg.DownloadDir),
		"--save-session-interval=30",
		// Logging
		"--console-log-level=notice",
	}

	sessionFile := fmt.Sprintf("%s/aria2.session", cfg.DownloadDir)
	if _, err := os.Stat(sessionFile); err == nil {
		args = append(args, fmt.Sprintf("--input-file=%s", sessionFile))
	}

	if cfg.DHTEnabled {
		args = append(args,
			fmt.Sprintf("--dht-listen-port=%d", cfg.ListenPort),
			// Bootstrap nodes (required for fresh installs with no dht.dat)
			"--dht-entry-point=router.bittorrent.com:6881",
			"--dht-entry-point6=dht.transmissionbt.com:6881",
			// Persist the routing table alongside the download dir
			fmt.Sprintf("--dht-file-path=%s/dht.dat", cfg.DownloadDir),
			fmt.Sprintf("--dht-file-path6=%s/dht6.dat", cfg.DownloadDir),
		)
	}
	// aria2c accepts 1-16 connections per server; out-of-range values make it exit.
	if cfg.MaxConnPerSrv > 0 {
		n := cfg.MaxConnPerSrv
		if n > 16 {
			log.Printf("aria2: max_connection_per_server %d is above aria2c's limit, using 16", n)
			n = 16
		}
		args = append(args, fmt.Sprintf("--max-connection-per-server=%d", n))
	}
	if cfg.MinSplitSize != "" {
		args = append(args, fmt.Sprintf("--min-split-size=%s", cfg.MinSplitSize))
	}
	if cfg.RPCSecret != "" {
		args = append(args, fmt.Sprintf("--rpc-secret=%s", cfg.RPCSecret))
	}
	if cfg.DiskCache > 0 {
		args = append(args, fmt.Sprintf("--disk-cache=%d", cfg.DiskCache))
	}
	if cfg.MaxUploadSpeed > 0 {
		args = append(args, fmt.Sprintf("--max-overall-upload-limit=%d", cfg.MaxUploadSpeed))
	}
	if cfg.MaxDownSpeed > 0 {
		args = append(args, fmt.Sprintf("--max-overall-download-limit=%d", cfg.MaxDownSpeed))
	}

	return &Process{
		binPath: binPath,
		args:    args,
		done:    make(chan error, 1),
		session: sessionFile,
	}
}

// Start launches the aria2c subprocess.
func (p *Process) Start() error {
	// Reset done channel for restarts
	p.done = make(chan error, 1)

	niceArgs := append([]string{"-n", "10", p.binPath}, p.args...)
	p.cmd = exec.Command("nice", niceArgs...)
	p.cmd.Stdout = os.Stdout
	p.cmd.Stderr = os.Stderr

	if err := p.cmd.Start(); err != nil {
		return fmt.Errorf("starting aria2c: %w", err)
	}

	log.Printf("aria2c started (PID %d)", p.cmd.Process.Pid)

	go func() {
		p.done <- p.cmd.Wait()
	}()

	return nil
}

// WaitReady waits for aria2c RPC to become available.
func (p *Process) WaitReady(client *Client, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := client.GetVersion(); err == nil {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("aria2c RPC not ready after %v", timeout)
}

// Stop gracefully stops the aria2c process.
func (p *Process) Stop() error {
	if p.cmd == nil || p.cmd.Process == nil {
		return nil
	}

	log.Println("Stopping aria2c...")
	if err := p.cmd.Process.Signal(os.Interrupt); err != nil {
		// Process might already be dead
		return nil
	}

	select {
	case <-p.done:
		return nil
	case <-time.After(5 * time.Second):
		log.Println("aria2c did not stop gracefully, killing...")
		return p.cmd.Process.Kill()
	}
}

// Done returns a channel that receives when the process exits.
func (p *Process) Done() <-chan error {
	return p.done
}
