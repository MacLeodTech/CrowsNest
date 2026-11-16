package auth

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"

	"github.com/macleodtech/crowsnest/internal/atomicfile"
	"github.com/macleodtech/crowsnest/internal/vault"
)

// usersPurpose binds the sealed users file to its role, so the same text copied into another sealed file fails to
// open (see vault.Seal).
const usersPurpose = "users-file"

// usersVault, when set, seals the users file on every write and opens it on every read. A plain-text file still
// loads (it is sealed by the next write, or by SealUsersFile at start-up).
var usersVault atomic.Pointer[vault.Vault]

// SetUsersVault turns on encryption of the users file under v. Call it once at start-up, before the first read,
// in every process that touches the file (the server and the command-line commands). nil turns it off.
func SetUsersVault(v *vault.Vault) { usersVault.Store(v) }

// ErrUsersFileSealed is returned when the users file is encrypted and this process has no master key for it.
var ErrUsersFileSealed = errors.New("the users file is encrypted but no master key is available (set CROWSNEST_MASTER_KEY, or keep master.key beside the session secret)")

// readUsersText reads path and, if it is sealed, opens it. A missing file is empty text, not an error.
func readUsersText(path string) (string, error) {
	b, err := os.ReadFile(path) // #nosec G304 -- users_file path comes from the operator's config.yaml
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("auth: read users file %s: %w", path, err)
	}
	text := string(b)
	if sealed := strings.TrimSpace(text); vault.IsSealed(sealed) {
		v := usersVault.Load()
		if v == nil {
			return "", fmt.Errorf("auth: %s: %w", path, ErrUsersFileSealed)
		}
		plain, _, err := v.Unseal(usersPurpose, sealed)
		if err != nil {
			return "", fmt.Errorf("auth: open users file %s: %w", path, err)
		}
		return plain, nil
	}
	return text, nil
}

// sealUsersText is what to write for the users file's plain text: sealed when a vault is set, as is otherwise.
func sealUsersText(plain string) (string, error) {
	v := usersVault.Load()
	if v == nil || plain == "" {
		return plain, nil
	}
	sealed, err := v.Seal(usersPurpose, plain)
	if err != nil {
		return "", err
	}
	return sealed + "\n", nil
}

// SealUsersFile encrypts a plain-text users file in place, reporting whether it changed anything. It does nothing
// without a vault, for a missing or empty file, or for a file that is already sealed with the current scheme.
func SealUsersFile(path string) (bool, error) {
	if usersVault.Load() == nil {
		return false, nil
	}
	usersMu.Lock()
	defer usersMu.Unlock()
	b, err := os.ReadFile(path) // #nosec G304 -- operator's users_file
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if len(strings.TrimSpace(string(b))) == 0 || vault.IsSealed(strings.TrimSpace(string(b))) {
		return false, nil
	}
	users, err := LoadUsers(path) // checks it parses before anything is rewritten
	if err != nil {
		return false, err
	}
	return true, writeUsers(path, users)
}

// ResealUsers moves the users file from one master key to another (crowsnest rekey), or to the newest scheme under
// the same key. A plain-text file is sealed with to. It returns 1 if the file changed, else 0, and is safe to run
// again: a file that from cannot open but to can is already done.
func ResealUsers(path string, from, to *vault.Vault) (int, error) {
	b, err := os.ReadFile(path) // #nosec G304 -- operator's users_file
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	text := strings.TrimSpace(string(b))
	if text == "" {
		return 0, nil
	}
	var out string
	if vault.IsSealed(text) {
		out, _, err = from.Reseal(to, usersPurpose, text)
	} else {
		out, err = to.Seal(usersPurpose, string(b))
	}
	if err != nil {
		return 0, err
	}
	if out == text {
		return 0, nil
	}
	if err := atomicfile.Write(path, []byte(out+"\n"), 0o600); err != nil {
		return 0, err
	}
	return 1, nil
}
