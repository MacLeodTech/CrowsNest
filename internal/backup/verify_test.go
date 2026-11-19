package backup

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/macleodtech/crowsnest/internal/games"
)

func writeTarGz(t *testing.T, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := gzip.NewWriter(f)
	tw := tar.NewWriter(zw)
	body := []byte("world data, long enough to matter")
	_ = tw.WriteHeader(&tar.Header{Name: "world/level.dat", Mode: 0o600, Size: int64(len(body))})
	_, _ = tw.Write(body)
	_ = tw.Close()
	_ = zw.Close()
	_ = f.Close()
}

func TestCheckArchive(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "a.tar.gz")
	writeTarGz(t, good)
	if known, err := CheckArchive(good); !known || err != nil {
		t.Fatalf("good tar.gz: %v %v", known, err)
	}
	raw, _ := os.ReadFile(good)
	cut := filepath.Join(dir, "cut.tar.gz")
	_ = os.WriteFile(cut, raw[:len(raw)/2], 0o600)
	if known, err := CheckArchive(cut); !known || err == nil {
		t.Errorf("truncated tar.gz passed: %v %v", known, err)
	}
	flipped := append([]byte(nil), raw...)
	flipped[len(flipped)-6] ^= 0xff // inside the gzip checksum or data
	bad := filepath.Join(dir, "flip.tgz")
	_ = os.WriteFile(bad, flipped, 0o600)
	if _, err := CheckArchive(bad); err == nil {
		t.Error("a corrupted tgz passed")
	}
	zp := filepath.Join(dir, "w.zip")
	zf, _ := os.Create(zp)
	zw := zip.NewWriter(zf)
	w, _ := zw.Create("level.dat")
	_, _ = w.Write([]byte("data"))
	_ = zw.Close()
	_ = zf.Close()
	if known, err := CheckArchive(zp); !known || err != nil {
		t.Errorf("good zip: %v %v", known, err)
	}
	zb, _ := os.ReadFile(zp)
	_ = os.WriteFile(filepath.Join(dir, "cut.zip"), zb[:len(zb)/2], 0o600)
	if _, err := CheckArchive(filepath.Join(dir, "cut.zip")); err == nil {
		t.Error("a truncated zip passed")
	}
	other := filepath.Join(dir, "world.7z")
	_ = os.WriteFile(other, []byte("x"), 0o600)
	if known, _ := CheckArchive(other); known {
		t.Error("an unknown format was claimed to be known")
	}
}

func TestVerifyNewestChecksOnlyTheNewestAndOnlyOnce(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "old.tar.gz")
	writeTarGz(t, old)
	_ = os.WriteFile(old, []byte("garbage"), 0o600) // an old broken one is not the way back anyone relies on
	past := time.Now().Add(-time.Hour)
	_ = os.Chtimes(old, past, past)
	newest := filepath.Join(dir, "new.tar.gz")
	writeTarGz(t, newest)
	earlier := time.Now().Add(-time.Minute)
	_ = os.Chtimes(newest, earlier, earlier)

	m := &Manager{}
	defs := []games.GameDef{{ID: "g", Backup: &games.BackupSpec{LocalDir: dir}}}
	if bad := m.VerifyNewest(defs); len(bad) != 0 {
		t.Fatalf("a good newest backup reported: %+v", bad)
	}
	// the newest gets damaged after the fact (same name, new size and time): it is read again
	_ = os.WriteFile(newest, []byte("truncated"), 0o600)
	_ = os.Chtimes(newest, earlier.Add(time.Second), earlier.Add(time.Second))
	bad := m.VerifyNewest(defs)
	if len(bad) != 1 || bad[0].Game != "g" || bad[0].File != "new.tar.gz" {
		t.Fatalf("a damaged newest backup: %+v", bad)
	}
	if again := m.VerifyNewest(defs); len(again) != 1 {
		t.Errorf("the verdict was lost on the next pass: %+v", again)
	}
}
