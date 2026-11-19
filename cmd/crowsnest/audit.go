package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/macleodtech/crowsnest/internal/audit"
	"github.com/macleodtech/crowsnest/internal/config"
)

const auditUsage = `usage:
  crowsnest audit verify [--anchor N:HASH] [file]
                                  check the audit file's hash chain (default: server.audit_file); with --anchor,
                                  also that it still holds entry N with that hash (the notice CrowsNest posts to
                                  your notification channel, daily and at start)
  crowsnest audit export [file]   write the audit file to standard output

verify says where the chain first breaks: an entry that was changed, removed, added or moved. On its own it
cannot see entries cut off the end or a whole file rewritten; the anchor notices in your notification channel
can, because the panel cannot reach them.`

func runAudit(args []string, out io.Writer) error {
	if len(args) == 0 || (args[0] != "verify" && args[0] != "export") {
		return errors.New(auditUsage)
	}
	cmd, args := args[0], args[1:]
	var anchorN uint64
	var anchorHash string
	if cmd == "verify" && len(args) >= 2 && args[0] == "--anchor" {
		ns, h, ok := strings.Cut(args[1], ":")
		n, err := strconv.ParseUint(ns, 10, 64)
		if !ok || err != nil || n == 0 || len(h) != 64 {
			return errors.New("--anchor wants N:HASH, as in the anchor notice")
		}
		anchorN, anchorHash, args = n, h, args[2:]
	}
	if len(args) > 1 {
		return errors.New(auditUsage)
	}
	var path string
	if len(args) == 1 {
		path = args[0]
	} else {
		cfg, err := config.Load(configPath())
		if err != nil {
			return fmt.Errorf("config does not load: %w", err)
		}
		path = cfg.Server.AuditFile
	}
	if cmd == "export" {
		f, err := os.Open(path) // #nosec G304 G703 -- chosen by the operator
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(out, f)
		return err
	}
	n, last, err := audit.Verify(path)
	if err != nil {
		if n > 0 || !os.IsNotExist(err) {
			return fmt.Errorf("%s: BROKEN after %d good entries: %w", path, n, err)
		}
		return err
	}
	if n == 0 {
		fmt.Fprintf(out, "%s: empty\n", path)
		return nil
	}
	fmt.Fprintf(out, "%s: OK, %d entries\nlast hash: %s\n", path, n, last)
	if anchorN > 0 {
		if err := audit.CheckAnchor(path, anchorN, anchorHash); err != nil {
			return fmt.Errorf("%s: ANCHOR FAILED: %w", path, err)
		}
		fmt.Fprintf(out, "anchor OK: entry %d has the hash you gave\n", anchorN)
	}
	return nil
}
