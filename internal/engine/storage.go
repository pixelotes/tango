package engine

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"tango/internal/aria2"
	"tango/internal/database"
)

func (e *Engine) moveFiles(s *aria2.Status, meta database.DownloadMeta) {
	if meta.Moved || meta.TargetPath == "" {
		return
	}

	srcDir := s.Dir
	if srcDir == "" {
		return
	}

	destDir := meta.TargetPath
	if err := os.MkdirAll(destDir, 0755); err != nil {
		log.Printf("Failed to create target directory %s: %v", destDir, err)
		return
	}

	for _, f := range s.Files {
		if f.Path == "" {
			continue
		}
		completed, _ := strconv.ParseInt(f.CompletedLength, 10, 64)
		total, _ := strconv.ParseInt(f.Length, 10, 64)
		if completed < total {
			continue // Skip incomplete files
		}

		src := f.Path
		// Build destination path: keep the relative structure from srcDir
		rel, err := filepath.Rel(srcDir, src)
		if err != nil {
			rel = filepath.Base(src)
		}
		dst := filepath.Join(destDir, rel)

		if err := moveFile(src, dst); err != nil {
			log.Printf("Failed to move %s -> %s: %v", src, dst, err)
			return
		}
	}

	name := downloadName(s)
	log.Printf("Moved files for %s to %s", name, destDir)
	e.db.MarkMoved(s.GID)
}

// moveFile tries hardlink first, then falls back to copy+delete.
func moveFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	// Try hardlink first (instant, no extra space)
	if err := os.Link(src, dst); err == nil {
		return nil
	}

	// Fallback: copy then delete
	return copyAndDelete(src, dst)
}

func copyAndDelete(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening source: %w", err)
	}
	defer srcFile.Close()

	info, err := srcFile.Stat()
	if err != nil {
		return fmt.Errorf("stat source: %w", err)
	}

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return fmt.Errorf("creating destination: %w", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		os.Remove(dst)
		return fmt.Errorf("copying: %w", err)
	}

	if err := dstFile.Close(); err != nil {
		return fmt.Errorf("closing destination: %w", err)
	}
	srcFile.Close()

	return os.Remove(src)
}

// removeDownloadFiles removes the data files for a download.
func removeDownloadFiles(s *aria2.Status) {
	for _, f := range s.Files {
		if f.Path != "" {
			os.Remove(f.Path)
		}
	}
	// Try to remove the parent directory if empty
	if s.Dir != "" {
		name := downloadName(s)
		if name != "" {
			dir := filepath.Join(s.Dir, name)
			os.Remove(dir) // Only succeeds if empty
		}
	}
}
