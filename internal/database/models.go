package database

import (
	"database/sql"
	"time"
)

type DownloadMeta struct {
	GID           string
	ContentGID    string
	InfoHash      string
	Name          string
	TargetPath    string
	Label         string
	AddedAt       time.Time
	CompletedAt   sql.NullTime
	Moved         bool
	SeedRatio     float64
	TotalUpload   int64
	TotalDownload int64
}

func (db *DB) SaveDownload(meta DownloadMeta) error {
	_, err := db.conn.Exec(`
		INSERT INTO downloads (gid, content_gid, info_hash, name, target_path, label, added_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(gid) DO UPDATE SET
			content_gid = excluded.content_gid,
			info_hash = excluded.info_hash,
			name = excluded.name,
			target_path = excluded.target_path,
			label = excluded.label
	`, meta.GID, meta.ContentGID, meta.InfoHash, meta.Name, meta.TargetPath, meta.Label, meta.AddedAt)
	return err
}

func (db *DB) GetDownload(gid string) (DownloadMeta, error) {
	var m DownloadMeta
	err := db.conn.QueryRow(`
		SELECT gid, content_gid, info_hash, name, target_path, label, added_at, completed_at, moved, seed_ratio, total_upload, total_download
		FROM downloads WHERE gid = ?
	`, gid).Scan(&m.GID, &m.ContentGID, &m.InfoHash, &m.Name, &m.TargetPath, &m.Label, &m.AddedAt, &m.CompletedAt, &m.Moved, &m.SeedRatio, &m.TotalUpload, &m.TotalDownload)
	return m, err
}

// GetDownloadByContentGID finds a download by its content GID (for followed downloads).
func (db *DB) GetDownloadByContentGID(contentGID string) (DownloadMeta, error) {
	var m DownloadMeta
	err := db.conn.QueryRow(`
		SELECT gid, content_gid, info_hash, name, target_path, label, added_at, completed_at, moved, seed_ratio, total_upload, total_download
		FROM downloads WHERE content_gid = ?
	`, contentGID).Scan(&m.GID, &m.ContentGID, &m.InfoHash, &m.Name, &m.TargetPath, &m.Label, &m.AddedAt, &m.CompletedAt, &m.Moved, &m.SeedRatio, &m.TotalUpload, &m.TotalDownload)
	return m, err
}

// SetContentGID records the metadata→content GID mapping.
func (db *DB) SetContentGID(metaGID, contentGID string) error {
	_, err := db.conn.Exec(`UPDATE downloads SET content_gid = ? WHERE gid = ?`, contentGID, metaGID)
	return err
}

func (db *DB) ListDownloads() ([]DownloadMeta, error) {
	rows, err := db.conn.Query(`
		SELECT gid, content_gid, info_hash, name, target_path, label, added_at, completed_at, moved, seed_ratio, total_upload, total_download
		FROM downloads ORDER BY added_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var metas []DownloadMeta
	for rows.Next() {
		var m DownloadMeta
		if err := rows.Scan(&m.GID, &m.ContentGID, &m.InfoHash, &m.Name, &m.TargetPath, &m.Label, &m.AddedAt, &m.CompletedAt, &m.Moved, &m.SeedRatio, &m.TotalUpload, &m.TotalDownload); err != nil {
			return nil, err
		}
		metas = append(metas, m)
	}
	return metas, rows.Err()
}

func (db *DB) DeleteDownload(gid string) error {
	_, err := db.conn.Exec(`DELETE FROM downloads WHERE gid = ?`, gid)
	return err
}

func (db *DB) MarkCompleted(gid string) error {
	_, err := db.conn.Exec(`UPDATE downloads SET completed_at = CURRENT_TIMESTAMP WHERE gid = ?`, gid)
	return err
}

func (db *DB) MarkMoved(gid string) error {
	_, err := db.conn.Exec(`UPDATE downloads SET moved = TRUE WHERE gid = ?`, gid)
	return err
}

func (db *DB) UpdateSeedStats(gid string, ratio float64, uploaded, downloaded int64) error {
	_, err := db.conn.Exec(`
		UPDATE downloads SET seed_ratio = ?, total_upload = ?, total_download = ?
		WHERE gid = ?
	`, ratio, uploaded, downloaded, gid)
	return err
}

func (db *DB) UpdateName(gid, name string) error {
	_, err := db.conn.Exec(`UPDATE downloads SET name = ? WHERE gid = ?`, name, gid)
	return err
}
