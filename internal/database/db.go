package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

type DB struct {
	conn *sql.DB
}

func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("creating database directory: %w", err)
	}

	conn, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	conn.SetMaxOpenConns(1)

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("migrating database: %w", err)
	}

	return db, nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) migrate() error {
	_, err := db.conn.Exec(`
		CREATE TABLE IF NOT EXISTS downloads (
			gid          TEXT PRIMARY KEY,
			content_gid  TEXT DEFAULT '',
			info_hash    TEXT,
			name         TEXT DEFAULT '',
			target_path  TEXT NOT NULL,
			label        TEXT DEFAULT '',
			added_at     DATETIME DEFAULT CURRENT_TIMESTAMP,
			completed_at DATETIME,
			moved        BOOLEAN DEFAULT FALSE,
			seed_ratio   REAL DEFAULT 0,
			total_upload  INTEGER DEFAULT 0,
			total_download INTEGER DEFAULT 0
		);
	`)
	if err != nil {
		return err
	}
	// Migration: add content_gid column for existing databases
	db.conn.Exec(`ALTER TABLE downloads ADD COLUMN content_gid TEXT DEFAULT ''`)
	return err
}
