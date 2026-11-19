package backup

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/macleodtech/crowsnest/internal/games"
)

// A backup nobody has ever opened is a guess. VerifyNewest reads the newest archive of a game from start to end
// (gzip checksum, every tar header, or the zip's central directory and checksums), so a truncated or corrupt file is
// found while the game's earlier backups still exist, not on the day of a restore.

// Damaged is one archive that failed its check.
type Damaged struct {
	Game  string `json:"game"`
	File  string `json:"file"`
	Error string `json:"error"`
}

// verifyCache remembers which file was last read, by name, size and time, so an unchanged newest backup is read once
// and not on every pass.
type verifyCache struct {
	mu   sync.Mutex
	seen map[string]verified
}

type verified struct {
	name string
	size int64
	mod  time.Time
	err  string
}

// newestArchive is the most recently written regular file in dir that looks like an archive.
func newestArchive(dir string) (os.FileInfo, bool) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, false
	}
	var files []os.FileInfo
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".") || !e.Type().IsRegular() {
			continue
		}
		if fi, err := e.Info(); err == nil {
			files = append(files, fi)
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].ModTime().After(files[j].ModTime()) })
	if len(files) == 0 {
		return nil, false
	}
	return files[0], true
}

// CheckArchive reads the archive at path. Formats it does not know (.tar.gz/.tgz, .tar, .zip are known) return
// known=false: a backup the panel cannot judge is not reported as bad.
func CheckArchive(path string) (known bool, err error) {
	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		f, err := os.Open(path) // #nosec G304 -- inside the operator's backups folder
		if err != nil {
			return true, err
		}
		defer f.Close()
		zr, err := gzip.NewReader(f)
		if err != nil {
			return true, err
		}
		return true, readTar(zr)
	case strings.HasSuffix(lower, ".tar"):
		f, err := os.Open(path) // #nosec G304 -- inside the operator's backups folder
		if err != nil {
			return true, err
		}
		defer f.Close()
		return true, readTar(f)
	case strings.HasSuffix(lower, ".zip"):
		return true, readZip(path)
	}
	return false, nil
}

func readTar(r io.Reader) error {
	tr := tar.NewReader(r)
	n := 0
	for {
		_, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("after %d entries: %w", n, err)
		}
		n++
		if _, err := io.Copy(io.Discard, tr); err != nil { // #nosec G110 -- nothing is kept, so a bomb costs time, not memory; also checks the gzip checksum
			return fmt.Errorf("entry %d: %w", n, err)
		}
	}
	if n == 0 {
		return fmt.Errorf("the archive holds no files")
	}
	// drain the trailer so a gzip stream is read to its checksum
	_, err := io.Copy(io.Discard, r)
	return err
}

func readZip(path string) error {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer zr.Close()
	if len(zr.File) == 0 {
		return fmt.Errorf("the archive holds no files")
	}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("%s: %w", f.Name, err)
		}
		_, err = io.Copy(io.Discard, rc) // #nosec G110 -- checks the entry's CRC; nothing is kept
		rc.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", f.Name, err)
		}
	}
	return nil
}

// VerifyNewest checks the newest archive in each game's own backup folder and returns the ones that failed. A file
// already checked and unchanged since is not read again.
func (m *Manager) VerifyNewest(defs []games.GameDef) []Damaged {
	var bad []Damaged
	m.verify.mu.Lock()
	if m.verify.seen == nil {
		m.verify.seen = map[string]verified{}
	}
	m.verify.mu.Unlock()
	for _, d := range defs {
		if d.Backup == nil || d.Backup.LocalDir == "" {
			continue
		}
		fi, ok := newestArchive(d.Backup.LocalDir)
		if !ok {
			continue
		}
		m.verify.mu.Lock()
		prev, had := m.verify.seen[d.ID]
		m.verify.mu.Unlock()
		cur := verified{name: fi.Name(), size: fi.Size(), mod: fi.ModTime()}
		if had && prev.name == cur.name && prev.size == cur.size && prev.mod.Equal(cur.mod) {
			cur.err = prev.err
		} else if time.Since(fi.ModTime()) < 30*time.Second {
			continue // still being written: look again next pass
		} else {
			known, err := CheckArchive(filepath.Join(d.Backup.LocalDir, fi.Name()))
			if !known {
				continue
			}
			if err != nil {
				cur.err = err.Error()
			}
			m.verify.mu.Lock()
			m.verify.seen[d.ID] = cur
			m.verify.mu.Unlock()
		}
		if cur.err != "" {
			bad = append(bad, Damaged{Game: d.ID, File: cur.name, Error: cur.err})
		}
	}
	return bad
}
