package auth

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/macleodtech/crowsnest/internal/vault"
)

func testVault(t *testing.T, key string) *vault.Vault {
	t.Helper()
	v, err := vault.New(key, "")
	if err != nil {
		t.Fatal(err)
	}
	return v
}

const (
	keyA = "alpha-master-key-0123456789abcdef"
	keyB = "bravo-master-key-0123456789abcdef"
)

func TestUsersFileIsSealedWhenAVaultIsSet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users.env")
	SetUsersVault(testVault(t, keyA))
	t.Cleanup(func() { SetUsersVault(nil) })

	if err := SetUserHash(path, "alice", "$2a$10$abcdefghijklmnopqrstuv"); err != nil {
		t.Fatal(err)
	}
	if err := SetRole(path, "alice", RolePlayer, false); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(raw), "enc:v2:") {
		t.Fatalf("file is not sealed: %.40q", raw)
	}
	for _, leak := range []string{"ALICE", "alice", "$2a$10", "PLAYER"} {
		if strings.Contains(string(raw), leak) {
			t.Errorf("sealed file leaks %q", leak)
		}
	}
	users, err := LoadUsers(path)
	if err != nil || users["alice"].Hash != "$2a$10$abcdefghijklmnopqrstuv" || users["alice"].Role != RolePlayer {
		t.Fatalf("round trip = %+v, %v", users, err)
	}
	// The store a running server uses reads it too.
	if s, err := NewUserStore(path); err != nil {
		t.Fatal(err)
	} else if _, ok := s.Lookup("alice"); !ok {
		t.Error("store cannot see alice in the sealed file")
	}
}

func TestPlainUsersFileLoadsAndIsSealedAtStartup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users.env")
	if err := os.WriteFile(path, []byte("BOB_PASSWORD_HASH=h\nBOB_ROLE=admin\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	SetUsersVault(testVault(t, keyA))
	t.Cleanup(func() { SetUsersVault(nil) })

	if u, err := LoadUsers(path); err != nil || u["bob"].Role != RoleAdmin {
		t.Fatalf("plain file did not load with a vault set: %+v, %v", u, err)
	}
	if changed, err := SealUsersFile(path); err != nil || !changed {
		t.Fatalf("SealUsersFile = %v, %v; want it to seal", changed, err)
	}
	if raw, _ := os.ReadFile(path); !strings.HasPrefix(string(raw), "enc:v2:") {
		t.Fatalf("not sealed after SealUsersFile: %.40q", raw)
	}
	if u, err := LoadUsers(path); err != nil || u["bob"].Hash != "h" {
		t.Fatalf("after sealing: %+v, %v", u, err)
	}
	if changed, _ := SealUsersFile(path); changed {
		t.Error("sealing an already sealed file should do nothing")
	}
	// A missing or empty file is left alone.
	if changed, err := SealUsersFile(filepath.Join(t.TempDir(), "none")); changed || err != nil {
		t.Errorf("missing file: %v, %v", changed, err)
	}
}

func TestSealedUsersFileNeedsTheRightKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users.env")
	SetUsersVault(testVault(t, keyA))
	if err := SetUserHash(path, "alice", "h"); err != nil {
		t.Fatal(err)
	}
	SetUsersVault(nil)
	if _, err := LoadUsers(path); !errors.Is(err, ErrUsersFileSealed) {
		t.Errorf("no key: err = %v, want ErrUsersFileSealed", err)
	}
	// Writing without a key must not clobber the sealed file with plain text read from nothing.
	SetUsersVault(testVault(t, keyB))
	t.Cleanup(func() { SetUsersVault(nil) })
	if _, err := LoadUsers(path); err == nil || !strings.Contains(err.Error(), "cannot decrypt") {
		t.Errorf("wrong key: err = %v, want a decrypt error", err)
	}
	if err := SetRole(path, "alice", RoleAdmin, false); err == nil {
		t.Error("a write with the wrong key should fail, not replace the file")
	}
	SetUsersVault(testVault(t, keyA))
	if u, err := LoadUsers(path); err != nil || u["alice"].Hash != "h" {
		t.Errorf("the right key still opens it: %+v, %v", u, err)
	}
}

func TestResealUsers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users.env")
	a, b := testVault(t, keyA), testVault(t, keyB)
	SetUsersVault(a)
	t.Cleanup(func() { SetUsersVault(nil) })
	if err := SetUserHash(path, "alice", "h"); err != nil {
		t.Fatal(err)
	}
	if n, err := ResealUsers(path, a, b); err != nil || n != 1 {
		t.Fatalf("reseal = %d, %v", n, err)
	}
	if n, err := ResealUsers(path, a, b); err != nil || n != 0 { // run again: already done
		t.Fatalf("second reseal = %d, %v", n, err)
	}
	SetUsersVault(b)
	if u, err := LoadUsers(path); err != nil || u["alice"].Hash != "h" {
		t.Fatalf("under the new key: %+v, %v", u, err)
	}
	SetUsersVault(a)
	if _, err := LoadUsers(path); err == nil {
		t.Error("the old key still opens the file after a rekey")
	}
	// A plain file is sealed by a rekey too.
	plain := filepath.Join(t.TempDir(), "plain.env")
	os.WriteFile(plain, []byte("CAL_PASSWORD_HASH=x\n"), 0o600)
	if n, err := ResealUsers(plain, a, b); err != nil || n != 1 {
		t.Fatalf("plain reseal = %d, %v", n, err)
	}
}
