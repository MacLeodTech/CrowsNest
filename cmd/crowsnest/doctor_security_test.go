package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const mountinfoSample = `22 1 8:1 / / rw,relatime shared:1 - ext4 /dev/sda1 rw
40 22 8:17 / /srv/games rw,nosuid,nodev,relatime shared:5 - ext4 /dev/sdb1 rw
41 22 8:33 / /srv/plain rw,relatime shared:6 - ext4 /dev/sdc1 rw
`

func writeMountinfo(t *testing.T) string {
	p := filepath.Join(t.TempDir(), "mountinfo")
	if err := os.WriteFile(p, []byte(mountinfoSample), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestMountOptionsUsesTheLongestMatchingMount(t *testing.T) {
	mi := writeMountinfo(t)
	for path, wantNosuid := range map[string]bool{"/srv/games": true, "/srv/games/palworld": true, "/srv/plain/x": false, "/home/me": false, "/srv/gamesX": false} {
		opts, known := mountOptions(path, mi)
		if !known || hasOpt(opts, "nosuid") != wantNosuid {
			t.Errorf("%s: opts %v known %v, want nosuid=%v", path, opts, known, wantNosuid)
		}
	}
	if _, known := mountOptions("/srv/games", filepath.Join(t.TempDir(), "missing")); known {
		t.Error("an unreadable mountinfo was reported as known")
	}
}

func TestDoctorSuidWarnsOnlyForVisibleFoldersWithoutNosuid(t *testing.T) {
	mi := writeMountinfo(t)
	run := func(path string) (string, *doctorReport) {
		var out bytes.Buffer
		r := &doctorReport{w: &out}
		doctorSuid(r, "the games folder", path, mi)
		return out.String(), r
	}
	// a path that is not a folder here is a note, never a warning
	if out, r := run("/srv/not-mounted-in-the-panel"); r.warns != 0 || !strings.Contains(out, "not visible") {
		t.Errorf("host-only path: %q (warns %d)", out, r.warns)
	}
	dir := t.TempDir() // exists; mountinfo has no entry for it except "/", which lacks nosuid
	mi2 := filepath.Join(t.TempDir(), "mi")
	_ = os.WriteFile(mi2, []byte("22 1 8:1 / / rw,relatime - ext4 /dev/sda1 rw\n"), 0o600)
	var out bytes.Buffer
	r := &doctorReport{w: &out}
	doctorSuid(r, "the backups folder", dir, mi2)
	if r.warns != 1 || !strings.Contains(out.String(), "nosuid") {
		t.Errorf("visible folder without nosuid: %q (warns %d)", out.String(), r.warns)
	}
}

func TestDoctorSymlinksFlagsOnlyLinksLeadingOutside(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "gs-one", "data"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "gs-one", "data"), filepath.Join(root, "gs-one", "inside")); err != nil {
		t.Fatal(err)
	}
	run := func() (string, *doctorReport) {
		var out bytes.Buffer
		r := &doctorReport{w: &out}
		doctorSymlinks(r, "the games folder", root)
		return out.String(), r
	}
	if out, r := run(); r.warns != 0 {
		t.Errorf("a link that stays inside was flagged: %s", out)
	}
	if err := os.Symlink(outside, filepath.Join(root, "gs-one", "escape")); err != nil {
		t.Fatal(err)
	}
	if out, r := run(); r.warns != 1 || !strings.Contains(out, "escape") {
		t.Errorf("a link leading outside: %q (warns %d)", out, r.warns)
	}
}

func TestDoctorSuidAlsoWantsNodev(t *testing.T) {
	dir := t.TempDir()
	mi := filepath.Join(t.TempDir(), "mi")
	run := func(opts string) (string, *doctorReport) {
		_ = os.WriteFile(mi, []byte("22 1 8:1 / / "+opts+" - ext4 /dev/sda1 rw\n"), 0o600)
		var out bytes.Buffer
		r := &doctorReport{w: &out}
		doctorSuid(r, "the games folder", dir, mi)
		return out.String(), r
	}
	if out, r := run("rw,nosuid,relatime"); r.warns != 1 || !strings.Contains(out, "nodev") {
		t.Errorf("nosuid without nodev: %q (warns %d)", out, r.warns)
	}
	if out, r := run("rw,nosuid,nodev,relatime"); r.warns != 0 || !strings.Contains(out, "nosuid,nodev") {
		t.Errorf("both set: %q (warns %d)", out, r.warns)
	}
}
