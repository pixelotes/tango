package aria2

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"
)

// Client communicates with aria2c via JSON-RPC over HTTP.
type Client struct {
	url    string
	secret string
	http   *http.Client
	id     atomic.Int64
}

// NewClient creates a new aria2 JSON-RPC client.
func NewClient(url, secret string) *Client {
	return &Client{
		url:    url,
		secret: secret,
		http: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// JSON-RPC request/response types

type rpcRequest struct {
	JsonRPC string        `json:"jsonrpc"`
	ID      int64         `json:"id"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
}

type rpcResponse struct {
	ID     int64           `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Download status returned by aria2c
type Status struct {
	GID             string `json:"gid"`
	Status          string `json:"status"`          // active, waiting, paused, error, complete, removed
	TotalLength     string `json:"totalLength"`     // bytes
	CompletedLength string `json:"completedLength"` // bytes
	UploadLength    string `json:"uploadLength"`    // bytes
	DownloadSpeed   string `json:"downloadSpeed"`   // bytes/sec
	UploadSpeed     string `json:"uploadSpeed"`     // bytes/sec
	InfoHash        string `json:"infoHash"`
	NumSeeders      string `json:"numSeeders"`
	NumPieces       string `json:"numPieces"`
	PieceLength     string `json:"pieceLength"`
	Connections     string `json:"connections"`
	Dir             string `json:"dir"`
	ErrorCode       string `json:"errorCode"`
	ErrorMessage    string `json:"errorMessage"`
	BitTorrent      *BTInfo  `json:"bittorrent"`
	Files           []File   `json:"files"`
	Seeder          string   `json:"seeder"`     // true/false
	FollowedBy      []string `json:"followedBy"` // GIDs spawned by this download (e.g. metadata → content)
	Following       string   `json:"following"`   // GID that spawned this download
}

type BTInfo struct {
	AnnounceList [][]string `json:"announceList"`
	Comment      string     `json:"comment"`
	Mode         string     `json:"mode"`         // single or multi
	Info         *BTMeta    `json:"info"`
}

type BTMeta struct {
	Name string `json:"name"`
}

type File struct {
	Index           string `json:"index"`
	Path            string `json:"path"`
	Length          string `json:"length"`
	CompletedLength string `json:"completedLength"`
	Selected        string `json:"selected"`
	URIs            []URI  `json:"uris"`
}

type URI struct {
	URI    string `json:"uri"`
	Status string `json:"status"` // used, waiting
}

type Peer struct {
	PeerID        string `json:"peerId"`
	IP            string `json:"ip"`
	Port          string `json:"port"`
	Bitfield      string `json:"bitfield"`
	AmChoking     string `json:"amChoking"`
	PeerChoking   string `json:"peerChoking"`
	DownloadSpeed string `json:"downloadSpeed"`
	UploadSpeed   string `json:"uploadSpeed"`
	Seeder        string `json:"seeder"`
}

type GlobalStat struct {
	DownloadSpeed   string `json:"downloadSpeed"`
	UploadSpeed     string `json:"uploadSpeed"`
	NumActive       string `json:"numActive"`
	NumWaiting      string `json:"numWaiting"`
	NumStopped      string `json:"numStopped"`
	NumStoppedTotal string `json:"numStoppedTotal"`
}

type VersionInfo struct {
	Version  string   `json:"version"`
	Features []string `json:"enabledFeatures"`
}

// call makes a JSON-RPC call to aria2c.
func (c *Client) call(method string, params ...interface{}) (json.RawMessage, error) {
	id := c.id.Add(1)

	// Prepend token if secret is set
	rpcParams := make([]interface{}, 0, len(params)+1)
	if c.secret != "" {
		rpcParams = append(rpcParams, "token:"+c.secret)
	}
	rpcParams = append(rpcParams, params...)

	req := rpcRequest{
		JsonRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  rpcParams,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	resp, err := c.http.Post(c.url, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("RPC call %s: %w", method, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var rpcResp rpcResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	if rpcResp.Error != nil {
		return nil, fmt.Errorf("aria2 error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	return rpcResp.Result, nil
}

// AddURI adds a download by URI (magnet link, HTTP URL, etc).
// Returns the GID of the new download.
func (c *Client) AddURI(uris []string, opts map[string]interface{}) (string, error) {
	result, err := c.call("aria2.addUri", uris, opts)
	if err != nil {
		return "", err
	}
	var gid string
	if err := json.Unmarshal(result, &gid); err != nil {
		return "", fmt.Errorf("parsing GID: %w", err)
	}
	return gid, nil
}

// AddTorrent adds a download from a .torrent file (base64-encoded).
func (c *Client) AddTorrent(torrentBase64 string, opts map[string]interface{}) (string, error) {
	result, err := c.call("aria2.addTorrent", torrentBase64, []string{}, opts)
	if err != nil {
		return "", err
	}
	var gid string
	if err := json.Unmarshal(result, &gid); err != nil {
		return "", fmt.Errorf("parsing GID: %w", err)
	}
	return gid, nil
}

// TellStatus returns the status of a download.
func (c *Client) TellStatus(gid string) (*Status, error) {
	result, err := c.call("aria2.tellStatus", gid)
	if err != nil {
		return nil, err
	}
	var status Status
	if err := json.Unmarshal(result, &status); err != nil {
		return nil, fmt.Errorf("parsing status: %w", err)
	}
	return &status, nil
}

// TellActive returns the list of active downloads.
func (c *Client) TellActive() ([]Status, error) {
	result, err := c.call("aria2.tellActive")
	if err != nil {
		return nil, err
	}
	var statuses []Status
	if err := json.Unmarshal(result, &statuses); err != nil {
		return nil, fmt.Errorf("parsing statuses: %w", err)
	}
	return statuses, nil
}

// TellWaiting returns waiting downloads.
func (c *Client) TellWaiting(offset, num int) ([]Status, error) {
	result, err := c.call("aria2.tellWaiting", offset, num)
	if err != nil {
		return nil, err
	}
	var statuses []Status
	if err := json.Unmarshal(result, &statuses); err != nil {
		return nil, fmt.Errorf("parsing statuses: %w", err)
	}
	return statuses, nil
}

// TellStopped returns stopped downloads.
func (c *Client) TellStopped(offset, num int) ([]Status, error) {
	result, err := c.call("aria2.tellStopped", offset, num)
	if err != nil {
		return nil, err
	}
	var statuses []Status
	if err := json.Unmarshal(result, &statuses); err != nil {
		return nil, fmt.Errorf("parsing statuses: %w", err)
	}
	return statuses, nil
}

// Remove removes a download. Returns the GID.
func (c *Client) Remove(gid string) (string, error) {
	result, err := c.call("aria2.remove", gid)
	if err != nil {
		return "", err
	}
	var removedGID string
	if err := json.Unmarshal(result, &removedGID); err != nil {
		return "", fmt.Errorf("parsing GID: %w", err)
	}
	return removedGID, nil
}

// ForceRemove forcefully removes a download.
func (c *Client) ForceRemove(gid string) (string, error) {
	result, err := c.call("aria2.forceRemove", gid)
	if err != nil {
		return "", err
	}
	var removedGID string
	if err := json.Unmarshal(result, &removedGID); err != nil {
		return "", fmt.Errorf("parsing GID: %w", err)
	}
	return removedGID, nil
}

// Pause pauses a download.
func (c *Client) Pause(gid string) (string, error) {
	result, err := c.call("aria2.pause", gid)
	if err != nil {
		return "", err
	}
	var pausedGID string
	if err := json.Unmarshal(result, &pausedGID); err != nil {
		return "", fmt.Errorf("parsing GID: %w", err)
	}
	return pausedGID, nil
}

// Unpause resumes a paused download.
func (c *Client) Unpause(gid string) (string, error) {
	result, err := c.call("aria2.unpause", gid)
	if err != nil {
		return "", err
	}
	var unpausedGID string
	if err := json.Unmarshal(result, &unpausedGID); err != nil {
		return "", fmt.Errorf("parsing GID: %w", err)
	}
	return unpausedGID, nil
}

// GetPeers returns peers for a BitTorrent download.
func (c *Client) GetPeers(gid string) ([]Peer, error) {
	result, err := c.call("aria2.getPeers", gid)
	if err != nil {
		return nil, err
	}
	var peers []Peer
	if err := json.Unmarshal(result, &peers); err != nil {
		return nil, fmt.Errorf("parsing peers: %w", err)
	}
	return peers, nil
}

// GetGlobalStat returns overall download/upload statistics.
func (c *Client) GetGlobalStat() (*GlobalStat, error) {
	result, err := c.call("aria2.getGlobalStat")
	if err != nil {
		return nil, err
	}
	var stat GlobalStat
	if err := json.Unmarshal(result, &stat); err != nil {
		return nil, fmt.Errorf("parsing global stat: %w", err)
	}
	return &stat, nil
}

// GetVersion returns aria2c version information.
func (c *Client) GetVersion() (*VersionInfo, error) {
	result, err := c.call("aria2.getVersion")
	if err != nil {
		return nil, err
	}
	var ver VersionInfo
	if err := json.Unmarshal(result, &ver); err != nil {
		return nil, fmt.Errorf("parsing version: %w", err)
	}
	return &ver, nil
}

// PurgeDownloadResult clears completed/error/removed downloads from memory.
func (c *Client) PurgeDownloadResult() error {
	_, err := c.call("aria2.purgeDownloadResult")
	return err
}

// RemoveDownloadResult removes a completed/error/removed download from memory.
func (c *Client) RemoveDownloadResult(gid string) error {
	_, err := c.call("aria2.removeDownloadResult", gid)
	return err
}
