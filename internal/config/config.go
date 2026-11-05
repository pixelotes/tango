package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Host         string         `yaml:"host"`
	Port         int            `yaml:"port"`
	DataDir      string         `yaml:"data_dir"`
	DatabasePath string         `yaml:"database_path"`
	WebUIEnabled bool           `yaml:"webui_enabled"`
	Aria2        Aria2Config    `yaml:"aria2"`
	Seeding      SeedConfig     `yaml:"seeding"`
	Trackers     TrackersConfig `yaml:"trackers"`
}

type TrackersConfig struct {
	ListURL        string `yaml:"list_url"`
	UpdateInterval string `yaml:"update_interval"`
	AutoAdd        bool   `yaml:"auto_add"`
}

type Aria2Config struct {
	RPCUrl    string `yaml:"rpc_url"`
	RPCSecret string `yaml:"rpc_secret"`
	// Bind the JSON-RPC endpoint on all interfaces instead of loopback only.
	// Needed when another container (e.g. Reel) drives aria2c directly.
	// Always pair it with rpc_secret.
	RPCListenAll bool `yaml:"rpc_listen_all"`
	// Path to aria2c binary (empty = search $PATH)
	BinPath string `yaml:"bin_path"`
	// aria2c listen port for BitTorrent peers
	ListenPort int `yaml:"listen_port"`
	// Max concurrent downloads
	MaxConcurrent int `yaml:"max_concurrent"`
	// Max connections per server
	MaxConnectionPerServer int `yaml:"max_connection_per_server"`
	// DHT enabled
	DHTEnabled bool `yaml:"dht_enabled"`
	// Peer exchange
	PEXEnabled bool `yaml:"pex_enabled"`
	// Max upload speed (0 = unlimited), in bytes/sec
	MaxUploadSpeed int `yaml:"max_upload_speed"`
	// Max download speed (0 = unlimited), in bytes/sec
	MaxDownloadSpeed int `yaml:"max_download_speed"`
	// Disk cache size in bytes
	DiskCache int64 `yaml:"disk_cache"`
	// Min split size for parallel downloads
	MinSplitSize string `yaml:"min_split_size"`
}

type SeedConfig struct {
	// Target seed ratio (e.g. 2.0 means upload 2x what you downloaded)
	TargetRatio float64 `yaml:"target_ratio"`
	// Max seed time in minutes (0 = no limit, ratio takes precedence)
	MaxSeedTimeMinutes int `yaml:"max_seed_time_minutes"`
	// Check interval for seeding status, in seconds
	CheckIntervalSeconds int `yaml:"check_interval_seconds"`
	// Auto-remove torrent after reaching target ratio
	AutoRemove bool `yaml:"auto_remove"`
	// Also delete data files when auto-removing
	AutoDeleteData bool `yaml:"auto_delete_data"`
}

func Defaults() Config {
	return Config{
		Host:         "0.0.0.0",
		Port:         7246,
		DataDir:      "./data",
		DatabasePath: "./session/tango.db",
		WebUIEnabled: true,
		Aria2: Aria2Config{
			RPCUrl:                 "http://127.0.0.1:6800/jsonrpc",
			RPCSecret:              "",
			BinPath:                "",
			ListenPort:             6881,
			MaxConcurrent:          5,
			MaxConnectionPerServer: 4,
			DHTEnabled:             true,
			PEXEnabled:             true,
			MaxUploadSpeed:         0,
			MaxDownloadSpeed:       0,
			DiskCache:              16 * 1024 * 1024, // 16 MiB
			MinSplitSize:           "10M",
		},
		Seeding: SeedConfig{
			TargetRatio:          1.5,
			MaxSeedTimeMinutes:   0,
			CheckIntervalSeconds: 30,
			AutoRemove:           false,
			AutoDeleteData:       false,
		},
		Trackers: TrackersConfig{
			ListURL:        "https://raw.githubusercontent.com/ngosang/trackerslist/master/trackers_all.txt",
			UpdateInterval: "168h",
			AutoAdd:        false,
		},
	}
}

func Load(path string) (Config, error) {
	cfg := Defaults()

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("reading config: %w", err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parsing config: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return cfg, fmt.Errorf("invalid config: %w", err)
	}

	return cfg, nil
}

func (c Config) validate() error {
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("port must be 1-65535, got %d", c.Port)
	}
	if c.DataDir == "" {
		return fmt.Errorf("data_dir is required")
	}
	if c.DatabasePath == "" {
		return fmt.Errorf("database_path is required")
	}
	if c.Seeding.TargetRatio < 0 {
		return fmt.Errorf("seeding.target_ratio must be >= 0")
	}
	return nil
}

func (c Config) ListenAddr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}
