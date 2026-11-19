package audit

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Head is how many entries the log holds and the hash of the last one.
func (l *Log) Head() (n uint64, last string) {
	if l == nil {
		return 0, ""
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.n, l.last
}

// AnchorMessage is the line posted to an outside channel. Whoever holds it can later prove, with
// `crowsnest audit verify --anchor N:HASH`, that the file still holds that entry, so a rewrite of the whole file or
// a cut at its end shows.
func AnchorMessage(n uint64, hash string) string {
	return fmt.Sprintf("CrowsNest audit anchor: entry %d, hash %s (check it with: crowsnest audit verify --anchor %d:%s)", n, hash, n, hash)
}

// AnchorSoon is how long after a kick (a user, policy or sign-in change) the head is posted. It is short enough to
// shorten the window in which an intruder could cut the change out, and long enough to batch a burst of changes.
const AnchorSoon = 30 * time.Second

// RunAnchors posts the log's head through post every interval, and once at the start, whenever it has changed since
// the last post. A receive on kick (may be nil) also posts, after soon: changes to who may sign in are the ones an
// intruder most wants to erase. The panel can rewrite its own audit file; a hash that was sent somewhere it cannot
// reach cannot be rewritten. It returns when ctx ends.
func RunAnchors(ctx context.Context, l *Log, interval, soon time.Duration, kick <-chan struct{}, post func(msg string)) {
	var posted uint64
	send := func() {
		if n, last := l.Head(); n > posted {
			post(AnchorMessage(n, last))
			posted = n
		}
	}
	send()
	t := time.NewTicker(interval)
	defer t.Stop()
	var soonC <-chan time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			send()
		case <-kick:
			if soonC == nil {
				soonC = time.After(soon)
			}
		case <-soonC:
			soonC = nil
			send()
		}
	}
}

// CheckAnchor confirms the file is a good chain that holds entry n with exactly this hash. A file that was rewritten
// from the start, or cut back before entry n, fails.
func CheckAnchor(path string, n uint64, hash string) error {
	count, _, err := Verify(path)
	if err != nil {
		return fmt.Errorf("the chain is broken after %d good entries: %w", count, err)
	}
	if count < n {
		return fmt.Errorf("the file holds only %d entries, but the anchor is entry %d: it was cut off at the end or replaced", count, n)
	}
	f, err := os.Open(path) // #nosec G304 -- chosen by the operator
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		var e Entry
		if json.Unmarshal(sc.Bytes(), &e) == nil && e.N == n {
			if e.Hash != hash {
				return fmt.Errorf("entry %d has hash %s, not the anchor's %s: the file was rewritten", n, e.Hash, hash)
			}
			return nil
		}
	}
	return fmt.Errorf("entry %d was not found", n)
}
